package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
)

// What a caller is told to set when no state directory can be named is one sentence, ops.StateDirHint, and the
// program says it from there: the failure of `index --embeddings`, the one of `status` (stateDirUnresolved) and
// the two checks of `doctor` all end in it. It was spelled out four times (two here, one in ops and one in a test
// of ops), so a change of the variable's name or of the wording had to be made in all of them and only some of
// them would have failed. The words are pinned once, in full, by TestStatusRefusesAStateDirectoryNobodyCanName
// and by the goldens of world 14; this test keeps the program from spelling them again.
func TestTheStateDirectoryHintIsSaidFromOnePlace(t *testing.T) {
	const words = "set HOME or LONGTERM_MEM_STATE_DIR"
	if ops.StateDirHint != words {
		t.Fatalf("ops.StateDirHint = %q, want %q", ops.StateDirHint, words)
	}

	var spelledIn []string
	for _, dir := range []string{".", filepath.Join("..", "..", "internal", "ops")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			source, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(source), words) {
				spelledIn = append(spelledIn, filepath.ToSlash(filepath.Join(dir, name)))
			}
		}
	}

	want := []string{"../../internal/ops/embeddingindex.go"}
	if strings.Join(spelledIn, " ") != strings.Join(want, " ") {
		t.Errorf("the hint %q is spelled in %v, want only %v (everything else says ops.StateDirHint)", words, spelledIn, want)
	}
}
