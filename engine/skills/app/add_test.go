package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// addedFoo is the entry add makes for foo: global, custom, installed for the three runtimes that
// have always been the default.
const addedFoo = "  - id: foo\n    path: foo\n    source:\n      type: custom\n" +
	"    install:\n      defaultScope: global\n      targets:\n        - claude\n        - opencode\n        - codex\n" +
	"    lifecycle:\n      updateStrategy: overlay-only\n"

func (o overlay) input(id string) AddInput {
	return AddInput{RegistryPath: o.registry, ManifestPath: o.manifest, SourceRoot: o.skills, ID: id}
}

func TestAddRegistersTheSkillInTheRegistryAndTheManifest(t *testing.T) {
	o := newOverlay(t)
	spy := newStagedSpy(nil)
	res, err := o.add(spy, "foo")
	if err != nil || res.ID != "foo" {
		t.Fatalf("AddSkill = %+v, %v, want foo added", res, err)
	}
	o.is(t, registryYAML("existing")+addedFoo, manifestOf("existing", "foo"))

	// Both files are staged, then the manifest is committed, then the registry (ADR-9), each at the
	// mode of the owner, in the directory of its destination.
	kinds := make([]string, len(spy.ops))
	for i, op := range spy.ops {
		kinds[i] = strings.Fields(op)[0]
	}
	if got, want := strings.Join(kinds, " "), "writetemp writetemp rename rename"; got != want {
		t.Errorf("the calls were %q, want %q", got, want)
	}
	if spy.ops[2] != "rename "+o.manifest || spy.ops[3] != "rename "+o.registry {
		t.Errorf("the commits were %q and %q, want the manifest and then the registry", spy.ops[2], spy.ops[3])
	}
	for _, p := range spy.perms {
		if p != 0o600 {
			t.Errorf("a file was staged at the mode %v, want 0600", p)
		}
	}
	for _, op := range spy.ops[:2] {
		if op != "writetemp "+o.dir+string(filepath.Separator) {
			t.Errorf("%q: a file is staged in the directory of its destination", op)
		}
	}
}

func TestAddMakesAnEntryExternalWhenItIsGivenARepoAndAnOptionalRef(t *testing.T) {
	for name, tc := range map[string]struct{ repo, ref, wantSource string }{
		"a repo":         {"https://example.test/foo.git", "", "      type: external\n      repo: https://example.test/foo.git\n"},
		"a repo and ref": {"https://example.test/foo.git", "v1", "      type: external\n      repo: https://example.test/foo.git\n      ref: v1\n"},
		"neither":        {"", "", "      type: custom\n"},
	} {
		t.Run(name, func(t *testing.T) {
			o := newOverlay(t)
			in := o.input("foo")
			in.Repo, in.Ref = tc.repo, tc.ref
			if _, err := AddSkill(o.addPorts(newStagedSpy(nil)), in); err != nil {
				t.Fatal(err)
			}
			if got := read(t, o.registry); !strings.Contains(got, "  - id: foo\n    path: foo\n    source:\n"+tc.wantSource+"    install:") {
				t.Errorf("the registry is %q, want the entry of foo with the source %q", got, tc.wantSource)
			}
		})
	}
}

