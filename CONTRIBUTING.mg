# Contributing to donkey

Thanks for considering a contribution. This document exists because the
architecture here enforces a few boundaries on purpose — a PR that "just
makes it work" can accidentally violate one of them without any test
failing. Read this before touching `core/` or adding an adapter.

## Project status

Pre-1.0. No tagged release yet. The public API (`core`, `adapter/*`,
`core/cli`) can change without notice until `v1.0.0` ships. If you're
building something real on top of this, pin a commit hash, not `main`.

## The one rule everything else follows from

This is **not** a seeder library for GORM. It's an orchestration
framework for seeders, with adapters for different persistence
mechanisms. That single sentence is why the repository is shaped the way
it is, and it's the lens every PR touching `core/` should be reviewed
through.

`core` owns:
- identifying seeders and their dependencies
- building the dependency graph, detecting cycles, topological sort
- compiling execution plans
- running plans under a transaction policy
- execution history (skip already-run seeders)

`core` never owns:
- a database connection, credentials, connection pooling, host/port, SSL
- any ORM-specific type (`*gorm.DB`, `*sql.DB`, `*queries.Queries`, ...)
- SQL dialect details (`Placeholder()`, `QuoteIdentifier()`, `Upsert()`, ...)
- how idempotency is achieved (that's the seeder/adapter's job)

## The six risks to check before opening a PR

These come from the original design brainstorm and have already caused
real bugs when violated accidentally during development. If your change
touches `core/` or `adapter/`, check it against this list before
requesting review — and say in the PR description which ones you
considered.

1. **Core becomes an ORM.** Symptom: something like
   `Insert()`/`Find()`/`Update()`/`Delete()`/`Transaction()` on a generic
   interface in `core`. Avoid it — let the concrete ORM/driver do that
   work inside the adapter.

2. **Core coupled to Postgres.** Avoid Postgres-specific SQL or types in
   `core`. Note: `HistoryStore`'s concrete implementations are
   intentionally Postgres-only for now (fixed `public.seeder_history`
   table, no dialect abstraction) — that's a deliberate, documented
   trade-off, not an oversight. It'll grow per-engine implementations
   behind the same `core.HistoryStore` interface when MySQL/SQL Server
   adapters land.

3. **Core coupled to a specific persistence mechanism.** No
   `*gorm.DB`/`*sql.DB`/driver-specific type may appear in any `core`
   interface. The type assertion from `core.Identifiable` to a concrete,
   adapter-specific `Seeder` interface happens in exactly one place per
   adapter (its `Adapter.Execute`) — never in `core`, never in user code.

4. **A SQL dialect abstraction shows up too early.** Don't add
   `Placeholder()`, `QuoteIdentifier()`, `Upsert()`, or anything like it to
   `core`. GORM and database drivers already handle this; the framework's
   job is orchestration, not being a SQL builder.

5. **Core tries to solve idempotency universally.** It doesn't and
   shouldn't. Idempotency is the seeder author's responsibility, using
   whatever the engine/ORM offers (`FirstOrCreate`, `ON CONFLICT`, etc.).
   There is no checksum system — history is a linear log of what ran, not
   a versioned, diffable state. A changed seed becomes a **new** seeder
   (e.g. `004-fix-default-user-names`), not an edit to an old one.

6. **Fragmenting into too many packages too early.** Only split a package
   out when there's a concrete reason — e.g. `internal/scaffold` was
   extracted only once two adapters (`gorm`, `postgres`) both needed the
   same file-generation mechanics. Don't pre-emptively create
   `adapter/mysql/`, `adapter/sqlserver/`, etc. before there's a second
   consumer of whatever you'd be splitting out.

### A seventh risk found the hard way: package-level coupling

