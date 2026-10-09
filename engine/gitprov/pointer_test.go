package gitprov

import (
	"path/filepath"
	"testing"
)

func TestPointerTargetReadsWhatTheLineNamesAfterThePrefix(t *testing.T) {
	base := filepath.FromSlash("/base/dir")
	tests := []struct {
		name         string
		line, prefix string
		want         string
		ok           bool
	}{
		{"an absolute path", "gitdir: /abs/path", "gitdir: ", filepath.FromSlash("/abs/path"), true},
		{"a relative path is taken relative to the base", "gitdir: ../other", "gitdir: ", filepath.FromSlash("/base/other"), true},
		{"no prefix at all", "/abs/path", "", filepath.FromSlash("/abs/path"), true},
		{"a relative path with no prefix", "worktrees/wt", "", filepath.FromSlash("/base/dir/worktrees/wt"), true},
		{"the prefix is exact: no space after the colon", "gitdir:/abs/path", "gitdir: ", "", false},
		{"the prefix is exact: another case", "GITDIR: /abs/path", "gitdir: ", "", false},
		{"nothing after the prefix", "gitdir: ", "gitdir: ", "", false},
		{"an empty line with no prefix", "", "", "", false},
		{"the prefix is not there", "some other text", "gitdir: ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PointerTarget(tt.line, tt.prefix, base)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("PointerTarget(%q, %q, %q) = %q, %v, want %q, %v", tt.line, tt.prefix, base, got, ok, tt.want, tt.ok)
			}
		})
	}
}
