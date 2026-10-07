package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

func TestRemoveTakesTheSkillOutOfTheRegistryAndTheManifestAndKeepsTheOthers(t *testing.T) {
	o := newOverlay(t)
	put(t, o.registry, registryYAML("existing", "foo", "other"))
	put(t, o.manifest, "# the skills of the overlay\n"+manifestOf("existing", "foo", "other")+"engine/tool.go custom\n")
	spy := newStagedSpy(nil)
	res, err := o.remove(spy, "foo")
	if err != nil || res.ID != "foo" {
		t.Fatalf("RemoveSkill = %+v, %v, want foo removed", res, err)
	}
	o.is(t, registryYAML("existing", "other"), "# the skills of the overlay\n"+manifestOf("existing", "other")+"engine/tool.go custom\n")
	if len(spy.ops) != 4 || spy.ops[2] != "rename "+o.manifest || spy.ops[3] != "rename "+o.registry {
		t.Errorf("the calls were %q, want both files staged and then the manifest and the registry committed", spy.ops)
	}
}

// Taking a skill out of the registry deletes no file of the skill: the source of foo is still
// where it was, and so is its approval record.
func TestRemoveDeletesNoFileOfTheSkill(t *testing.T) {
	o := newOverlay(t)
	put(t, o.registry, registryYAML("existing", "foo"))
	put(t, o.manifest, manifestOf("existing", "foo"))
	if _, err := o.remove(newStagedSpy(nil), "foo"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(o.skills, "foo", "SKILL.md"), skills.ApprovalRecordPath(o.skills, "foo")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("remove touched %s: %v", path, err)
		}
	}
}

func TestRemoveWritesNothingWhenItRefuses(t *testing.T) {
	var (
		registry  *RegistryError
		idMissing *IDRequiredError
		manifest  *ManifestReadError
	)
	for name, tc := range map[string]struct {
		input func(o overlay) RemoveInput
		check func(t *testing.T, err error)
	}{
		"no id": {
			func(o overlay) RemoveInput { return RemoveInput{RegistryPath: o.registry, ManifestPath: o.manifest} },
			func(t *testing.T, err error) {
				if !errors.As(err, &idMissing) || err.Error() != "skills remove requires an <id> argument" {
					t.Errorf("err = %v, want the id required", err)
				}
			},
		},
		"an id that is not registered": {
			func(o overlay) RemoveInput {
				return RemoveInput{RegistryPath: o.registry, ManifestPath: o.manifest, ID: "never-registered"}
			},
			func(t *testing.T, err error) {
				if err == nil || err.Error() != `id "never-registered": not found in registry` {
					t.Errorf("err = %v, want the id not found", err)
				}
			},
		},
		"a registry that is not there": {
			func(o overlay) RemoveInput {
				return RemoveInput{RegistryPath: filepath.Join(o.dir, "absent.yaml"), ManifestPath: o.manifest, ID: "existing"}
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &registry) || !registry.Unreadable() {
					t.Errorf("err = %v, want an unreadable registry", err)
				}
			},
		},
		"a manifest that is not there": {
			func(o overlay) RemoveInput {
				return RemoveInput{RegistryPath: o.registry, ManifestPath: filepath.Join(o.dir, "absent.manifest"), ID: "existing"}
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &manifest) {
					t.Errorf("err = %v, want the manifest unreadable", err)
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := newOverlay(t)
			spy := newStagedSpy(nil)
			_, err := RemoveSkill(RemovePorts{Registries: o.registries(), Files: os.ReadFile, Staged: spy}, tc.input(o))
			if err == nil {
				t.Fatal("RemoveSkill succeeded, want a refusal")
			}
			tc.check(t, err)
			if len(spy.ops) != 0 {
				t.Errorf("the writes were called (%q) before the refusal", spy.ops)
			}
			o.untouched(t)
		})
	}
}

// A registry the reader left fields out of cannot be written back whole: remove refuses it, and
// says what the reader left out, in the refusal and not also in a warning.
func TestRemoveRefusesARegistryTheReaderLeftFieldsOutOfAndSaysWhatItLeftOutOnce(t *testing.T) {
	o := newOverlay(t)
	put(t, o.registry, registryYAML("existing")+"unknownTopLevel: true\n")
	res, err := o.remove(newStagedSpy(nil), "existing")
	if err == nil || !strings.Contains(err.Error(), "the registry has fields this program does not read") {
		t.Fatalf("err = %v, want the registry refused", err)
	}
	if !strings.Contains(err.Error(), "unknownTopLevel") {
		t.Errorf("err = %v, want it to say which field the reader left out", err)
	}
	if res.UnreadWarning != "" {
		t.Errorf("the result warns %q of what the refusal already says", res.UnreadWarning)
	}
}

// A manifest that the removal would leave in disagreement with the registry is not written.
func TestRemoveRefusesWhenTheManifestWouldDisagreeWithTheRegistry(t *testing.T) {
	o := newOverlay(t)
	put(t, o.manifest, manifestOf("existing")+"ghost/SKILL.md custom\n")
	var diverge *DivergenceError
	if _, err := o.remove(newStagedSpy(nil), "existing"); !errors.As(err, &diverge) {
		t.Fatalf("err = %v, want the divergence the write would leave", err)
	}
	if got := read(t, o.manifest); got != manifestOf("existing")+"ghost/SKILL.md custom\n" {
		t.Errorf("the manifest is %q, want it as it was", got)
	}
}

func TestRemovePutsBackWhatItStagedWhenAStepFails(t *testing.T) {
	for name, spy := range map[string]func(o overlay) *stagedSpy{
		"staging the manifest":    func(overlay) *stagedSpy { return failing("writetemp", "", 1, errors.New("create temp: no room")) },
		"staging the registry":    func(overlay) *stagedSpy { return failing("writetemp", "", 2, errors.New("sync: disk gone")) },
		"committing the manifest": func(o overlay) *stagedSpy { return failing("rename", o.manifest, 0, errInjected) },
	} {
		t.Run(name, func(t *testing.T) {
			o := newOverlay(t)
			var staged *skills.StagedWriteError
			if _, err := o.remove(spy(o), "existing"); !errors.As(err, &staged) {
				t.Fatalf("err = %v, want a staged write refused", err)
			}
			o.untouched(t)
		})
	}
}
