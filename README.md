# donkey

A seeder **orchestration framework** for Go — not a GORM seeding library.

`core` owns dependency graphs, execution planning, transaction policies and
run history. It has zero knowledge of GORM, SQLC, `database/sql`, or any
particular database. Adapters plug into `core`'s ports; your application
wires them together and embeds the CLI as its own subcommand. See
[`docs/roadmap.md`](docs/roadmap.md) for the full design rationale.

```
User CLI (embedded in your binary)
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
| `adapter/gorm` + `adapter/postgres` | done, integration-tested against real Postgres |
| `adapter/noop` | done — backs `plan`/`list`/`run --dry-run` |
| `core/cli` — `list`/`plan`/`run`/`status` | done |
| `cmd/seeder-init` scaffolding | not started (V0.5+, not blocking) |
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

    cmd := cli.New(runner, toSeed)
    if err := cmd.Execute(); err != nil {
        var ec core.ExitCoder
        if errors.As(err, &ec) {
            os.Exit(ec.ExitCode())
        }
        os.Exit(1)
    }
}
```

Full working copy: [`example/cmd/seed/main.go`](example/cmd/seed/main.go).

```bash
docker compose up -d                 # postgres:16 on localhost:5432
go run ./example/cmd/seed list       # what would run, in order — no DB write
go run ./example/cmd/seed plan       # same, numbered
go run ./example/cmd/seed status     # already-run vs pending — reads history
go run ./example/cmd/seed run --dry-run   # logs what it would do — no DB write at all
go run ./example/cmd/seed run        # actually seeds, marks history
docker compose down
```

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
go test ./...                       # unit tests only — no Docker required
docker compose up -d
go test ./... -tags=integration -v  # + real Postgres via GORM
docker compose down
```

The `integration` build tag keeps DB-touching tests out of the default
`go test ./...` run.
