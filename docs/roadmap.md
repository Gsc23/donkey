GUIA DE IMPLEMENTAÇÃO — FRAMEWORK DE SEEDERS EM GO
====================================================

Documento consolidado a partir do brainstorm original + sessão de fechamento
de decisões arquiteturais. Este arquivo é o guia de referência para a
implementação. Segue a ordem recomendada de execução.

PRINCÍPIO CENTRAL
==================

NÃO é uma "lib de seeder para GORM".
É um FRAMEWORK DE ORQUESTRAÇÃO DE SEEDERS com adapters plugáveis.

O core define contratos e resolve o problema de orquestração:

- registro e identificação de seeders;
- grafo de dependências, validação, detecção de ciclo, ordenação topológica;
- planejamento de execução (execution plan);
- execução, respeitando políticas de transação;
- histórico de execução.

O core NUNCA sabe de:

- conexão com banco, credenciais, pool, host/porta, SSL;
- API específica de GORM, SQLC ou database/sql;
- SQL builder, dialect, ORM.

A conexão é sempre responsabilidade do consumidor (aplicação do usuário).
O binário final contém apenas os adapters que o consumidor efetivamente
importar (o linker do Go cuida de remover o resto).

ARQUITETURA GERAL
==================

    User API / CLI
          │
       Runner
          │
       Planner  →  Dependency Graph → Validação → Ciclo → Toposort
          │
    Execution Plan
          │
       Adapter (contrato / port)
          │
     ┌────┼────┐
    GORM SQLC database/sql
          │
    PostgreSQL / MySQL / SQL Server

Hexagonal: Core → Port (Adapter interface) → Adapter concreto → tecnologia externa.

RISCOS ARQUITETURAIS A EVITAR (checklist permanente)
=====================================================

1. Core virar um ORM (Insert/Find/Update/Delete/Transaction genéricos).
   → Evitar: deixar o ORM/driver executar as operações de banco.

2. Core acoplado a PostgreSQL.
   → Evitar: SQL específico e tipos específicos de Postgres no core.
   → EXCEÇÃO CONSCIENTE: o HistoryStore inicial é Postgres-only
   (schema public). Isso é uma concessão pragmática documentada,
   não um esquecimento. Ver seção "HistoryStore" abaixo.

3. Core acoplado a GORM.
   → Evitar: *gorm.DB dentro de qualquer interface do core.

4. Criar abstração de SQL Dialect cedo demais.
   → Evitar: Placeholder(), QuoteIdentifier(), Upsert() no core.

5. Tentar resolver idempotência universalmente no core.
   → Deixar a cargo do seeder/adapter (FirstOrCreate, ON CONFLICT etc.)
   até existir necessidade clara e concreta.

6. Criar módulos/pacotes demais desde o primeiro commit.
   → Começar pequeno. Fragmentar só quando houver razão concreta.

DECISÕES FECHADAS — CONTRATOS DO CORE
========================================

1. IDENTIFIABLE (core/seeder.go)

---

O core só precisa disso para montar o grafo. Nada de Run() aqui.

    package core

    type Identifiable interface {
        ID() string
        Dependencies() []string
    }

IDs são versionados como eventos históricos, não como nomes de arquivo:
"2026-08-28-create-admin"
"2026-08-29-create-products"

Mudanças em dados já seedados viram NOVOS seeders (ex:
"004-fix-default-user-names"), nunca edição do seeder antigo.
Não há checksum — o histórico é uma sequência linear de operações
já executadas.

2. SEEDER TIPADO POR ADAPTER

---

Um seeder por adapter, com certeza. Cada adapter define seu próprio
contrato de execução, estendendo Identifiable:

    // adapter/gorm/seeder.go
    package gormadapter

    type Seeder interface {
        core.Identifiable
        Run(ctx context.Context, db *gorm.DB) error
    }

