package fsstore_test

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// hex64 returns a well-formed repo key made of 64 copies of one hex digit.
func hex64(digit string) string { return strings.Repeat(digit, 64) }

func validBinding() projection.Binding {
	return projection.Binding{
		Version:    projection.BindingVersion,
		RepoKey:    hex64("a"),
		ProjectID:  "proj-1",
		WorkflowID: "wf-1",
		BoundAt:    "2026-09-29T10:00:00Z",
	}
}

// rawBinding renders the compact JSON text of a valid binding for key, the
// starting point the byte-level tests corrupt one way at a time.
func rawBinding(key string) string {
	return fmt.Sprintf(`{"version":1,"repo_key":%q,"project_id":"proj-1","workflow_id":"wf-1","bound_at":"2026-09-29T10:00:00Z"}`, key)
}
