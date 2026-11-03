# donkey

A seeder **orchestration framework** for Go — not a GORM seeding library.

`core` owns dependency graphs, execution planning, transaction policies and
run history. It has zero knowledge of GORM, SQLC, `database/sql`, or any
particular database. Adapters plug into `core`'s ports; your application
wires them together and embeds the CLI as its own subcommand.

```
User CLI (embedded in your binary) ── depends only on cli.Seeds
        │
     core.Service   (driving port: Order / Status / Run)
        │
     Runner ── TransactionMode: NoTransaction | PerSeeder | All
        │
     Planner  →  Dependency Graph → Cycle detection → Topological sort
        │
  ExecutionPlan (Step: run / skip, with reason)
        │
     Adapter (core.Adapter port)
        │
 ┌──────┼──────┐
GORM   SQLC   database/sql        (only the ones you import ship in your binary)
        │
 PostgreSQL / MySQL / SQL Server
```

## Status

| Piece | State |
|---|---|
| `core` — graph, planner, runner, errors | done, unit-tested (no DB) |
| `adapter/gorm` (ORM) + `adapter/postgres` HistoryStore | done, integration-tested against real Postgres |
| `adapter/postgres` (native `database/sql`, no ORM) | done, integration-tested against real Postgres — see below |
| `adapter/noop` | done — logging no-op `core.Adapter` (no longer used by the CLI: dry-run lives in `core.Service`) |
| `core.Service` — driving port (`Order`/`Status`/`Run` + `RunReport`) | done, unit-tested (no DB) |
| `core/cli` — `list`/`plan`/`run`/`status`, talks only to `cli.Seeds` | done |
| `seed new <nome>` — `core.Service.New` + `core.Scaffolder` port (`gormadapter.NewScaffolder`, `postgres.NewScaffolder`) | done, embedded `.go.tmpl` per adapter, mechanics shared via `internal/scaffold` |
| `cmd/seeder-init` (scaffolds a whole new project) | not started (V0.5+, not blocking) |
| MySQL / SQL Server | not started (V0.7) |

## Install

```bash
go get github.com/Gsc23/donkey
```

## Writing a seeder

A seeder only needs `core.Identifiable` to be schedulable — the core never
sees how it runs:

```go
type Identifiable interface {
    ID() string
    Dependencies() []string
}
```

To actually run under the GORM adapter, it also implements
`gormadapter.Seeder`:

```go
// example/seeders/create_admin.go
type CreateAdmin struct{}

func (CreateAdmin) ID() string             { return "2026-09-26-create-admin" }
func (CreateAdmin) Dependencies() []string { return nil }

func (CreateAdmin) Run(ctx context.Context, db *gorm.DB) error {
    if err := db.WithContext(ctx).AutoMigrate(&User{}); err != nil {
        return err
    }
    return db.WithContext(ctx).FirstOrCreate(&User{Name: "admin"}, User{Name: "admin"}).Error
}
```

IDs are historical event names ("2026-09-26-create-admin"), not filenames.
A change to already-seeded data is always a *new* seeder, never an edit to
an old one — there's no checksum, the history is a linear log of what ran.

### Or: native Postgres, no ORM

If you don't want GORM as a dependency, `adapter/postgres` also has its own
Seeder contract, built on plain `database/sql` — no ORM, works with any
driver registered for Postgres (e.g. `github.com/jackc/pgx/v5/stdlib`):

```go
type CreateProducts struct{}

func (CreateProducts) ID() string             { return "2026-09-26-create-products" }
func (CreateProducts) Dependencies() []string { return []string{"2026-09-26-create-admin"} }

func (CreateProducts) Run(ctx context.Context, db postgres.Querier) error {
    _, err := db.ExecContext(ctx, `INSERT INTO products (name) VALUES ($1)`, "default")
    return err
}
```

`postgres.Querier` is the method set shared by `*sql.DB` and `*sql.Tx`
(`ExecContext`/`QueryContext`/`QueryRowContext`) — the same `Run` works
standalone (`NoTransaction`) or inside a transaction (`PerSeeder`/`All`);
it never needs to know which. Wire it the same way as the GORM adapter:

