package app_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"gotictok/internal/app"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr error
	}{
		{name: "no arguments prints version", args: nil, wantOut: app.Name},
		{name: "version command prints version", args: []string{"version"}, wantOut: app.Name},
		{name: "unknown command is rejected", args: []string{"bogus"}, wantErr: app.ErrUnknownCommand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			err := app.Run(context.Background(), tt.args, &out)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("Run() output = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}

func TestRunReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := app.Run(ctx, nil, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestRunReportsWriteFailure(t *testing.T) {
	t.Parallel()

	if err := app.Run(context.Background(), nil, failingWriter{}); err == nil {
		t.Fatal("Run() error = nil, want a write error")
	}
}