Two adapters that each claim independence (e.g. "use this if you don't
want GORM as a dependency") must actually live in **separate Go
packages**, not just separate files within one package. Go compiles
whole packages, not individual files — if `adapter/postgres/history.go`
imports `gorm.io/gorm` while `adapter/postgres/adapter.go` doesn't,
anyone importing the package to use the GORM-free path still pulls GORM
into their build. Verify with:

```bash
go list -deps ./adapter/postgres | grep -i gorm   # should print nothing
```

Run this kind of check whenever a PR claims an adapter is independent of
another — don't take the claim on faith from where a doc comment sits.

## Development setup

```bash
git clone <repo>
cd donkey
go mod download
docker compose up -d   # only needed for integration tests
```

## Running tests

```bash
go test ./...                        # unit tests — no Docker required
go test ./... -tags=integration -v   # + real Postgres, both adapters
```

Unit tests must never touch Docker or a network connection. If you're
adding a test that needs a real database, it belongs behind the
`integration` build tag (`//go:build integration` at the top of the
file) and should clean up after itself (`t.Cleanup`) so the suite is
re-runnable without tearing the container down.

### Verify regression tests actually catch the regression

Before you consider a bug fixed, don't just add a test and watch it
pass — that only proves the test runs, not that it detects the bug. The
pattern used throughout this codebase (and expected in review) is:

1. Write the test asserting correct behavior.
2. Temporarily reintroduce the bug you're fixing.
3. Run the test and confirm it fails, for the reason you expect.
4. Restore the fix and confirm the test passes again.

This has already caught real issues during development — a transaction
mode silently rolling back less than it should, a dry-run silently
writing to history it shouldn't have touched, a scaffold command
generating unparseable Go when a directory's basename wasn't a valid Go
identifier. None of those were hypothetical: each was found by someone
deliberately trying to break their own fix before trusting it.

## Adding a new adapter

An adapter is a new persistence mechanism (a new ORM, a new low-level
driver) or, in the future, a new database engine. To add one:

1. Define a `Seeder` interface specific to the adapter, extending
   `core.Identifiable`:
   ```go
   type Seeder interface {
       core.Identifiable
       Run(ctx context.Context, db YourConcreteType) error
   }
   ```
2. Implement `core.Adapter` (`Execute`), doing the type assertion to your
   adapter's `Seeder` in exactly this one place.
3. If the adapter supports transactions, implement
   `core.TransactionalEngine` as well, handing `fn` a `core.Scope`
   bundling both the adapter and the `HistoryStore` bound to the same
   transaction/connection — history marking and seeder execution must
   commit or roll back together.
4. Provide a `HistoryStore` implementation if none of the existing ones
   fit your engine (e.g. table name conventions differ, or the engine
   isn't Postgres).
5. Optionally, provide a `NewCmd()` (`*cobra.Command`) that scaffolds a
   new seeder file with your adapter's `Run` signature already filled
   in. Share the generic mechanics (slug/package validation, `--deps`
   rendering, overwrite protection) via `internal/scaffold` — write only
   the template that's specific to your adapter.
6. Add integration tests proving, against the real engine: a seeder runs
   and is skipped on the second run; `PerSeeder` isolates one seeder's
   failure without undoing earlier ones; `All` (if supported) rolls back
   the whole batch on any failure.
7. Run `go list -deps` against your new package and confirm it doesn't
   accidentally pull in another adapter's dependency (see risk #7 above).

`core/cli` never imports any adapter package. If your adapter ships a
`new` command, the consumer wires it in explicitly:

```go
cmd := cli.New(runner, seeders, youradapter.NewCmd())
```

## Commit and PR expectations

- Keep unrelated concerns in separate commits (e.g. a behavior fix and a
  docs update documenting it are two commits, not one).
- If your change touches `core/`, say explicitly in the PR description
  which of the risks above you checked against.
- If your change fixes a bug, describe how you verified the fix — ideally
  including the "reintroduce the bug, watch the test fail, revert"
  sequence above.
- New exported types/functions need a doc comment. Run
  `go doc -all ./core` (or whichever package you touched) and check
  nothing exported is undocumented.
- `go vet ./...` and `gofmt -l .` must be clean before requesting review.

## Exit code convention (don't change without discussion)

| Code | Meaning |
|---|---|
| 0 | success — everything applied, or nothing pending |
| 1 | generic, unclassified error |
| 2 | partial run (`PerSeeder` mode) — stopped mid-way, state preserved, resumable with another `run` |
| 3 | circular dependency detected in the plan |

This is a public contract consumers may script against
(`if [ $? -eq 2 ]; then ...`). Don't repurpose a code or add a new one
without discussing it in an issue first.

## Questions

Open an issue before investing time in a large PR, especially anything
touching `core/` or proposing a new transaction mode, a new CLI command,
or a change to the history/skip semantics. It's much cheaper to discuss
a design before code exists than after.