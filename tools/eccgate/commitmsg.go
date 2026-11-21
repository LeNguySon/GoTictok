package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxSubjectLength mirrors the header limit ECC applies to its own commits.
const maxSubjectLength = 100

const commitFormatHelp = "expected \"<type>: <description>\" or \"<type>(<scope>): <description>\" " +
	"with type one of feat, fix, refactor, docs, test, chore, perf, ci, build, style, revert"

var (
	errEmptyCommit    = errors.New("commit message is empty")
	errSubjectTooLong = errors.New("commit subject is too long")
	errCommitFormat   = errors.New("commit subject is not a conventional commit")

	// reCommitSubject is the ECC commit format (common/git-workflow.md).
	reCommitSubject = regexp.MustCompile(
		`^(feat|fix|refactor|docs|test|chore|perf|ci|build|style|revert)(\([a-z0-9][a-z0-9._/-]*\))?!?: \S.*$`)
)

// validateCommitMessage checks the subject line of a commit message.
func validateCommitMessage(message string) error {
	subject := commitSubject(message)
	switch {
	case subject == "":
		return errEmptyCommit
	case isGitGenerated(subject):
		return nil
	case utf8.RuneCountInString(subject) > maxSubjectLength:
		return fmt.Errorf("%w (%d > %d characters): %q",
			errSubjectTooLong, utf8.RuneCountInString(subject), maxSubjectLength, subject)
	case !reCommitSubject.MatchString(subject):
		return fmt.Errorf("%w: %q\n%s", errCommitFormat, subject, commitFormatHelp)
	}
	return nil
}

// commitSubject returns the first line that is neither blank nor a git comment.
func commitSubject(message string) string {
	for _, line := range strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

// isGitGenerated reports subjects git writes itself for merges, reverts and
// autosquash commits; those are not held to the conventional format.
func isGitGenerated(subject string) bool {
	for _, prefix := range []string{"Merge ", "Revert \"", "fixup! ", "squash! ", "amend! "} {
		if strings.HasPrefix(subject, prefix) {
			return true
		}
	}
	return false
}
