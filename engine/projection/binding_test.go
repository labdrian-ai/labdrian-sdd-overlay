package projection_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// The fixtures below are shared by every test file of package projection_test,
// including bindingstore_test.go. projection/fsstore keeps its own copies in
// helpers_test.go, because a _test package cannot import another's helpers.

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

func TestBindingConstants(t *testing.T) {
	if projection.BindingVersion != 1 {
		t.Errorf("BindingVersion = %d, want 1", projection.BindingVersion)
	}
	if projection.MaxBindingBytes != 4096 {
		t.Errorf("MaxBindingBytes = %d, want 4096 (4 KiB)", projection.MaxBindingBytes)
	}
}

func TestMarshalIsIndentedJSONInContractFieldOrderWithOneTrailingNewline(t *testing.T) {
	got, err := validBinding().Marshal()
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}
	want := fmt.Sprintf("{\n  \"version\": 1,\n  \"repo_key\": %q,\n  \"project_id\": \"proj-1\",\n  \"workflow_id\": \"wf-1\",\n  \"bound_at\": \"2026-09-29T10:00:00Z\"\n}\n", hex64("a"))
	if string(got) != want {
		t.Fatalf("Marshal() =\n%s\nwant\n%s", got, want)
	}
}

func TestMarshalThenParseRoundTrips(t *testing.T) {
	want := validBinding()
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}
	got, err := projection.ParseBinding(data)
	if err != nil {
		t.Fatalf("ParseBinding(Marshal()) = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestMarshalRefusesAnInvalidBinding(t *testing.T) {
	b := validBinding()
	b.Version = 2
	data, err := b.Marshal()
	if err == nil || data != nil {
		t.Fatalf("Marshal() = %q, %v, want no bytes and an error: an invalid binding must never be written", data, err)
	}
}

func TestValidateAcceptsAWellFormedBinding(t *testing.T) {
	if err := validBinding().Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidateAcceptsIdentifiersAtTheMaximumLength(t *testing.T) {
	b := validBinding()
	b.ProjectID = strings.Repeat("p", 128)
	b.WorkflowID = strings.Repeat("w", 128)
	if err := b.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for identifiers of exactly 128 characters", err)
	}
}

func TestValidateRefusals(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*projection.Binding)
		field string
	}{
		{"version zero", func(b *projection.Binding) { b.Version = 0 }, "version"},
		{"version from the future", func(b *projection.Binding) { b.Version = 2 }, "version"},
		{"negative version", func(b *projection.Binding) { b.Version = -1 }, "version"},

		{"empty repo key", func(b *projection.Binding) { b.RepoKey = "" }, "repo_key"},
		{"short repo key", func(b *projection.Binding) { b.RepoKey = strings.Repeat("a", 63) }, "repo_key"},
		{"long repo key", func(b *projection.Binding) { b.RepoKey = strings.Repeat("a", 65) }, "repo_key"},
		{"uppercase hex repo key", func(b *projection.Binding) { b.RepoKey = strings.Repeat("A", 64) }, "repo_key"},
		{"non-hex repo key", func(b *projection.Binding) { b.RepoKey = strings.Repeat("g", 64) }, "repo_key"},
		{"repo key with a trailing newline", func(b *projection.Binding) { b.RepoKey = strings.Repeat("a", 63) + "\n" }, "repo_key"},
		{"repo key that climbs out of the directory", func(b *projection.Binding) { b.RepoKey = "../" + strings.Repeat("a", 61) }, "repo_key"},

		{"empty project id", func(b *projection.Binding) { b.ProjectID = "" }, "project_id"},
		{"project id with a slash", func(b *projection.Binding) { b.ProjectID = "a/b" }, "project_id"},
		{"project id with a space", func(b *projection.Binding) { b.ProjectID = "a b" }, "project_id"},
		{"hidden project id", func(b *projection.Binding) { b.ProjectID = ".hidden" }, "project_id"},
		{"non-ASCII project id", func(b *projection.Binding) { b.ProjectID = "proyecto-" + string(rune(0xF1)) }, "project_id"},
		{"project id over 128 characters", func(b *projection.Binding) { b.ProjectID = strings.Repeat("p", 129) }, "project_id"},

		{"empty workflow id", func(b *projection.Binding) { b.WorkflowID = "" }, "workflow_id"},
		{"workflow id with a slash", func(b *projection.Binding) { b.WorkflowID = "a/b" }, "workflow_id"},
		{"hidden workflow id", func(b *projection.Binding) { b.WorkflowID = ".hidden" }, "workflow_id"},
		{"workflow id with a colon", func(b *projection.Binding) { b.WorkflowID = "wf:1" }, "workflow_id"},
		{"workflow id over 128 characters", func(b *projection.Binding) { b.WorkflowID = strings.Repeat("w", 129) }, "workflow_id"},

		{"empty timestamp", func(b *projection.Binding) { b.BoundAt = "" }, "bound_at"},
		{"timestamp with a numeric UTC offset", func(b *projection.Binding) { b.BoundAt = "2026-09-29T10:00:00+00:00" }, "bound_at"},
		{"timestamp with a local offset", func(b *projection.Binding) { b.BoundAt = "2026-09-29T12:00:00+02:00" }, "bound_at"},
		{"timestamp without a zone", func(b *projection.Binding) { b.BoundAt = "2026-09-29T10:00:00" }, "bound_at"},
		{"timestamp with a lowercase zone", func(b *projection.Binding) { b.BoundAt = "2026-09-29T10:00:00z" }, "bound_at"},
		{"timestamp with a space separator", func(b *projection.Binding) { b.BoundAt = "2026-09-29 10:00:00Z" }, "bound_at"},
		{"impossible date", func(b *projection.Binding) { b.BoundAt = "2026-13-40T10:00:00Z" }, "bound_at"},
		{"not a timestamp", func(b *projection.Binding) { b.BoundAt = "yesterday, Z" }, "bound_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBinding()
			tt.edit(&b)
			err := b.Validate()
			if err == nil {
				t.Fatalf("Validate(%+v) = nil, want an error naming %s", b, tt.field)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Errorf("Validate() = %q, want it to name %s", err, tt.field)
			}
		})
	}
}

