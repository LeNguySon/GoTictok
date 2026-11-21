// Command gotictok is the GoTictok application binary.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"gotictok/internal/app"
)

// Process exit codes.
const (
	exitOK = iota
	// exitError means the command failed and the reason was reported.
	exitError
	// exitNoStderr means the command failed and the reason could not even be
	// written to standard error.
	exitNoStderr
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is the testable body of main: it returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := app.Run(ctx, args, stdout); err != nil {
		if _, werr := fmt.Fprintf(stderr, "%s: %v\n", app.Name, err); werr != nil {
			return exitNoStderr
		}
		return exitError
	}
	return exitOK
}
