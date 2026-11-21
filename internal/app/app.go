// Package app is the entry point of the GoTictok application logic.
//
// It is a deliberately small placeholder: real features are added through the
// ECC workflow described in AGENTS.md (plan, test first, implement, review,
// verify), not by growing this file.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const (
	// Name is the program name shown to users.
	Name = "gotictok"
	// Version is the current application version.
	Version = "0.1.0"
)

// ErrUnknownCommand is returned when the first argument is not a known command.
var ErrUnknownCommand = errors.New("unknown command")

// Run executes the command named by args and writes its output to out.
// With no arguments it behaves like the "version" command.
func Run(ctx context.Context, args []string, out io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("run cancelled: %w", err)
	}

	command := "version"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "version":
		if _, err := fmt.Fprintf(out, "%s %s\n", Name, Version); err != nil {
			return fmt.Errorf("write version: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnknownCommand, command)
	}
}