Se existir um adapter SQLC no futuro, ele define sqlcadapter.Seeder
com Run(ctx, *queries.Queries). Um seeder concreto do usuário PODE
implementar as duas interfaces se quiser portabilidade, mas isso
nunca é exigido pelo core.

3. ADAPTER (core/adapter.go)

---

Contrato mínimo, agnóstico de tecnologia:

    package core

    type Adapter interface {
        Execute(ctx context.Context, s Identifiable) error
    }

O type assertion para o tipo concreto do seeder acontece em UM único
lugar: dentro do adapter concreto, nunca no core, nunca no código do
usuário.

    // adapter/gorm/adapter.go
    package gormadapter

    type GORMAdapter struct{ db *gorm.DB }

    func New(db *gorm.DB) *GORMAdapter {
        return &GORMAdapter{db: db}
    }

    func (a *GORMAdapter) Execute(ctx context.Context, s core.Identifiable) error {
        gs, ok := s.(Seeder)
        if !ok {
            return fmt.Errorf("seeder %q não implementa gormadapter.Seeder", s.ID())
        }
        return gs.Run(ctx, a.db.WithContext(ctx))
    }

4. HISTORYSTORE (core/history.go)
------------------------------------

Interface no core, implementação concreta Postgres-only por decisão
consciente (trade-off aceito, documentado no ponto 2 do checklist de
riscos acima). Nome da tabela e schema são fixos, sem opção de
customização — decisão intencional para evitar overengineering
prematuro (Risco 6).

    // core/history.go
    package core

    type HistoryStore interface {
        EnsureSchema(ctx context.Context) error
        HasRun(ctx context.Context, id string) (bool, error)
        MarkRun(ctx context.Context, id string) error
    }

    // adapter/postgres/history.go
    package postgres

    const createTableSQL = `
    CREATE TABLE IF NOT EXISTS public.seeder_history (
        seeder_id   VARCHAR(255) PRIMARY KEY,
        executed_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );`

    type HistoryStore struct{ db *gorm.DB }

    func (h *HistoryStore) EnsureSchema(ctx context.Context) error {
        return h.db.WithContext(ctx).Exec(createTableSQL).Error
    }

    func (h *HistoryStore) HasRun(ctx context.Context, id string) (bool, error) {
        var count int64
        err := h.db.WithContext(ctx).
            Table("public.seeder_history").
            Where("seeder_id = ?", id).
            Count(&count).Error
        return count > 0, err
    }

    func (h *HistoryStore) MarkRun(ctx context.Context, id string) error {
        return h.db.WithContext(ctx).
            Exec(`INSERT INTO public.seeder_history (seeder_id) VALUES (?)`, id).Error
    }

Quando V0.7 (MySQL/SQL Server) chegar: essa implementação vira
postgres.HistoryStore e cada engine ganha a sua própria, todas atrás
da mesma interface core.HistoryStore.

5. NOOP ADAPTER (adapter/noop/adapter.go)

---

Usado por: `seed plan`, `seed list` e `seed run --dry-run`. Funciona
com QUALQUER seeder porque só olha ID() — nunca faz assertion de tipo
nem chama Run().

    package noop

    type Adapter struct {
        Log func(id string)
    }

    func New() *Adapter {
        return &Adapter{
            Log: func(id string) { fmt.Printf("[dry-run] executaria: %s\n", id) },
        }
    }

    func (a *Adapter) Execute(ctx context.Context, s core.Identifiable) error {
        a.Log(s.ID())
        return nil
    }

6. STEP / EXECUTIONPLAN (core/plan.go)
------------------------------------------

Carrega o resultado calculado do planejamento: se vai rodar ou pular,
e por quê. É o que a CLI imprime em `plan`/`status`.

    package core

    type StepAction int

    const (
        ActionRun StepAction = iota
        ActionSkip
    )

    type Step struct {
        Seeder Identifiable
        Action StepAction
        Reason string // "already executed", ou "" quando vai rodar
    }

    type ExecutionPlan struct {
        Steps []Step
    }

