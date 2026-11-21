package main

import (
	"errors"
	"math"
	"testing"
)

func TestCoveragePercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile string
		want    float64
	}{
		{name: "no statements counts as fully covered", profile: "mode: atomic\n", want: 100},
		{name: "all blocks covered", profile: "mode: set\na.go:1.1,2.2 3 1\n", want: 100},
		{name: "mixed blocks weigh by statements", profile: "mode: atomic\na.go:1.1,2.2 3 5\na.go:3.1,4.2 1 0\n", want: 75},
		{name: "untested package lowers the total", profile: "mode: atomic\na.go:1.1,2.2 2 1\nb.go:1.1,9.2 8 0\n", want: 20},
		{name: "duplicate block is covered if any listing ran", profile: "mode: atomic\na.go:1.1,2.2 2 0\na.go:1.1,2.2 2 3\n", want: 100},
		{name: "duplicate block keeps coverage from an earlier listing", profile: "mode: atomic\na.go:1.1,2.2 2 3\na.go:1.1,2.2 2 0\n", want: 100},
		{name: "windows line endings", profile: "mode: atomic\r\na.go:1.1,2.2 1 1\r\na.go:3.1,4.2 1 0\r\n", want: 50},
		{name: "path containing a space", profile: "mode: atomic\nmy dir/a.go:1.1,2.2 1 1\n", want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := coveragePercent([]byte(tt.profile))
			if err != nil {
				t.Fatalf("coveragePercent() error = %v", err)
			}
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("coveragePercent() = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestCoveragePercentRejectsBadProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile string
		wantErr error
	}{
		{name: "empty input", profile: "", wantErr: errNoCoverageMode},
		{name: "missing mode line", profile: "a.go:1.1,2.2 1 1\n", wantErr: errNoCoverageMode},
		{name: "too few fields", profile: "mode: atomic\na.go:1.1,2.2 1\n"},
		{name: "statement count is not a number", profile: "mode: atomic\na.go:1.1,2.2 x 1\n"},
		{name: "hit count is not a number", profile: "mode: atomic\na.go:1.1,2.2 1 x\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := coveragePercent([]byte(tt.profile))
			if err == nil {
				t.Fatal("coveragePercent() error = nil, want an error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("coveragePercent() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
