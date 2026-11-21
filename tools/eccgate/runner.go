package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// command describes one external process the gate wants to run.
type command struct {
	dir  string
	name string
	args []string
	env  []string // extra KEY=VALUE entries added to the parent environment
}

// runner executes external commands and returns their combined output.
// It is the only seam between the gate and the operating system, so every
// check can be tested with a fake.
type runner interface {
	Run(ctx context.Context, c command) (string, error)
}

// execRunner runs commands for real.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, c command) (string, error) {
	//nolint:gosec // G204: the gate only runs its own fixed tool commands, never external input.
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	cmd.Dir = c.dir
	cmd.Env = append(os.Environ(), c.env...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", c.name, err)
	}
	return string(out), nil
}

// printer writes formatted text and remembers the first write error, so
// callers can print freely and check once at the end.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}
