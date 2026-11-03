package core

import (
	"errors"
	"fmt"
	"strings"
)

var ErrTransactionNotSupported = errors.New("adapter does not support transactional execution")

type ExitCoder interface {
	ExitCode() int
}

type PartialRunError struct {
	FailedID  string
	Completed int
	Err       error
}

func (e *PartialRunError) Error() string {
	return fmt.Sprintf("failed at %q after %d seeder(s) applied: %v", e.FailedID, e.Completed, e.Err)
}

func (e *PartialRunError) Unwrap() error { return e.Err }

func (e *PartialRunError) ExitCode() int { return 2 }

type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("circular dependency detected: %s", strings.Join(e.Path, " -> "))
}

func (e *CycleError) ExitCode() int { return 3 }
