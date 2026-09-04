package register

import (
	"os"
	"path/filepath"
	"testing"
)

// errormessage_test.go's assertHelperErrorAttribution claims to pin the
// convention on "the install-state file helpers" as well as on the
// format-level ones. It did not: no call site passed a LoadInstallState or
// Save error, so reverting installstate.go's own prefixes left this
// package green and the claim was documentation of an intention.
//
// LoadInstallState and Save are shared verbatim by both directions -- every
// register AND every unregister loads this file and most of them save it --
// so they are exactly the shape the convention is about: they must name the
// FILE and the FAILURE and leave the command word to their one caller.
func TestLoadInstallState_ErrorNamesTheFileAndNoCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, installStateFileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt install state: %v", err)
	}

	_, err := LoadInstallState(path)
	assertHelperErrorAttribution(t, err, path, "parse")
}

func TestInstallStateSave_ErrorNamesTheFileAndNoCommand(t *testing.T) {
	// A path whose parent is a FILE, not a directory: Save cannot create
	// the directory it needs, which is the failure an operator meets on a
	// state directory that is not what it was expected to be.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	path := filepath.Join(blocker, "sub", installStateFileName)

	state := &InstallState{}
	state.Set("claude", TargetRecord{Fingerprint: "deadbeef"})
	assertHelperErrorAttribution(t, state.Save(path), path)
}