7. PLANNER (core/planner.go)
--------------------------------

Um único método serve dois casos de uso: plano estático sem tocar
banco (history == nil, usado por `seed plan`/`list`) e plano real
com skip calculado (history populado, usado por `seed run`/`status`).

    package core

    type Planner struct {
        history HistoryStore // pode ser nil
    }

    func NewPlanner(h HistoryStore) *Planner {
        return &Planner{history: h}
    }

    func (p *Planner) Compile(ctx context.Context, seeders []Identifiable) (*ExecutionPlan, error) {
        graph, err := buildGraph(seeders)
        if err != nil {
            return nil, err // erro de dependência inexistente
        }
        if cycle := graph.DetectCycle(); cycle != nil {
            return nil, &CycleError{Path: cycle}
        }
        ordered, err := graph.TopoSort()
        if err != nil {
            return nil, err
        }

        steps := make([]Step, 0, len(ordered))
        for _, s := range ordered {
            step := Step{Seeder: s, Action: ActionRun}
            if p.history != nil {
                ran, err := p.history.HasRun(ctx, s.ID())
                if err != nil {
                    return nil, err
                }
                if ran {
                    step.Action = ActionSkip
                    step.Reason = "already executed"
                }
            }
            steps = append(steps, step)
        }
        return &ExecutionPlan{Steps: steps}, nil
    }

Pendente de implementação (não bloqueia o design): core/graph.go com
buildGraph, DetectCycle, TopoSort. É a peça mais testável isoladamente
— não depende de banco nenhum, só de estruturas de dados.

8. TRANSACTIONALENGINE + SCOPE (core/adapter.go)

---

A transação precisa cobrir EXECUÇÃO + MARCAÇÃO DE HISTÓRICO como uma
unidade atômica — por isso o Scope entrega os dois juntos, presos à
mesma transação/conexão.

    package core

    type Scope struct {
        Adapter Adapter
        History HistoryStore
    }

    type TransactionalEngine interface {
        Adapter
        Transaction(ctx context.Context, fn func(ctx context.Context, scope Scope) error) error
    }

    // adapter/gorm/adapter.go
    func (a *GORMAdapter) Transaction(ctx context.Context, fn func(context.Context, core.Scope) error) error {
        return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
            scope := core.Scope{
                Adapter: &GORMAdapter{db: tx},
                History: postgres.NewHistoryStore(tx), // mesma tx, não conexão nova
            }
            return fn(ctx, scope)
        })
    }

9. RUNNER (core/runner.go)
------------------------------

Três modos de transação, todos usando o mesmo padrão de Scope.

    package core

    type TransactionMode int

    const (
        NoTransaction TransactionMode = iota
        PerSeeder
        All
    )

    type Runner struct {
        adapter Adapter
        history HistoryStore
        txMode  TransactionMode
    }

    func New(adapter Adapter, history HistoryStore, opts ...Option) *Runner { /* ... */ }

    func (r *Runner) History() HistoryStore { return r.history }

    func (r *Runner) Run(ctx context.Context, plan *ExecutionPlan) error {
        switch r.txMode {
        case All:
            return r.runAll(ctx, plan)
        case PerSeeder:
            return r.runPerSeeder(ctx, plan)
        default:
            return r.runSequential(ctx, plan)
        }
    }

