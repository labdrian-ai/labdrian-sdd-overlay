package shaper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// LoadHandoff parses what a ContainedSource returns; reading the file is the source's.
// These tests give it a fake source, so they prove what the domain decides on its own. The
// read of real files is proved against the file-backed source, in engine/shaper/fsadapter,
// through this same function.

// assertLoadHandoffRejected fails unless LoadHandoff returned an error
// together with the zero HandoffSource, so no rejection leaks partial state.
func assertLoadHandoffRejected(t *testing.T, got HandoffSource, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("LoadHandoff accepted %s", what)
	}
	if !reflect.DeepEqual(got, HandoffSource{}) {
		t.Errorf("LoadHandoff returned a partial HandoffSource alongside error %v: %#v", err, got)
	}
}

func TestLoadHandoffReturnsParsedHandoffRawBytesAndDigest(t *testing.T) {
	data := documentWith(t, nil)
	src := &fakeSource{files: map[string][]byte{"shaper/handoff.json": data}}

	got, err := LoadHandoff(src, sourceRoot, "./shaper/handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff: %v", err)
	}
	if got.SourcePath != "shaper/handoff.json" {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, "shaper/handoff.json")
	}
	if string(got.Bytes) != string(data) {
		t.Errorf("Bytes = %q, want %q", got.Bytes, data)
	}
	sum := sha256.Sum256(data)
	if want := hex.EncodeToString(sum[:]); got.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", got.SHA256, want)
	}
	want, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(got.Handoff, want) {
		t.Errorf("Handoff = %#v, want %#v", got.Handoff, want)
	}
}

// The source is asked for the cleaned path, under the root it was given, and told what the
// bytes are for.
func TestLoadHandoffAsksTheSourceForTheCleanedPathUnderItsLabel(t *testing.T) {
	src := &fakeSource{files: map[string][]byte{"h/handoff.json": documentWith(t, nil)}}

	if _, err := LoadHandoff(src, sourceRoot, "h/./x/../handoff.json"); err != nil {
		t.Fatalf("LoadHandoff: %v", err)
	}
	want := []readCall{{root: sourceRoot, rel: "h/handoff.json", label: "handoff source"}}
	if !reflect.DeepEqual(src.calls, want) {
		t.Errorf("source was asked %+v, want exactly %+v", src.calls, want)
	}
}

func TestLoadHandoffSHA256ChangesOnWhitespaceOnlyEdit(t *testing.T) {
	data := documentWith(t, nil)
	before, err := LoadHandoff(&fakeSource{files: map[string][]byte{"handoff.json": data}}, sourceRoot, "handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff before edit: %v", err)
	}

	edited := append(append([]byte{}, data...), ' ', '\n')
	after, err := LoadHandoff(&fakeSource{files: map[string][]byte{"handoff.json": edited}}, sourceRoot, "handoff.json")
	if err != nil {
		t.Fatalf("LoadHandoff after edit: %v", err)
	}
	if !reflect.DeepEqual(before.Handoff, after.Handoff) {
		t.Fatalf("whitespace-only edit changed the parsed Handoff")
	}
	if before.SHA256 == after.SHA256 {
		t.Errorf("SHA256 did not change on a whitespace-only edit: %q", before.SHA256)
	}
}

// What the source refuses is reported, with the operation named, and nothing is loaded.
func TestLoadHandoffReportsWhatTheSourceRefusesWithoutAPartialSource(t *testing.T) {
	refusal := errors.New(`handoff source "handoff.json" resolves outside the worktree root`)
	got, err := LoadHandoff(&fakeSource{err: refusal}, sourceRoot, "handoff.json")
	assertLoadHandoffRejected(t, got, err, "a handoff the source refused")
	if want := "load handoff: " + refusal.Error(); err.Error() != want {
		t.Errorf("LoadHandoff error = %q, want %q", err, want)
	}
	if !errors.Is(err, refusal) {
		t.Errorf("LoadHandoff error %v does not wrap the source's", err)
	}
}

// Bytes the source read but that are not a handoff are refused by the strict parse, in the
// parse's words and under the same prefix.
func TestLoadHandoffRejectsBytesThatAreNotAHandoff(t *testing.T) {
	src := &fakeSource{files: map[string][]byte{"handoff.json": documentWith(t, map[string]any{"version": 9})}}
	got, err := LoadHandoff(src, sourceRoot, "handoff.json")
	assertLoadHandoffRejected(t, got, err, "an invalid handoff")
	if want := "load handoff: parse shaper handoff"; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("LoadHandoff error = %v, want it to start with %q", err, want)
	}
}
