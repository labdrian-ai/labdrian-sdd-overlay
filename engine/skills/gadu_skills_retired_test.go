package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two GADU skills are retired with the agent they loaded (PR 5 of the role agents; the owner
// decision of 2026-10-23 retired gadu-orchestrate like gadu-operator): no registry entry, no manifest
// row and no directory in skills/. A retired skill leaves the approval baseline deliberately, which
// TestApprovalBaseline_PinnedToTheRepositoryRegistry pins. The registry entry is checked through the
// registry reader, which only the external test package can reach
// (TestTheGaduSkillsAreGoneFromTheRegistry).
func TestTheGaduSkillsAreGoneFromTheManifestAndTheTree(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	for _, id := range []string{"gadu-operator", "gadu-orchestrate"} {
		for _, row := range rows {
			if strings.HasPrefix(row.Path, id+"/") {
				t.Errorf("the manifest still has the row %+v of the retired skill %s", row, id)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "skills", id)); !os.IsNotExist(err) {
			t.Errorf("skills/%s is still there (Stat err = %v)", id, err)
		}
	}
}