MODO PERSEEDER — regra de negócio fechada:
"Cada seeder é uma transação isolada. Um seeder que falha aborta
SUA PRÓPRIA transação e interrompe os seguintes, mas os já aplicados
se mantêm. O estado é preservado via HistoryStore — não é necessário
nenhum mecanismo de checkpoint adicional, porque o próprio
seeder_history já é o checkpoint."

    func (r *Runner) runPerSeeder(ctx context.Context, plan *ExecutionPlan) error {
        txEngine, ok := r.adapter.(TransactionalEngine)
        if !ok {
            return ErrTransactionNotSupported
        }

        for i, step := range plan.Steps {
            if step.Action == ActionSkip {
                continue
            }
            err := txEngine.Transaction(ctx, func(txCtx context.Context, scope Scope) error {
                if err := scope.Adapter.Execute(txCtx, step.Seeder); err != nil {
                    return err
                }
                return scope.History.MarkRun(txCtx, step.Seeder.ID())
            })
            if err != nil {
                return &PartialRunError{
                    FailedID:  step.Seeder.ID(),
                    Completed: i,
                    Err:       err,
                }
            }
        }
        return nil
    }

Na próxima chamada de `seed run`, o Planner.Compile já marca como
ActionSkip tudo que tem MarkRun gravado — o loop recomeça exatamente
do seeder que falhou. Retomada automática, sem estado extra.

MODO ALL — mesmo padrão de Scope, transação única cobrindo tudo:

    func (r *Runner) runAll(ctx context.Context, plan *ExecutionPlan) error {
        txEngine, ok := r.adapter.(TransactionalEngine)
        if !ok {
            return ErrTransactionNotSupported
        }
        return txEngine.Transaction(ctx, func(txCtx context.Context, scope Scope) error {
            for _, step := range plan.Steps {
                if step.Action == ActionSkip {
                    continue
                }
                if err := scope.Adapter.Execute(txCtx, step.Seeder); err != nil {
                    return fmt.Errorf("seeder %q: %w", step.Seeder.ID(), err)
                }
                if err := scope.History.MarkRun(txCtx, step.Seeder.ID()); err != nil {
                    return err
                }
            }
            return nil
        })
    }

DIFERENÇA DE COMPORTAMENTO (documentar no README, evita confusão):

    | Aspecto        | PerSeeder                          | All                      |
    |----------------|-------------------------------------|--------------------------|
    | Falha no meio  | rollback só do seeder que falhou;   | rollback de TUDO,        |
    |                | anteriores ficam commitados         | mesmo o já "passado"     |
    | Retomada       | próxima seed run pula os já feitos  | refaz tudo do zero       |
    | Erro retornado | PartialRunError (exit code 2)       | erro genérico (exit 1)   |

10. ERROS E EXIT CODES (core/errors.go)
-------------------------------------------

    package core

    type PartialRunError struct {
        FailedID  string
        Completed int
        Err       error
    }

    func (e *PartialRunError) Error() string {
        return fmt.Sprintf("falhou em %q após %d seeder(s) aplicados: %v",
            e.FailedID, e.Completed, e.Err)
    }

    func (e *PartialRunError) ExitCode() int { return 2 }

    type CycleError struct{ Path []string }

    func (e *CycleError) ExitCode() int { return 3 }

    type ExitCoder interface {
        ExitCode() int
    }

TABELA DE EXIT CODES (convenção fechada):

    0 — sucesso, tudo aplicado (ou nada pendente)
    1 — erro genérico não classificado
    2 — execução parcial (PerSeeder) — parou no meio, estado
        preservado, retomável com novo `seed run`
    3 — dependência circular detectada no plano

11. CLI COMO TOOLKIT, NÃO COMO BINÁRIO DONO DA CONEXÃO
------------------------------------------------------------

Decisão que resolve a tensão "CLI genérica vs binário customizado"
do brainstorm original: a CLI não é um binário externo que conhece
connection string. Ela é uma casca fina (core/cli) que o usuário
embute no PRÓPRIO binário, recebendo um Runner já configurado.

    // core/cli/cli.go
    package cli

    func New(runner *core.Runner, seeders []core.Identifiable) *cobra.Command {
        root := &cobra.Command{Use: "seed"}
        root.AddCommand(listCmd(seeders))
        root.AddCommand(planCmd(seeders))
        root.AddCommand(runCmd(runner, seeders))
        root.AddCommand(statusCmd(runner, seeders))
        return root
    }

