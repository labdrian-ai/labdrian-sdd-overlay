// Package projection holds the session binding: a small record, stored outside
// every worktree, that says which workflow a repository follows. A binding is a
// pointer. It names a workflow by project_id and workflow_id and carries no
// workflow state of its own, so it never goes stale in a way the workflow's own
// log does not already reveal.
//
// The package also holds the projection itself (context.go): the pure decision
// of what a session is told about the workflow its repository is bound to, and
// the parsing and printing of the hook JSON around it.
//
// The package is pure Go over the standard library and the engine's own pure
// packages (jsonstrict, workflow, workflowprofile, memoryscope, capability). It
// starts no process and makes no network call (a static test in engine/runtime
// pins that), and it keeps nothing in memory between calls: every call reads the
// disk, so a restarted process sees exactly what the last one wrote.
package projection

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// BindingVersion is the only Binding wire version this package accepts.
const BindingVersion = 1

// MaxBindingBytes bounds the size of a binding document that ParseBinding
// decodes and of a binding file that Store.Load reads. A valid binding is under
// 500 bytes even with every identifier at its maximum length (three fields of
// at most 128 characters, a 64-character key, and a timestamp), so 4 KiB is
// generous. The bound exists so that a corrupted or hostile file is rejected
// without being read in full.
const MaxBindingBytes = 4 * 1024

// ErrBindingTooLarge is returned by ParseBinding when the input exceeds
// MaxBindingBytes.
var ErrBindingTooLarge = errors.New("projection: binding document exceeds the maximum size")

// Binding is the record that ties one repository to one workflow. RepoKey
// identifies the repository (the SHA-256, as 64 lowercase hex digits, of its
// git common directory, so every worktree of a repository shares one binding);
// ProjectID and WorkflowID name the workflow it follows; BoundAt is when the
// binding was made, an RFC3339 UTC timestamp ending in "Z".
type Binding struct {
	Version    int    `json:"version"`
	RepoKey    string `json:"repo_key"`
	ProjectID  string `json:"project_id"`
	WorkflowID string `json:"workflow_id"`
	BoundAt    string `json:"bound_at"`
}

// bindingFields lists every wire field of a Binding, for the exact-case
// unknown-field check.
var bindingFields = []string{"version", "repo_key", "project_id", "workflow_id", "bound_at"}

// repoKeyPattern is exactly 64 lowercase hex digits. In Go's regexp syntax "$"
// matches only at the end of the text (not before a trailing newline), so a
// key followed by a newline does not match.
var repoKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseBinding strictly parses one Binding document: bounded size, valid
// UTF-8, no duplicate keys, no unknown fields (exact case), no trailing data,
// the pinned version, and every field shape via Validate.
func ParseBinding(data []byte) (Binding, error) {
	if len(data) > MaxBindingBytes {
		return Binding{}, fmt.Errorf("parse binding: %w: %d bytes exceeds the maximum of %d", ErrBindingTooLarge, len(data), MaxBindingBytes)
	}
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return Binding{}, fmt.Errorf("parse binding: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return Binding{}, fmt.Errorf("parse binding: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "binding", bindingFields); err != nil {
		return Binding{}, fmt.Errorf("parse binding: %w", err)
	}
	var b Binding
	if err := jsonstrict.DecodeStrict(data, "binding", &b); err != nil {
		return Binding{}, fmt.Errorf("parse binding: %w", err)
	}
	if err := b.Validate(); err != nil {
		return Binding{}, fmt.Errorf("parse binding: %w", err)
	}
	return b, nil
}

// Validate checks every field of b and returns the first violation: the
// version is BindingVersion, repo_key is 64 lowercase hex digits,
// project_id and workflow_id are valid workflow identifiers
// (workflow.ValidateIdentifier), and bound_at is an RFC3339 UTC timestamp
// ending in "Z".
func (b Binding) Validate() error {
	if b.Version != BindingVersion {
		return fmt.Errorf("version must be %d, got %d", BindingVersion, b.Version)
	}
	if err := validateRepoKey(b.RepoKey); err != nil {
		return err
	}
	if err := workflow.ValidateIdentifier("project_id", b.ProjectID); err != nil {
		return err
	}
	if err := workflow.ValidateIdentifier("workflow_id", b.WorkflowID); err != nil {
		return err
	}
	return validateUTCTimestamp("bound_at", b.BoundAt)
}

// Marshal returns the indented JSON of a valid binding: fields in wire order,
// two-space indentation, and exactly one trailing newline. It refuses an
// invalid binding, so an invalid binding is never written.
func (b Binding) Marshal() ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("marshal binding: %w", err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal binding: %w", err)
	}
	return append(data, '\n'), nil
}

// validateRepoKey requires key to be exactly 64 lowercase hex digits. The
// store also relies on it for safety: a key that passes is a plain file name
// that cannot climb out of the bindings directory.
func validateRepoKey(key string) error {
	if !repoKeyPattern.MatchString(key) {
		return fmt.Errorf("repo_key must be 64 lowercase hex characters, got %q", key)
	}
	return nil
}

// validateUTCTimestamp requires an RFC3339 timestamp expressed in UTC (a "Z"
// offset), the same rule engine/workflow applies to an event's "at" field, so
// two writers recording the same instant produce byte-identical values.
func validateUTCTimestamp(name, value string) error {
	if !strings.HasSuffix(value, "Z") {
		return fmt.Errorf("%s must be an RFC3339 UTC timestamp ending in \"Z\", got %q", name, value)
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be a valid RFC3339 timestamp: %w", name, err)
	}
	return nil
}
