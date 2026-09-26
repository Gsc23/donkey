package core

import (
	"errors"
	"fmt"
	"strings"
)

// ErrTransactionNotSupported is returned when a Runner is configured with
// PerSeeder or All but the underlying Adapter does not implement
// TransactionalEngine.
var ErrTransactionNotSupported = errors.New("adapter não suporta execução transacional")

// ExitCoder is implemented by errors that map to a specific process exit
// code, letting a CLI dispatch os.Exit without knowing every error type.
type ExitCoder interface {
	ExitCode() int
}

// PartialRunError signals that a run stopped partway through: everything up
// to Completed is preserved (and recorded in the HistoryStore), and the next
// run will resume from FailedID.
type PartialRunError struct {
	FailedID  string
	Completed int
	Err       error
}

func (e *PartialRunError) Error() string {
	return fmt.Sprintf("falhou em %q após %d seeder(s) aplicados: %v", e.FailedID, e.Completed, e.Err)
}

func (e *PartialRunError) Unwrap() error { return e.Err }

func (e *PartialRunError) ExitCode() int { return 2 }

// CycleError signals that the dependency graph has a circular dependency,
// making the seeders impossible to order.
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("dependência circular detectada: %s", strings.Join(e.Path, " -> "))
}

func (e *CycleError) ExitCode() int { return 3 }