func TestAddWritesNothingWhenItRefuses(t *testing.T) {
	var (
		registry   *RegistryError
		notFound   *SkillNotFoundError
		lint       *LintRefusal
		unapproved *NotApprovedError
		diverge    *DivergenceError
		manifest   *ManifestReadError
		idMissing  *IDRequiredError
	)
	removeRecord := func(t *testing.T, o overlay) {
		t.Helper()
		if err := os.Remove(skills.ApprovalRecordPath(o.skills, "foo")); err != nil {
			t.Fatal(err)
		}
	}
	for name, tc := range map[string]struct {
		prepare func(t *testing.T, o overlay) AddInput
		check   func(t *testing.T, err error)
	}{
		"no id": {
			func(t *testing.T, o overlay) AddInput { return AddInput{} },
			func(t *testing.T, err error) {
				if !errors.As(err, &idMissing) || err.Error() != "skills add requires an <id> argument" {
					t.Errorf("err = %v, want the id required", err)
				}
			},
		},
		"a ref with no repo": {
			func(t *testing.T, o overlay) AddInput { return AddInput{ID: "foo", Ref: "v1"} },
			func(t *testing.T, err error) {
				if !errors.Is(err, ErrRefWithoutRepo) {
					t.Errorf("err = %v, want ErrRefWithoutRepo", err)
				}
			},
		},
		"a registry that is not there": {
			func(t *testing.T, o overlay) AddInput {
				return AddInput{RegistryPath: filepath.Join(o.dir, "absent.yaml"), ID: "foo"}
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &registry) || !registry.Unreadable() {
					t.Errorf("err = %v, want an unreadable registry", err)
				}
			},
		},
		"an id that is no slug": {
			func(t *testing.T, o overlay) AddInput { return o.input("Foo") },
			func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), `id "Foo": invalid slug`) {
					t.Errorf("err = %v, want an invalid slug", err)
				}
			},
		},
		"an id that is registered": {
			func(t *testing.T, o overlay) AddInput { return o.input("existing") },
			func(t *testing.T, err error) {
				if err == nil || err.Error() != `id "existing": already registered` {
					t.Errorf("err = %v, want the id already registered", err)
				}
			},
		},
		"a skill with no SKILL.md": {
			func(t *testing.T, o overlay) AddInput {
				if err := os.Remove(filepath.Join(o.skills, "foo", "SKILL.md")); err != nil {
					t.Fatal(err)
				}
				return o.input("foo")
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &notFound) || notFound.ID != "foo" || !strings.Contains(err.Error(), `skill "foo": SKILL.md not found at`) {
					t.Errorf("err = %v, want the SKILL.md not found", err)
				}
			},
		},
		"a hard lint finding": {
			func(t *testing.T, o overlay) AddInput {
				put(t, filepath.Join(o.skills, "foo", "SKILL.md"), strings.Replace(skillFor("foo"), "  version: \"1.0\"\n", "", 1))
				approve(t, o.skills, "foo")
				return o.input("foo")
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &lint) || len(lint.Findings) == 0 || !strings.Contains(lint.Findings[0].Error(), "[lint:required-fields]") {
					t.Errorf("err = %v, want the hard findings of the lint", err)
				}
			},
		},
		"a skill nobody approved": {
			func(t *testing.T, o overlay) AddInput { removeRecord(t, o); return o.input("foo") },
			func(t *testing.T, err error) {
				if !errors.As(err, &unapproved) || !strings.Contains(unapproved.Detail, "labdrian skills approve --id foo") {
					t.Errorf("err = %v, want an approval refusal that names the fixing command", err)
				}
			},
		},
		"a skill changed after it was approved": {
			func(t *testing.T, o overlay) AddInput {
				put(t, filepath.Join(o.skills, "foo", "SKILL.md"), skillFor("foo")+"\nA line after the approval.\n")
				return o.input("foo")
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &unapproved) {
					t.Errorf("err = %v, want an approval refusal", err)
				}
			},
		},
		"a record that cannot be read": {
			func(t *testing.T, o overlay) AddInput {
				removeRecord(t, o)
				if err := os.Mkdir(skills.ApprovalRecordPath(o.skills, "foo"), 0o755); err != nil {
					t.Fatal(err)
				}
				return o.input("foo")
			},
			func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), `skill "foo": reading approval record`) {
					t.Errorf("err = %v, want the unreadable record refused, not taken for none", err)
				}
			},
		},
		"a manifest that lists a skill the registry does not": {
			func(t *testing.T, o overlay) AddInput {
				put(t, o.manifest, manifestOf("existing")+"ghost/SKILL.md custom\n")
				return o.input("foo")
			},
			func(t *testing.T, err error) {
				if !errors.As(err, &diverge) || len(diverge.Divergences) == 0 {
					t.Errorf("err = %v, want the divergences the write would leave", err)
				}
			},
		},
		"a manifest that cannot be read": {
			func(t *testing.T, o overlay) AddInput {
				in := o.input("foo")
				in.ManifestPath = filepath.Join(o.dir, "absent.manifest")
				return in
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
			in := tc.prepare(t, o)
			spy := newStagedSpy(nil)
			_, err := AddSkill(o.addPorts(spy), in)
			if err == nil {
				t.Fatal("AddSkill succeeded, want a refusal")
			}
			tc.check(t, err)
			if len(spy.ops) != 0 {
				t.Errorf("the writes were called (%q) before the refusal", spy.ops)
			}
			o.noLitter(t)
		})
	}
}

// A hard finding of the lint is refused before the approval is looked at: the verb says what is
// wrong with the file, not what is missing next to it.
func TestAddTellsALintFindingBeforeAMissingApproval(t *testing.T) {
	o := newOverlay(t)
	put(t, filepath.Join(o.skills, "foo", "SKILL.md"), strings.Replace(skillFor("foo"), "  version: \"1.0\"\n", "", 1))
	if err := os.Remove(skills.ApprovalRecordPath(o.skills, "foo")); err != nil {
		t.Fatal(err)
	}
	var lint *LintRefusal
	if _, err := o.add(newStagedSpy(nil), "foo"); !errors.As(err, &lint) {
		t.Fatalf("err = %v, want the lint refusal", err)
	}
}