```go
db, _ := sql.Open("pgx", os.Getenv("DATABASE_URL"))
history := postgres.NewHistoryStore(db)
history.EnsureSchema(context.Background())

runner := core.New(postgres.NewAdapter(db), history, core.PerSeeder)
svc := core.NewService(runner, toSeed, core.WithScaffolder(postgres.NewScaffolder()))
cmd := cli.New(svc)
```

The two adapters are independent — pick one, or use both if different
seeders in your project prefer different styles (they share the same
`public.seeder_history` table format, but each keeps its own
`HistoryStore` implementation since one holds a `*gorm.DB` and the other a
`Querier`). This is a real package boundary, not just a naming
convention: `adapter/postgres` has zero GORM in its dependency graph
(`gormadapter.HistoryStore`, the `*gorm.DB`-based one, lives in
`adapter/gorm` instead, precisely so importing the native path never
compiles `gorm.io/gorm` — Go compiles whole packages, so keeping them
separate is what actually enforces "no ORM dependency", not just where
the doc comment happens to sit).

One asymmetry worth knowing if you're choosing between the two: inside a
`PerSeeder`/`All` transaction, `gormadapter`'s inner adapter still
technically permits calling `Transaction` again (GORM represents a
savepoint the same way it represents a transaction), while
`postgres.txAdapter` does not implement `TransactionalEngine` at all —
nesting doesn't apply to `*sql.Tx` here. Neither adapter's own seeders
are expected to call `Transaction` from inside `Run`, so this doesn't
show up in normal use.

## Wiring it into your own binary

The CLI is a toolkit (`core/cli`), not a standalone binary — you embed it,
because only your application knows the connection string:

```go
// example/cmd/seed/main.go
func main() {
    db, _ := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})

    history := pgstore.NewHistoryStore(db)
    history.EnsureSchema(context.Background()) // creates public.seeder_history once

    runner := core.New(gormadapter.New(db), history, core.PerSeeder)

    toSeed := []core.Identifiable{
        seeders.CreateAdmin{},
    }

    svc := core.NewService(runner, toSeed, core.WithScaffolder(gormadapter.NewScaffolder()))
    cmd := cli.New(svc)
    if err := cmd.Execute(); err != nil {
        var ec core.ExitCoder
        if errors.As(err, &ec) {
            os.Exit(ec.ExitCode())
        }
        os.Exit(1)
    }
}
```

Full working copy: [`example/cmd/seed/main.go`](example/cmd/seed/main.go). The
native-Postgres equivalent —
[`example/cmd/seed-postgres/main.go`](example/cmd/seed-postgres/main.go),
wiring `postgres.NewAdapter`/`postgres.NewHistoryStore`/`postgres.NewScaffolder`
against [`example/pgseeders`](example/pgseeders) instead — is identical in
shape, just swapping the adapter.

```bash
docker compose up -d                 # postgres:16 on localhost:5432
go run ./example/cmd/seed list       # what would run, in order — no DB write
go run ./example/cmd/seed plan       # same, numbered
go run ./example/cmd/seed status     # already-run vs pending — reads history
go run ./example/cmd/seed run --dry-run   # logs what it would do — no DB write at all
go run ./example/cmd/seed run        # actually seeds, prints "seeded: <id>" per step, marks history

# same five commands, native Postgres adapter instead of GORM:
go run ./example/cmd/seed-postgres list
go run ./example/cmd/seed-postgres run
docker compose down
```

Both binaries write to the same `public.seeder_history` table (they're
seeding the same `seeder_dev` database in this example), which is exactly
why their seeders use distinct IDs (`...-create-admin` vs.
`...-create-admin-pg`) — nothing stops two adapters sharing one project's
history, as long as IDs don't collide.

## Scaffolding a new seeder

`seed new` goes through core like every other command:

```
cli (new) ──cli.Seeds.New──► core.Service ──► «port» core.Scaffolder ──► adapter template ──► file
                                 └─ validates first: slug, --deps exist, ID not taken
```

