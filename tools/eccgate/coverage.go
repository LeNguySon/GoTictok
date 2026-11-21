package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// minCoverage is the ECC minimum statement coverage (common/testing.md).
const minCoverage = 80.0

var errNoCoverageMode = errors.New("coverage profile has no mode line")

type coverBlock struct {
	statements int
	covered    bool
}

// coveragePercent returns the share of statements covered in a Go cover
// profile. A block listed several times counts once, and is covered when any
// listing was executed. Packages without tests appear with zero counts, so
// untested code lowers the total instead of hiding from it.
func coveragePercent(profile []byte) (float64, error) {
	lines := strings.Split(strings.ReplaceAll(string(profile), "\r\n", "\n"), "\n")
	if !strings.HasPrefix(lines[0], "mode:") {
		return 0, errNoCoverageMode
	}

	blocks := map[string]coverBlock{}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, block, err := parseCoverLine(line)
		if err != nil {
			return 0, err
		}
		block.covered = block.covered || blocks[key].covered
		blocks[key] = block
	}

	total, covered := 0, 0
	for _, b := range blocks {
		total += b.statements
		if b.covered {
			covered += b.statements
		}
	}
	if total == 0 {
		return fullCoverage, nil
	}
	return fullCoverage * float64(covered) / float64(total), nil
}

const (
	// fullCoverage is the percentage reported when every statement ran.
	fullCoverage = 100.0
	// coverLineFields is the minimum number of columns in a profile line:
	// the block position, its statement count and its hit count.
	coverLineFields = 3
)

// parseCoverLine parses "file.go:1.2,3.4 <statements> <count>".
func parseCoverLine(line string) (string, coverBlock, error) {
	fields := strings.Fields(line)
	if len(fields) < coverLineFields {
		return "", coverBlock{}, fmt.Errorf("malformed coverage line %q", line)
	}
	last := len(fields) - 1

	statements, err := strconv.Atoi(fields[last-1])
	if err != nil {
		return "", coverBlock{}, fmt.Errorf("coverage line %q: statement count: %w", line, err)
	}
	count, err := strconv.ParseUint(fields[last], 10, 64)
	if err != nil {
		return "", coverBlock{}, fmt.Errorf("coverage line %q: hit count: %w", line, err)
	}

	key := strings.Join(fields[:last-1], " ")
	return key, coverBlock{statements: statements, covered: count > 0}, nil
}
