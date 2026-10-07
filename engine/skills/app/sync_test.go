package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// syncOverlay is an overlay whose registry lists existing and other, and whose manifest lists
// existing and a ghost the registry does not know, with a line of its own that is no skill row.
func syncOverlay(t *testing.T) overlay {
	t.Helper()
	o := newOverlay(t)
	put(t, o.registry, registryYAML("existing", "other"))
	put(t, o.manifest, "# the overlay\nexisting/SKILL.md custom\nghost/SKILL.md custom\nengine/tool.go custom\n")
	return o
}

func TestSyncRegeneratesTheSkillRowsAndKeepsEveryOtherLine(t *testing.T) {
	o := syncOverlay(t)
	spy := newStagedSpy(nil)
	res, err := o.sync(spy)
	if err != nil || res.InSync {
		t.Fatalf("SyncManifest = %+v, %v, want the manifest written", res, err)
	}
	if got, want := read(t, o.manifest), "# the overlay\nexisting/SKILL.md custom\nother/SKILL.md custom\nengine/tool.go custom\n"; got != want {
		t.Errorf("the manifest is %q, want %q", got, want)
	}
	if len(res.Changes.Added) != 1 || len(res.Changes.Dropped) != 1 || len(res.Changes.Retagged) != 0 {
		t.Errorf("the changes were %+v, want one row added (other) and one dropped (ghost)", res.Changes)
	}
	// The registry is never modified, and the manifest is staged at the mode of the owner.
	if got := read(t, o.registry); got != registryYAML("existing", "other") {
		t.Errorf("the registry is %q, want it as it was", got)
	}
	if len(spy.ops) != 2 || spy.perms[0] != 0o600 || spy.ops[1] != "rename "+o.manifest {
		t.Errorf("the calls were %q at the modes %v, want one staging at 0600 and one commit of the manifest", spy.ops, spy.perms)
	}
}

func TestSyncWritesNothingWhenTheManifestAlreadyAgrees(t *testing.T) {
	o := newOverlay(t)
	spy := newStagedSpy(nil)
	res, err := o.sync(spy)
	if err != nil || !res.InSync {
		t.Fatalf("SyncManifest = %+v, %v, want it already in sync", res, err)
	}
	if len(spy.ops) != 0 {
		t.Errorf("the writes were called (%q) for a manifest that was right", spy.ops)
	}
}

func TestSyncRefusesWhatItCannotReadAndWritesNothing(t *testing.T) {
	var (
		registry *RegistryError
		manifest *ManifestReadError
	)
	o := syncOverlay(t)
	spy := newStagedSpy(nil)
	_, err := SyncManifest(SyncPorts{Registries: o.registries(), Files: os.ReadFile, Staged: spy},
		SyncInput{RegistryPath: filepath.Join(o.dir, "absent.yaml"), ManifestPath: o.manifest})
	if !errors.As(err, &registry) || !registry.Unreadable() {
		t.Errorf("err = %v, want an unreadable registry", err)
	}
	_, err = SyncManifest(SyncPorts{Registries: o.registries(), Files: os.ReadFile, Staged: spy},
		SyncInput{RegistryPath: o.registry, ManifestPath: filepath.Join(o.dir, "absent.manifest")})
	if !errors.As(err, &manifest) {
		t.Errorf("err = %v, want the manifest unreadable", err)
	}
	if len(spy.ops) != 0 {
		t.Errorf("the writes were called (%q) before the refusal", spy.ops)
	}
}

// The registry the reader left fields out of is read and used, as it is for every verb that does
// not write it back; the result says what was left out.
func TestSyncSaysWhatTheReaderLeftOutOfTheRegistryAndGoesOn(t *testing.T) {
	o := syncOverlay(t)
	put(t, o.registry, registryYAML("existing", "other")+"unknownTopLevel: true\n")
	res, err := o.sync(newStagedSpy(nil))
	if err != nil || res.UnreadWarning == "" {
		t.Fatalf("SyncManifest = %+v, %v, want it to go on and say what the reader left out", res, err)
	}
}

func TestSyncPutsBackWhatItStagedWhenAStepFails(t *testing.T) {
	for name, tc := range map[string]struct {
		spy  func(o overlay) *stagedSpy
		want string
	}{
		"staging": {
			func(overlay) *stagedSpy { return failing("writetemp", "", 1, errors.New("create temp: no room")) },
			"writing manifest: writeFileAtomic: create temp: no room",
		},
		"committing": {
			func(o overlay) *stagedSpy { return failing("rename", o.manifest, 0, errInjected) },
			"finalizing manifest: injected failure",
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := syncOverlay(t)
			_, err := o.sync(tc.spy(o))
			var staged *skills.StagedWriteError
			if !errors.As(err, &staged) || err.Error() != tc.want {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			o.noLitter(t)
			if got := read(t, o.manifest); got != "# the overlay\nexisting/SKILL.md custom\nghost/SKILL.md custom\nengine/tool.go custom\n" {
				t.Errorf("the manifest is %q after a failed write, want it as it was", got)
			}
		})
	}
}