Uso do lado do usuário (o binário final da aplicação):

    // cmd/seed/main.go (do usuário)
    func main() {
        db, _ := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})

        history := postgres.NewHistoryStore(db)
        runner := core.New(gormadapter.New(db), history, core.PerSeeder)

        seeders := []core.Identifiable{
            seeders.CreateAdmin{},
            seeders.CreateProducts{},
            seeders.CreateOrders{},
        }

        cmd := cli.New(runner, seeders)
        if err := cmd.Execute(); err != nil {
            var ec core.ExitCoder
            if errors.As(err, &ec) {
                os.Exit(ec.ExitCode())
            }
            os.Exit(1)
        }
    }

COMANDOS:

    | Comando          | O que chama                          | Toca o banco? |
    |------------------|----------------------------------------|---------------|
    | seed list        | Planner com history=nil                | não           |
    | seed plan        | Planner com history=nil, imprime ordem | não           |
    | seed run         | Planner + Runner.Run real              | sim           |
    | seed run --dry-run | Planner + Runner com noop.Adapter    | não           |
    | seed status      | Planner com history real, sem executar | leitura       |

CORREÇÃO (pós-implementação, Passo 6) — "Runner com noop.Adapter" na
linha de --dry-run é impreciso e por pouco virou um bug real: trocar
SÓ o Adapter por noop.Adapter, mantendo o HistoryStore real, não
basta. runSequential (usado em NoTransaction) chama
HistoryStore.MarkRun depois de todo Execute bem-sucedido, e
noop.Execute sempre retorna nil — logo um `seed run --dry-run` feito
assim marcaria de verdade cada seeder como executado no banco, sem
ter rodado nada. A tabela promete "não toca o banco"; essa
implementação ingênua quebraria a promessa silenciosamente (só
apareceria depois, num `seed status` mostrando "já executado" para
algo que nunca rodou).

Implementação correta: compilar o plano SEMPRE com o HistoryStore
real (`core.NewPlanner(runner.History())`, pra `status`/skip
continuarem corretos), mas na hora de EXECUTAR o dry-run, trocar o
Adapter por noop.Adapter *e* zerar o HistoryStore para nil
(`core.New(noop.New(), nil, core.NoTransaction)`). O guard
`if r.history != nil` que já existia em runSequential — originalmente
só para permitir rodar sem HistoryStore configurado — passa a ser,
deliberadamente, a defesa contra escrita indevida durante o dry-run.

Regra geral pra não repetir esse erro: sempre que um "modo dry-run"
existir em cima de um Runner com efeitos colaterais em duas frentes
(execução E persistência de estado), as duas frentes têm que ser
checadas separadamente — trocar só uma peça (o Adapter) não garante
que a outra (o HistoryStore) também vira inofensiva. Isso está
coberto por teste de integração automatizado
(core/cli/cli_integration_test.go,
TestIntegration_RunDryRun_DoesNotWriteHistory) que falha se essa
regressão for reintroduzida.

`run <id>` (executar um seeder específico + dependências) foi
DELIBERADAMENTE ADIADO para V0.5+. Motivo: como o HistoryStore já
resolve retomada automática, `seed run` sozinho já cobre o caso de
uso real de "continuar de onde parou". Não decidir isso agora não
bloqueia nada.

Comando `seed init` (scaffolding) é um binário SEPARADO, instalável
globalmente via `go install`, porque não depende de Runner nenhum —
só escreve arquivos (main.go + seeders/). Ciclo de vida diferente do
toolkit acima (one-shot vs runtime).

ESTRUTURA DE PROJETO RECOMENDADA
====================================

