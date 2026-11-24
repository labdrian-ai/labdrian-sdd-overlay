package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// B15: reviewed and rule compare the identifier or kind they are given with a cell as text, as reviewed-each does,
// on every awk found: a cell 01 is not 1, 1.0 is not 1 and 1e0 is not 1 (review 4f714c67e87c-5 F5, T4q, L78 choice 2).
// Each awk other than the one on PATH runs through a scratch bin/awk that execs it, first on PATH.
func TestEvidenceStore_ReviewedModesCompareAsTextOnEveryAwk(t *testing.T) {
	// No blank around a cell, so no trim makes the text of a cell differ from the bytes written.
	page := evidencePage("|alpha-kind|1||||project|\n|01|1||||project|\n", "|alpha-kind|1|01|2026-01-01|x|\n|alpha-kind|1|1.0|2026-01-02|y|\n")
	type awkFound struct {
		name string
		argv []string // nil: the awk on PATH as it is
	}
	found := []awkFound{{"awk on PATH", nil}}
	for _, name := range []string{"gawk", "mawk", "nawk", "original-awk"} {
		if path, err := exec.LookPath(name); err == nil {
			found = append(found, awkFound{name, []string{path}})
		}
	}
	if path, err := exec.LookPath("busybox"); err == nil {
		found = append(found, awkFound{"busybox awk", []string{path, "awk"}})
	}
	var ran []string
	for _, a := range found {
		ran = append(ran, a.name)
		t.Run(a.name, func(t *testing.T) {
			w := newEvidenceWorld(t, page)
			var env []string
			if a.argv != nil {
				dir := filepath.Join(w.tmp, "textawk")
				body := "#!/bin/sh\nexec"
				for _, word := range a.argv {
					body += " '" + word + "'"
				}
				writeEvidenceFile(t, filepath.Join(dir, "awk"), body+" \"$@\"\n")
				if err := os.Chmod(filepath.Join(dir, "awk"), 0o755); err != nil {
					t.Fatal(err)
				}
				env = []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
			}
			for _, c := range []struct {
				id   string
				exit int
			}{{"1", 1}, {"01", 0}, {"1.0", 0}, {"1e0", 1}} {
				wantResult(t, "reviewed "+c.id, w.run(w.repo, env, "reviewed", c.id), c.exit, "", "")
			}
			wantResult(t, "reviewed-each", w.run(w.repo, env, "reviewed-each", "docs/wiki", "1", "01", "1.0", "1e0"), 0, "no\nyes\nyes\nno\n", "")
			wantResult(t, "rule 01", w.run(w.repo, env, "rule", "01"), 0, "kind: 01\nlevel: 1\nrule: \napplies when: \nsource: \n", "")
			wantResult(t, "rule 1", w.run(w.repo, env, "rule", "1"), 1, "",
				"evidence.sh: no kind \"1\" in docs/wiki/evidence.md or "+filepath.Join(w.tmp, "skills", "_shared", "evidence", "evidence-rules.md")+"\n")
		})
	}
	t.Logf("awks run: %v", ran)
}