- **core** owns the rules: the slug must be lowercase words joined by
  hyphens and start with a letter, the ID is `<today>-<slug>`, every
  `--deps` ID must be a seeder you already registered, and the new ID must
  not collide with one. A bad `--deps` fails here, not at the next `plan`.
- **the adapter** owns what the file looks like, because only it knows its
  `Run` signature (`*gorm.DB` vs. `postgres.Querier`). Each one ships a
  plain Go template, `seeder.go.tmpl`, embedded with `go:embed`
  ([`adapter/gorm/seeder.go.tmpl`](adapter/gorm/seeder.go.tmpl)), and
  exposes it as a `core.Scaffolder` via `NewScaffolder()`. Adapters don't
  import cobra — `core/cli` is the only package that does.
- **`internal/scaffold`** is the shared mechanics: derive package and type
  names, render, `gofmt` (so a template bug fails loudly instead of writing
  broken Go), and write without ever overwriting an existing file.

Enable it by passing the scaffolder of the adapter you use:

```go
svc := core.NewService(runner, toSeed, core.WithScaffolder(gormadapter.NewScaffolder()))
```

Without it, `seed new` returns `core.ErrScaffolderNotConfigured`. A future
SQLC adapter would ship its own `sqlcadapter.NewScaffolder()`.

```bash
go run ./example/cmd/seed new create-products \
    --dir ./example/seeders \
    --deps 2026-09-26-create-admin
```

```go
package seeders

type CreateProducts struct{}

func (CreateProducts) ID() string             { return "2026-09-26-create-products" }
func (CreateProducts) Dependencies() []string { return []string{"2026-09-26-create-admin"} }

func (CreateProducts) Run(ctx context.Context, db *gorm.DB) error {
    // TODO: implement
    return nil
}
```

Flags: `--dir` (default `./seeders`), `--package` (defaults to the
basename of `--dir` — pass it explicitly if that basename isn't a valid Go
identifier, e.g. a generated temp path), `--deps` (comma-separated IDs of
already-registered seeders). It refuses to overwrite an existing file, and **does not** add the
new type to your `toSeed` slice — that registration stays a manual,
explicit edit in your `main.go`.

Note: because the example `main.go` connects to Postgres unconditionally
before dispatching to any subcommand, running `seed new` (or even `list`/
`plan`, which also never touch the database) still requires
`docker compose up -d` there. That's a wrinkle in the *example's* wiring,
not in `core` or the adapters — a leaner `main.go` would defer opening the
DB connection until a command that actually needs it (`run`/`status`) is
invoked.

## Transaction modes

| Mode | On failure | Retry behavior | Error / exit code |
|---|---|---|---|
| `NoTransaction` | whatever the adapter already committed stays | resume via history skip | `PartialRunError`, exit 2 |
| `PerSeeder` | only the failing seeder's own transaction rolls back; earlier seeders stay committed | next `run` resumes exactly at the failed seeder | `PartialRunError`, exit 2 |
| `All` | the entire batch rolls back, even seeders that "passed" earlier in this run | next `run` starts over from scratch | generic error, exit 1 |

Exit codes: `0` nothing pending / all applied, `1` unclassified error, `2`
partial run (resumable), `3` circular dependency detected.

## Testing

```bash
go test ./...                        # unit tests only — no Docker required
go test ./... -tags=integration -v   # + real Postgres (both adapters) — needs Docker
```

The `integration` build tag keeps DB-touching tests out of the default
`go test ./...` run. Each integration package starts its own throwaway
`postgres:16` container via [testcontainers-go](https://golang.testcontainers.org/)
(`internal/testdb`), so packages run in parallel without sharing state, no
port is published on your machine, and there's nothing to start or stop by
hand. Set `DATABASE_URL` to skip the containers and use an existing database
instead — packages then share it, so add `-p 1`.

`docker-compose.yml` is only for running the example binaries.

## License

AGPL3 - see [LICENSE](./LICENSE.md)