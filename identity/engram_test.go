package identity

import "testing"

// Engram names a project by its repository: the last segment of the path its origin remote
// names, or the name of the directory where there is no remote. EngramName gives that name from
// the shared identity, so that the memory gate can compare an Engram read's project with the
// workflow's identity by exact equality (C23, decision 3 of M2; wired by M4).
func TestEngramNameIsTheNameEngramComputesForTheIdentity(t *testing.T) {
	for _, tt := range []struct {
		name     string
		identity string
		want     string
	}{
		{"a normalized origin is its last segment", "github.com/acme/widgets", "widgets"},
		{"a nested group keeps only the last segment", "gitlab.com/acme/platform/widgets", "widgets"},
		{"a port on the host does not change the segment", "git.example.com:2222/acme/widgets", "widgets"},
		{"a directory name is itself", "widgets", "widgets"},
		{"a declared name without a slash is itself", "chosen-name", "chosen-name"},
		{"a declared name with a slash is its last segment", "acme/widgets", "widgets"},
		{"the case is kept", "github.com/Acme/Widgets", "Widgets"},
		{"nothing is nothing", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := EngramName(tt.identity); got != tt.want {
				t.Errorf("EngramName(%q) = %q, want %q", tt.identity, got, tt.want)
			}
		})
	}
}
