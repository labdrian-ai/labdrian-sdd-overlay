package projection_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// These tests are the rules of keeping a binding, on values alone: no file, no
// lock, no directory. The file-backed store that applies them is tested where it
// lives, in projection/fsstore.
//
// The shared fixtures hex64, validBinding, and rawBinding are defined in
// binding_test.go, which is in this same package (projection_test).

func owned(b projection.Binding) projection.Loaded {
	return projection.Loaded{Classification: projection.ClassificationOwned, Binding: b}
}

func stateOf(c projection.Classification, detail string) projection.Loaded {
	return projection.Loaded{Classification: c, Detail: detail}
}

func bindingFor(project, workflowID, at string) projection.Binding {
	return projection.Binding{Version: 1, RepoKey: hex64("a"), ProjectID: project, WorkflowID: workflowID, BoundAt: at}
}

func TestNewBindingStoresTheTimeAsUTCWithoutFractions(t *testing.T) {
	at := time.Date(2026, 9, 29, 12, 0, 0, 123456789, time.FixedZone("CEST", 2*3600))
	got := projection.NewBinding(hex64("a"), "proj-1", "wf-1", at)
	want := projection.Binding{Version: projection.BindingVersion, RepoKey: hex64("a"), ProjectID: "proj-1", WorkflowID: "wf-1", BoundAt: "2026-09-29T10:00:00Z"}
	if got != want {
		t.Fatalf("NewBinding() = %+v, want %+v", got, want)
	}
}

func TestClassifyBinding(t *testing.T) {
	valid := rawBinding(hex64("a"))
	atCap := valid + strings.Repeat(" ", projection.MaxBindingBytes-len(valid))
	tests := []struct {
		name    string
		data    string
		want    projection.Classification
		detail  string // a fragment of the detail, or the whole of it when exact
		exactly bool
	}{
		{"the compact document", valid, projection.ClassificationOwned, "", false},
		{"surrounding whitespace", "\n  " + valid + "  \n\n", projection.ClassificationOwned, "", false},
		{"exactly the size cap", atCap, projection.ClassificationOwned, "", false},
		{"an unrelated object", `{"hello":"world"}` + "\n", projection.ClassificationForeign, "not a binding we recognize", false},
		{"a newer version", strings.Replace(valid, `"version":1`, `"version":2`, 1), projection.ClassificationForeign, "version", false},
		{"a binding for another repository", rawBinding(hex64("b")), projection.ClassificationForeign,
			`binding file names repo_key "` + hex64("b") + `", but its file name says "` + hex64("a") + `"`, true},
		{"a JSON array", "[]\n", projection.ClassificationForeign, "", false},
		{"an empty file", "", projection.ClassificationMalformed, "binding file is empty", true},
		{"plain text", "not json\n", projection.ClassificationMalformed, "not one valid JSON document", false},
		{"data after the document", valid + "\n{}\n", projection.ClassificationMalformed, "data after the document", false},
		{"invalid UTF-8", strings.Replace(valid, "proj-1", "proj-\xff", 1), projection.ClassificationMalformed, "binding file is not valid UTF-8", true},
		{"one byte over the size cap", atCap + " ", projection.ClassificationMalformed, "binding file exceeds the maximum of 4096 bytes", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projection.ClassifyBinding(hex64("a"), []byte(tt.data))
			if got.Classification != tt.want {
				t.Fatalf("ClassifyBinding() = %+v, want classification %q", got, tt.want)
			}
			if tt.want == projection.ClassificationOwned {
				if got.Binding != validBinding() || got.Detail != "" {
					t.Fatalf("ClassifyBinding() = %+v, want the parsed binding and no detail", got)
				}
				return
			}
			if got.Binding != (projection.Binding{}) {
				t.Errorf("ClassifyBinding() carries %+v for a %s file, want the zero Binding", got.Binding, tt.want)
			}
			if tt.exactly && got.Detail != tt.detail || !tt.exactly && !strings.Contains(got.Detail, tt.detail) || got.Detail == "" {
				t.Errorf("ClassifyBinding() detail = %q, want %q", got.Detail, tt.detail)
			}
		})
	}
}