func TestParseBindingAcceptsAValidDocument(t *testing.T) {
	got, err := projection.ParseBinding([]byte(rawBinding(hex64("a"))))
	if err != nil {
		t.Fatalf("ParseBinding() = %v, want nil", err)
	}
	if got != validBinding() {
		t.Fatalf("ParseBinding() = %+v, want %+v", got, validBinding())
	}
}

func TestParseBindingToleratesWhitespaceAroundTheDocument(t *testing.T) {
	for _, doc := range []string{"\n" + rawBinding(hex64("a")), rawBinding(hex64("a")) + "\n\n", "  \t" + rawBinding(hex64("a")) + " \r\n"} {
		if _, err := projection.ParseBinding([]byte(doc)); err != nil {
			t.Errorf("ParseBinding(%q) = %v, want nil: Marshal ends with a newline, and JSON allows surrounding whitespace", doc, err)
		}
	}
}

func TestParseBindingRefusals(t *testing.T) {
	valid := rawBinding(hex64("a"))
	tests := []struct {
		name  string
		input string
		want  string // a fragment the error must contain; empty means any error
	}{
		{"empty input", "", "empty"},
		{"whitespace only", " \n\t ", "empty"},
		{"an empty object", "{}", "version"},
		{"an unknown field", strings.Replace(valid, `"version":1,`, `"version":1,"extra":true,`, 1), `"extra"`},
		{"a duplicate key", strings.Replace(valid, `"version":1,`, `"version":1,"version":1,`, 1), "duplicate"},
		{"a duplicate key with a different value", strings.Replace(valid, `"version":1,`, `"version":2,"version":1,`, 1), "duplicate"},
		{"trailing data after the document", valid + `{}`, "trailing"},
		{"trailing garbage after the document", valid + "\nx", ""},
		{"two documents", valid + "\n" + valid, "trailing"},
		{"a case-variant field name", strings.Replace(valid, `"version"`, `"Version"`, 1), `"Version"`},
		{"an upper-case field name", strings.Replace(valid, `"project_id"`, `"PROJECT_ID"`, 1), `"PROJECT_ID"`},
		{"a string version", strings.Replace(valid, `"version":1`, `"version":"1"`, 1), ""},
		{"a fractional version", strings.Replace(valid, `"version":1`, `"version":1.0`, 1), ""},
		{"a numeric project id", strings.Replace(valid, `"project_id":"proj-1"`, `"project_id":5`, 1), ""},
		{"a null workflow id", strings.Replace(valid, `"workflow_id":"wf-1"`, `"workflow_id":null`, 1), "workflow_id"},
		{"a null version", strings.Replace(valid, `"version":1`, `"version":null`, 1), "version"},
		{"a missing field", strings.Replace(valid, `"bound_at":"2026-09-29T10:00:00Z"`, `"unbound_at":"2026-09-29T10:00:00Z"`, 1), ""},
		{"an array", "[]", ""},
		{"a string", `"binding"`, ""},
		{"a number", "42", ""},
		{"null", "null", ""},
		{"true", "true", ""},
		{"invalid UTF-8 inside a string", strings.Replace(valid, `proj-1`, "proj-\xff", 1), "UTF-8"},
		{"a truncated document", valid[:len(valid)-12], ""},
		{"a byte order mark", string(rune(0xFEFF)) + valid, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projection.ParseBinding([]byte(tt.input))
			if err == nil {
				t.Fatalf("ParseBinding(%q) = %+v, want an error", tt.input, got)
			}
			if got != (projection.Binding{}) {
				t.Errorf("ParseBinding() returned %+v alongside an error, want the zero Binding", got)
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseBinding() = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestParseBindingSizeCap pins the 4 KiB bound: a document of exactly the cap
// still parses (whitespace padding stands in for a large document, which a
// valid binding can never be), and one byte more is refused before any field
// is inspected.
func TestParseBindingSizeCap(t *testing.T) {
	valid := rawBinding(hex64("a"))
	atCap := valid + strings.Repeat(" ", projection.MaxBindingBytes-len(valid))
	if len(atCap) != projection.MaxBindingBytes {
		t.Fatalf("test document is %d bytes, want %d", len(atCap), projection.MaxBindingBytes)
	}
	if _, err := projection.ParseBinding([]byte(atCap)); err != nil {
		t.Errorf("ParseBinding(%d bytes) = %v, want nil at exactly the cap", len(atCap), err)
	}

	_, err := projection.ParseBinding([]byte(atCap + " "))
	if !errors.Is(err, projection.ErrBindingTooLarge) {
		t.Errorf("ParseBinding(%d bytes) = %v, want ErrBindingTooLarge", len(atCap)+1, err)
	}
}
