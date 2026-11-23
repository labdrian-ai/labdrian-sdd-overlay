package agentgen

// The origin and the licence a notice names (T3c-f of odd/tasks/merge-playbook-into-sdd-overlay.md, L52):
// the header of the notice comments is made from the fields of the Claude-only role, and the fields are held
// to the two files of the source that state the same things. This test reads unexported fields, so it lives
// in package agentgen.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheNoticeNamesTheOriginAndLicenseOfItsSource(t *testing.T) {
	t.Run("the fields agree with PROVENANCE.md and the notice file", func(t *testing.T) {
		checked := 0
		for _, r := range claudeOnlyRoles {
			if r.notice == "" {
				continue
			}
			checked++
			if r.origin == "" || r.licenseName == "" {
				t.Errorf("%s carries a notice but its origin %q or licenseName %q is empty", r.name, r.origin, r.licenseName)
				continue
			}
			dir := filepath.Join("..", "..", "roles", r.name)
			provenance, err := os.ReadFile(filepath.Join(dir, "PROVENANCE.md"))
			if err != nil {
				t.Fatal(err)
			}
			if want := "- Source repository: `" + r.origin + "`"; !strings.Contains("\n"+string(provenance), "\n"+want+"\n") {
				t.Errorf("roles/%s/PROVENANCE.md has no line %q", r.name, want)
			}
			notice, err := os.ReadFile(filepath.Join(dir, r.notice))
			if err != nil {
				t.Fatal(err)
			}
			if first, _, _ := strings.Cut(string(notice), "\n"); first != r.licenseName {
				t.Errorf("the first line of roles/%s/%s is %q, want licenseName %q", r.name, r.notice, first, r.licenseName)
			}
		}
		if checked == 0 {
			t.Error("no Claude-only role carries a notice: the test checked nothing")
		}
	})

	t.Run("the header is made from the fields and nothing else", func(t *testing.T) {
		r := claudeOnlyRole{role: role{name: "example"}, notice: "LICENSE", origin: "example/origin", licenseName: "Example License"}
		header, _, _ := strings.Cut(r.noticeComments("Example License\n"), "\n")
		for _, want := range []string{"example/origin", "Example License"} {
			if !strings.Contains(header, want) {
				t.Errorf("the header %q does not hold %q", header, want)
			}
		}
		for _, banned := range []string{"andresfrei/playbook", "MIT License"} {
			if strings.Contains(header, banned) {
				t.Errorf("the header %q holds %q, which is not the origin or licence of this role", header, banned)
			}
		}
	})
}