func TestAdmitBind(t *testing.T) {
	bound := bindingFor("proj-1", "wf-1", "2026-09-29T10:00:00Z")
	same := bindingFor("proj-1", "wf-1", "2026-09-30T10:00:00Z")
	other := bindingFor("proj-2", "wf-9", "2026-09-30T10:00:00Z")

	tests := []struct {
		name    string
		loaded  projection.Loaded
		record  projection.Binding
		replace bool
		write   bool
		wantErr error
		inErr   []string
	}{
		{"an absent binding is created", stateOf(projection.ClassificationAbsent, ""), same, false, true, nil, nil},
		{"the same workflow is a no-op", owned(bound), same, false, false, nil, nil},
		{"the same workflow is a no-op even when replacing", owned(bound), same, true, false, nil, nil},
		{"a different workflow is refused", owned(bound), other, false, false, projection.ErrAlreadyBound, []string{"wf-1", "proj-1"}},
		{"a different workflow is replaced when asked", owned(bound), other, true, true, nil, nil},
		{"a foreign file is refused", stateOf(projection.ClassificationForeign, "not ours"), other, true, false, projection.ErrRefuseForeignBinding, []string{"not ours"}},
		{"a malformed file is refused", stateOf(projection.ClassificationMalformed, "garbled"), other, true, false, projection.ErrRefuseMalformedBinding, []string{"garbled"}},
		{"an unavailable state is refused", stateOf(projection.ClassificationUnavailable, "permission denied"), other, true, false, projection.ErrBindingUnavailable, []string{"permission denied"}},
		{"an unknown classification is refused", stateOf("something else", ""), other, true, false, nil, []string{"unknown classification", "something else"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write, err := projection.AdmitBind(tt.loaded, tt.record, tt.replace)
			if write != tt.write {
				t.Errorf("AdmitBind() write = %v, want %v", write, tt.write)
			}
			requireError(t, err, tt.wantErr, tt.inErr)
		})
	}
}

func TestCheckExpected(t *testing.T) {
	good := bindingFor("proj-1", "wf-1", "2026-09-29T10:00:00Z")
	invalid := good
	invalid.ProjectID = "../escape"
	elsewhere := good
	elsewhere.RepoKey = hex64("b")

	if err := projection.CheckExpected("bind if unchanged", hex64("a"), good); err != nil {
		t.Errorf("CheckExpected(valid) = %v, want nil", err)
	}
	err := projection.CheckExpected("bind if unchanged", hex64("a"), invalid)
	if err == nil || !strings.HasPrefix(err.Error(), "projection store: bind if unchanged: expected binding: ") || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("CheckExpected(invalid) = %v, want the operation, \"expected binding\" and the invalid field", err)
	}
	want := `projection store: unbind if unchanged: expected binding names repo_key "` + hex64("b") + `", not "` + hex64("a") + `"`
	if err := projection.CheckExpected("unbind if unchanged", hex64("a"), elsewhere); err == nil || err.Error() != want {
		t.Errorf("CheckExpected(other repository) = %v, want %q", err, want)
	}
}

func TestUnchangedSince(t *testing.T) {
	expected := bindingFor("proj-1", "wf-1", "2026-09-29T10:00:00Z")
	tests := []struct {
		name   string
		loaded projection.Loaded
		inErr  []string // nil: unchanged
	}{
		{"the same binding", owned(expected), nil},
		{"another workflow", owned(bindingFor("proj-1", "wf-2", "2026-09-30T10:00:00Z")), []string{`workflow "wf-2"`, `project "proj-1"`, "2026-09-30T10:00:00Z"}},
		{"the same workflow bound again", owned(bindingFor("proj-1", "wf-1", "2026-09-30T10:00:00Z")), []string{"2026-09-30T10:00:00Z"}},
		{"removed", stateOf(projection.ClassificationAbsent, ""), []string{"it was removed"}},
		{"a file that is not ours", stateOf(projection.ClassificationForeign, "someone else's"), []string{"it is now foreign: someone else's"}},
		{"a malformed file", stateOf(projection.ClassificationMalformed, "garbled"), []string{"it is now malformed: garbled"}},
		{"an unreadable state", stateOf(projection.ClassificationUnavailable, "permission denied"), []string{"it is now unavailable: permission denied"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := projection.UnchangedSince(tt.loaded, expected)
			if tt.inErr == nil {
				requireError(t, err, nil, nil)
				return
			}
			requireError(t, err, projection.ErrBindingChanged, tt.inErr)
		})
	}
}