// A warning of the lint is advisory and does not stop the add.
func TestAddGoesOnPastAnAdvisoryLintWarning(t *testing.T) {
	o := newOverlay(t)
	path := filepath.Join(o.skills, "foo", "SKILL.md")
	put(t, path, strings.Replace(skillFor("foo"),
		"Load this skill for its documented procedure.", "Load this skill from /home/example/fixture for its documented procedure.", 1))
	approve(t, o.skills, "foo")
	if hard, warnings := skills.LintSkillFile([]byte(read(t, path))); len(hard) != 0 || len(warnings) == 0 {
		t.Fatalf("the fixture has %d hard findings and %d warnings, want warnings only", len(hard), len(warnings))
	}
	if _, err := o.add(newStagedSpy(nil), "foo"); err != nil {
		t.Fatalf("AddSkill = %v, want the warning not to stop it", err)
	}
}

// A registry the reader left fields out of cannot be written back whole, so add refuses it, and
// the result says what the reader left out even then: the CLI tells it before the refusal.
func TestAddSaysWhatTheReaderLeftOutOfTheRegistryEvenWhenItRefuses(t *testing.T) {
	o := newOverlay(t)
	put(t, o.registry, registryYAML("existing")+"unknownTopLevel: true\n")
	res, err := o.add(newStagedSpy(nil), "foo")
	if err == nil || !strings.Contains(err.Error(), "the registry has fields this program does not read") {
		t.Fatalf("err = %v, want the registry refused: writing it back would drop what was left out", err)
	}
	if res.UnreadWarning == "" {
		t.Error("the result does not say what the reader left out")
	}
}

func TestAddPutsBackWhatItStagedWhenAStepFails(t *testing.T) {
	for name, tc := range map[string]struct {
		spy      func(o overlay) *stagedSpy
		wantErr  string
		manifest string // "same" or "new"
		registry string
	}{
		"staging the manifest": {
			func(overlay) *stagedSpy { return failing("writetemp", "", 1, errors.New("create temp: no room")) },
			"writing manifest: writeFileAtomic: create temp: no room", "same", "same",
		},
		"staging the registry": {
			func(overlay) *stagedSpy { return failing("writetemp", "", 2, errors.New("sync: disk gone")) },
			"writing registry: writeFileAtomic: sync: disk gone", "same", "same",
		},
		"committing the manifest": {
			func(o overlay) *stagedSpy { return failing("rename", o.manifest, 0, errInjected) },
			"finalizing manifest: injected failure", "same", "same",
		},
		// The two files are renamed one after the other: a failure of the second leaves the manifest
		// as it was written and the registry as it was, and the temporary file of the registry is
		// removed. The registry and the manifest are not committed as one.
		"committing the registry": {
			func(o overlay) *stagedSpy { return failing("rename", o.registry, 0, errInjected) },
			"finalizing registry: injected failure", "new", "same",
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := newOverlay(t)
			_, err := o.add(tc.spy(o), "foo")
			var staged *skills.StagedWriteError
			if !errors.As(err, &staged) || err.Error() != tc.wantErr {
				t.Errorf("err = %v, want %q", err, tc.wantErr)
			}
			if got := read(t, o.manifest); (got == manifestOf("existing")) != (tc.manifest == "same") {
				t.Errorf("the manifest is %q, want it %s", got, tc.manifest)
			}
			if got := read(t, o.registry); (got == registryYAML("existing")) != (tc.registry == "same") {
				t.Errorf("the registry is %q, want it %s", got, tc.registry)
			}
			o.noLitter(t)
		})
	}
}

