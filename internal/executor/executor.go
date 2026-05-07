package executor

import (
	"context"
	"time"
)

type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

func (r ExecutionResult) Success() bool {
	return r.ExitCode == 0
}

type Executor interface {
	Execute(ctx context.Context, code string) (ExecutionResult, error)
}
