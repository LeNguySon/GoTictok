package main

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCommitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		wantErr error
	}{
		{name: "feature", message: "feat: add video feed endpoint"},
		{name: "fix with scope", message: "fix(auth): reject expired tokens"},
		{name: "breaking change marker", message: "refactor(api)!: drop v1 routes"},
		{name: "body and git comments are ignored", message: "# Please enter the message\n\ndocs: explain the gate\n\nLonger body here.\n"},
		{name: "windows line endings", message: "test: cover the parser\r\n\r\nbody\r\n"},
		{name: "every ECC type is accepted", message: "perf: cache the profile"},
		{name: "git merge commit", message: "Merge branch 'main' into feature"},
		{name: "git revert commit", message: "Revert \"feat: add video feed endpoint\""},
		{name: "autosquash commit", message: "fixup! feat: add video feed endpoint"},
		{name: "empty", message: "", wantErr: errEmptyCommit},
		{name: "only comments", message: "# comment\n\n# another\n", wantErr: errEmptyCommit},
		{name: "no type", message: "added some stuff", wantErr: errCommitFormat},
		{name: "unknown type", message: "feature: add video feed", wantErr: errCommitFormat},
		{name: "missing space after colon", message: "feat:add video feed", wantErr: errCommitFormat},
		{name: "missing description", message: "feat: ", wantErr: errCommitFormat},
		{name: "upper case type", message: "Feat: add video feed", wantErr: errCommitFormat},
		{name: "subject too long", message: "feat: " + strings.Repeat("x", maxSubjectLength), wantErr: errSubjectTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateCommitMessage(tt.message)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("validateCommitMessage(%q) error = %v, want %v", tt.message, err, tt.wantErr)
			}
		})
	}
}