// What the store writes must read back as the registry that was meant: a registry that does not is
// never staged.
func TestAddRefusesARegistryThatTheStoreDoesNotReadBackAsItWasWritten(t *testing.T) {
	o := newOverlay(t)
	for name, tc := range map[string]struct {
		repo skills.RegistryRepository
		want string
	}{
		"cannot encode":  {brokenStore{encodeErr: errors.New("no room in the codec"), inner: o.registries()}, "serializing registry: no room in the codec"},
		"cannot decode":  {brokenStore{decodeErr: errors.New("garbled"), inner: o.registries()}, "validate-before-write re-parse failed: garbled"},
		"reads it wrong": {brokenStore{drop: true, inner: o.registries()}, "validate-before-write: serialize→parse round-trip mismatch"},
	} {
		t.Run(name, func(t *testing.T) {
			spy := newStagedSpy(nil)
			ports := o.addPorts(spy)
			ports.Registries = tc.repo
			_, err := AddSkill(ports, o.input("foo"))
			if err == nil || err.Error() != tc.want {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			if len(spy.ops) != 0 {
				t.Errorf("the writes were called (%q) for a registry that does not read back", spy.ops)
			}
		})
	}
}

// brokenStore is a registry repository whose codec fails, or reads back less than was written.
type brokenStore struct {
	inner     skills.RegistryRepository
	encodeErr error
	decodeErr error
	drop      bool
}

func (b brokenStore) Load(location string) (skills.Registry, error) { return b.inner.Load(location) }

func (b brokenStore) Encode(reg skills.Registry) ([]byte, error) {
	if b.encodeErr != nil {
		return nil, b.encodeErr
	}
	return b.inner.Encode(reg)
}

func (b brokenStore) Decode(data []byte) (skills.Registry, error) {
	if b.decodeErr != nil {
		return skills.Registry{}, b.decodeErr
	}
	reg, err := b.inner.Decode(data)
	if b.drop && len(reg.Skills) > 0 {
		reg.Skills = reg.Skills[:len(reg.Skills)-1]
	}
	return reg, err
}

// legacyBaseline grandfathers the skill id with the exact bytes it has now, as the fixed baseline
// grandfathers the global skills that predate the approval record.
func legacyBaseline(t *testing.T, o overlay, id string) skills.BaselineLookup {
	t.Helper()
	digest := skills.SkillDigest([]byte(read(t, filepath.Join(o.skills, id, "SKILL.md"))))
	return func(candidate string) (string, bool) { return digest, candidate == id }
}

// A skill of the baseline needs no approval record while its bytes are the pinned ones: it is
// registered with no record on disk, and the record is not written by add. The same rule as
// validate, which counts it as grandfathered; the two cannot disagree.
func TestAddRegistersAGrandfatheredSkillWithNoApprovalRecord(t *testing.T) {
	o := newOverlay(t)
	put(t, filepath.Join(o.skills, "legacy", "SKILL.md"), skillFor("legacy"))
	ports := o.addPorts(newStagedSpy(nil))
	ports.Baseline = legacyBaseline(t, o, "legacy")

	res, err := AddSkill(ports, o.input("legacy"))

	if err != nil || res.ID != "legacy" {
		t.Fatalf("AddSkill = %+v, %v, want legacy added with no approval record", res, err)
	}
	if _, err := os.Stat(skills.ApprovalRecordPath(o.skills, "legacy")); err == nil {
		t.Error("add wrote an approval record for a grandfathered skill")
	}
	o.is(t, registryYAML("existing")+strings.ReplaceAll(addedFoo, "foo", "legacy"), manifestOf("existing", "legacy"))
}

// The exemption is for the pinned bytes alone: one byte more and the skill needs a record like any
// other, and the refusal says the file differs from the baseline.
func TestAddRefusesAGrandfatheredSkillWhoseBytesChanged(t *testing.T) {
	o := newOverlay(t)
	put(t, filepath.Join(o.skills, "legacy", "SKILL.md"), skillFor("legacy"))
	baseline := legacyBaseline(t, o, "legacy")
	put(t, filepath.Join(o.skills, "legacy", "SKILL.md"), skillFor("legacy")+"\n")
	ports := o.addPorts(newStagedSpy(nil))
	ports.Baseline = baseline

	_, err := AddSkill(ports, o.input("legacy"))

	var refusal *NotApprovedError
	if !errors.As(err, &refusal) || refusal.Class != skills.DivApprovalMissing || !strings.Contains(refusal.Detail, "differs from the grandfathered baseline") {
		t.Fatalf("err = %v, want a *NotApprovedError saying the bytes differ from the baseline", err)
	}
	o.untouched(t)
}

// A skill that is not in the baseline needs its record whatever the baseline says of others.
func TestAddRefusesASkillTheBaselineDoesNotNameWhenItHasNoRecord(t *testing.T) {
	o := newOverlay(t)
	put(t, filepath.Join(o.skills, "newcomer", "SKILL.md"), skillFor("newcomer"))
	ports := o.addPorts(newStagedSpy(nil))
	ports.Baseline = legacyBaseline(t, o, "foo")

	_, err := AddSkill(ports, o.input("newcomer"))

	var refusal *NotApprovedError
	if !errors.As(err, &refusal) || refusal.Class != skills.DivApprovalMissing {
		t.Fatalf("err = %v, want a *NotApprovedError", err)
	}
	o.untouched(t)
}