func TestAdmitReplace(t *testing.T) {
	expected := bindingFor("proj-1", "wf-1", "2026-09-29T10:00:00Z")
	next := bindingFor("proj-2", "wf-9", "2026-09-30T10:00:00Z")

	if write, err := projection.AdmitReplace(owned(expected), expected, next); !write || err != nil {
		t.Errorf("AdmitReplace(unchanged) = %v, %v, want a write", write, err)
	}
	// The binding is what the caller read and already names the target: nothing to
	// rewrite, and the original bound_at is kept.
	again := bindingFor("proj-1", "wf-1", "2026-09-30T10:00:00Z")
	if write, err := projection.AdmitReplace(owned(expected), expected, again); write || err != nil {
		t.Errorf("AdmitReplace(unchanged, same target) = %v, %v, want a no-op", write, err)
	}
	if write, err := projection.AdmitReplace(owned(next), expected, next); write || !errors.Is(err, projection.ErrBindingChanged) {
		t.Errorf("AdmitReplace(changed) = %v, %v, want no write and ErrBindingChanged", write, err)
	}
	if write, err := projection.AdmitReplace(stateOf(projection.ClassificationAbsent, ""), expected, next); write || !errors.Is(err, projection.ErrBindingChanged) {
		t.Errorf("AdmitReplace(removed) = %v, %v, want no write and ErrBindingChanged", write, err)
	}
}

func TestAdmitUnbind(t *testing.T) {
	tests := []struct {
		name    string
		loaded  projection.Loaded
		remove  bool
		wantErr error
	}{
		{"an absent binding is already unbound", stateOf(projection.ClassificationAbsent, ""), false, nil},
		{"an owned binding is removed", owned(bindingFor("proj-1", "wf-1", "2026-09-29T10:00:00Z")), true, nil},
		{"a foreign file is refused", stateOf(projection.ClassificationForeign, "x"), false, projection.ErrRefuseForeignBinding},
		{"a malformed file is refused", stateOf(projection.ClassificationMalformed, "x"), false, projection.ErrRefuseMalformedBinding},
		{"an unavailable state is refused", stateOf(projection.ClassificationUnavailable, "x"), false, projection.ErrBindingUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remove, err := projection.AdmitUnbind(tt.loaded)
			if remove != tt.remove {
				t.Errorf("AdmitUnbind() remove = %v, want %v", remove, tt.remove)
			}
			requireError(t, err, tt.wantErr, nil)
		})
	}
}

func TestAdmitRemoval(t *testing.T) {
	expected := bindingFor("proj-1", "wf-1", "2026-09-29T10:00:00Z")

	if remove, err := projection.AdmitRemoval(owned(expected), expected); !remove || err != nil {
		t.Errorf("AdmitRemoval(unchanged) = %v, %v, want a removal", remove, err)
	}
	// Gone is the state the caller wanted: nothing to remove and nothing wrong.
	if remove, err := projection.AdmitRemoval(stateOf(projection.ClassificationAbsent, ""), expected); remove || err != nil {
		t.Errorf("AdmitRemoval(gone) = %v, %v, want no removal and no error", remove, err)
	}
	for _, loaded := range []projection.Loaded{
		owned(bindingFor("proj-2", "wf-9", "2026-09-30T10:00:00Z")),
		stateOf(projection.ClassificationForeign, "x"),
		stateOf(projection.ClassificationMalformed, "x"),
		stateOf(projection.ClassificationUnavailable, "x"),
	} {
		if remove, err := projection.AdmitRemoval(loaded, expected); remove || !errors.Is(err, projection.ErrBindingChanged) {
			t.Errorf("AdmitRemoval(%s) = %v, %v, want no removal and ErrBindingChanged", loaded.Classification, remove, err)
		}
	}
}

// requireError checks err against a sentinel (nil: no error) and fragments of its
// message.
func requireError(t *testing.T, err, want error, fragments []string) {
	t.Helper()
	if want == nil && fragments == nil {
		if err != nil {
			t.Errorf("error = %v, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("error = nil, want %v %v", want, fragments)
	}
	if want != nil && !errors.Is(err, want) {
		t.Errorf("error = %v, want it to wrap %v", err, want)
	}
	for _, f := range fragments {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("error %q does not contain %q", err, f)
		}
	}
}
