package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The script ends the options of cd, dirname and basename with "--", so a path that starts with "-" is read as a
// path (T4i, T4k). This shows that every shell found on the machine, and the dirname and basename it finds on PATH,
// accept that form: sh is required (skipped locally, a failure on CI, as the other evidence tests), the others run
// when found. A dirname that prints its "--" instead of reading it fails this test.
func TestEvidenceStore_EndOfOptionsHoldsInEveryShellFound(t *testing.T) {
	overlaytest.RequireShAndAwk(t)
	const script = "cd -- -w && pwd; dirname -- -x/y; basename -- -x/y"
	shells := [][]string{{"sh"}, {"dash"}, {"bash"}, {"ksh"}, {"mksh"}, {"zsh"}, {"busybox", "sh"}}
	ran := 0
	for _, shell := range shells {
		if _, err := exec.LookPath(shell[0]); err != nil {
			continue
		}
		ran++
		t.Run(shell[0], func(t *testing.T) {
			tmp, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(tmp, "-w"), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(shell[0], append(shell[1:], "-c", script)...)
			cmd.Dir = tmp
			cmd.Env = []string{"HOME=" + tmp, "PATH=" + os.Getenv("PATH")}
			wantResult(t, strings.Join(shell, " "), overlaytest.Capture(t, cmd), 0, filepath.Join(tmp, "-w")+"\n-x\ny\n", "")
		})
	}
	t.Logf("shells run: %d of %d tried", ran, len(shells))
}