Começar pequeno. Só fragmentar em módulos quando houver razão
concreta (Risco 6). Estrutura inicial sugerida:

    github.com/gs/seeder/

    ├── core/
    │   ├── seeder.go        (Identifiable)
    │   ├── adapter.go       (Adapter, TransactionalEngine, Scope)
    │   ├── plan.go          (Step, ExecutionPlan, StepAction)
    │   ├── planner.go       (Planner.Compile)
    │   ├── graph.go         (buildGraph, DetectCycle, TopoSort)
    │   ├── runner.go        (Runner, runSequential/runPerSeeder/runAll)
    │   ├── history.go       (interface HistoryStore)
    │   ├── errors.go        (PartialRunError, CycleError, ExitCoder)
    │   └── cli/
    │       └── cli.go
    │
    ├── adapter/
    │   ├── gorm/
    │   │   ├── seeder.go    (interface Seeder específica de GORM)
    │   │   └── adapter.go
    │   ├── postgres/
    │   │   └── history.go
    │   └── noop/
    │       └── adapter.go
    │
    └── cmd/
        └── seeder-init/     (scaffolding, binário separado)
            └── main.go

ROADMAP MVP (atualizado com CLI antecipada)
================================================

    V0.1
        - Identifiable, Runner, Adapter
        - GORM adapter + PostgreSQL
        - execução sequencial (NoTransaction)

    V0.2
        - Dependencies(), dependency graph
        - validação, detecção de ciclo, toposort
        - CLI: `list` e `plan` (grátis — só depende do Planner,
          sem tocar banco)

    V0.3
        - HistoryStore (Postgres, public.seeder_history)
        - skip de seeders já executados
        - CLI: `status`

    V0.4
        - Transaction policies: NoTransaction / PerSeeder / All
        - Scope{Adapter, History}, PartialRunError
        - CLI: `run` (com --dry-run via noop.Adapter)

    V0.5
        - Polimento da CLI: output JSON, flags extras
        - `run <id>` (subgraph) — decisão de design ainda em aberto

    V0.6
        - Suporte Postgres mais robusto
        - documentação, testes de integração

    V0.7
        - MySQL, SQL Server
        - HistoryStore ganha implementações específicas por engine

    V1.0
        - adapters adicionais, possível suporte a SQLC
        - execução concorrente de nós independentes do DAG
        - observabilidade

ORDEM DE IMPLEMENTAÇÃO RECOMENDADA (passo a passo)
========================================================

    1. go mod init + core/ com seeder.go, adapter.go, plan.go,
       errors.go — só os contratos já fechados, sem lógica ainda.

    2. core/graph.go — grafo de dependências, toposort e detecção
       de ciclo. É a peça com lógica não-trivial mais isolada e
       testável: não depende de banco nenhum. Escrever com testes
       desde o início (cobertura de: grafo válido, ciclo simples,
       ciclo indireto A->B->C->A, dependência inexistente).

    3. core/planner.go + core/runner.go usando o graph.
       Testar Planner.Compile com history=nil e com um
       HistoryStore fake/in-memory (mock), sem precisar de
       Postgres real ainda.

    4. adapter/gorm/ (Seeder + GORMAdapter) e adapter/postgres/
       (HistoryStore real). Aqui sim entra teste de integração
       contra Postgres de verdade (ex: via testcontainers).

    5. adapter/noop/ — trivial, mas necessário para `plan`/`list`/
       `--dry-run`.

    6. core/cli/ — comandos list, plan, run, status, com o
       dispatch de exit codes.

    7. cmd/seeder-init/ — scaffolding, pode vir por último, é
       o que menos afeta a qualidade arquitetural do restante.

PENDÊNCIAS EM ABERTO (não bloqueiam o início da implementação)
====================================================================

    - `run <id>` (executar um seeder + subgrafo de dependências):
      adiado para V0.5+, sem decisão de design ainda.

    - Formato de output JSON da CLI (para uso em scripts/CI):
      não definido, tratado como parte do polimento em V0.5.

    - HistoryStore para MySQL/SQL Server: só necessário a partir
      de V0.7, estrutura já permite (é uma interface no core).

FIM DO GUIA
