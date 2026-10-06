package app

// The fixture of the tests of the verbs that write an overlay: a real directory in a temporary
// place, the YAML adapter of the registry and the file system adapter, and a spy between the use
// case and the staged writes that can fail one call. What the adapters do with a real disk is
// tested in their own packages; what the program leaves on disk is pinned by the golden files of
// engine/cmd.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

const approvedAt = "2026-09-30T12:00:00Z"

func fixedClock(ts string) func() string { return func() string { return ts } }

// registryYAML is the registry file of the given ids: global custom skills for claude.
func registryYAML(ids ...string) string {
	var sb strings.Builder
	sb.WriteString("version: \"1\"\nskills:\n")
	for _, id := range ids {
		sb.WriteString("  - id: " + id + "\n    path: " + id + "\n    source:\n      type: custom\n")
		sb.WriteString("    install:\n      defaultScope: global\n      targets:\n        - claude\n")
		sb.WriteString("    lifecycle:\n      updateStrategy: overlay-only\n")
	}
	return sb.String()
}

// manifestOf is a manifest that lists the SKILL.md of each id.
func manifestOf(ids ...string) string {
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteString(id + "/SKILL.md custom\n")
	}
	return sb.String()
}

// skillFor is the smallest SKILL.md that passes the hard lint.
func skillFor(id string) string {
	return "---\nname: " + id + "\ndescription: A concise procedural skill for " + id + ".\nlicense: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n---\n" +
		"## Activation Contract\nLoad this skill for its documented procedure.\n\n" +
		"## Hard Rules\n- Keep the procedure explicit.\n\n" +
		"## Execution Steps\n1. Follow the procedure.\n"
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// approve writes the record a human's `skills approve` leaves for the SKILL.md now on disk.
func approve(t *testing.T, root, id string) {
	t.Helper()
	data, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{
		Skill: id, SHA256: skills.SkillDigest([]byte(read(t, filepath.Join(root, id, "SKILL.md")))),
		ApprovedAt: approvedAt, Approver: "fixture-reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	put(t, skills.ApprovalRecordPath(root, id), string(data))
}

// overlay is a directory that holds a registry and a manifest that agree on one skill, existing,
// and the source of that skill and of foo, a second one that add can register; both are approved.
type overlay struct {
	dir, registry, manifest, skills string
}

func newOverlay(t *testing.T) overlay {
	t.Helper()
	dir := t.TempDir()
	o := overlay{dir: dir, registry: filepath.Join(dir, "registry.yaml"), manifest: filepath.Join(dir, "overlay.manifest"), skills: filepath.Join(dir, "skills")}
	put(t, o.registry, registryYAML("existing"))
	put(t, o.manifest, manifestOf("existing"))
	for _, id := range []string{"existing", "foo"} {
		put(t, filepath.Join(o.skills, id, "SKILL.md"), skillFor(id))
		approve(t, o.skills, id)
	}
	return o
}

func (o overlay) registries() skills.RegistryRepository {
	return registryyaml.NewRepository(os.ReadFile)
}

func (o overlay) addPorts(staged skills.StagedWrites) AddPorts {
	return AddPorts{Registries: o.registries(), Files: os.ReadFile, Stat: os.Stat, Approvals: skillsfs.Approvals{}, Staged: staged}
}

func (o overlay) add(staged skills.StagedWrites, id string) (AddResult, error) {
	return AddSkill(o.addPorts(staged), AddInput{RegistryPath: o.registry, ManifestPath: o.manifest, SourceRoot: o.skills, ID: id})
}

func (o overlay) remove(staged skills.StagedWrites, id string) (RemoveResult, error) {
	return RemoveSkill(RemovePorts{Registries: o.registries(), Files: os.ReadFile, Staged: staged},
		RemoveInput{RegistryPath: o.registry, ManifestPath: o.manifest, ID: id})
}

func (o overlay) sync(staged skills.StagedWrites) (SyncResult, error) {
	return SyncManifest(SyncPorts{Registries: o.registries(), Files: os.ReadFile, Staged: staged},
		SyncInput{RegistryPath: o.registry, ManifestPath: o.manifest})
}

// untouched says the registry and the manifest are as the fixture made them, and that no
// temporary file is left in the directory.
func (o overlay) untouched(t *testing.T) {
	t.Helper()
	o.is(t, registryYAML("existing"), manifestOf("existing"))
}

func (o overlay) is(t *testing.T, wantRegistry, wantManifest string) {
	t.Helper()
	if got := read(t, o.registry); got != wantRegistry {
		t.Errorf("the registry is %q, want %q", got, wantRegistry)
	}
	if got := read(t, o.manifest); got != wantManifest {
		t.Errorf("the manifest is %q, want %q", got, wantManifest)
	}
	o.noLitter(t)
}

func (o overlay) noLitter(t *testing.T) {
	t.Helper()
	entries, _ := os.ReadDir(o.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-skills-") {
			t.Errorf("the temporary file %s was left behind", e.Name())
		}
	}
}

// stagedSpy records every call to the writes it wraps and can turn one into an error.
type stagedSpy struct {
	real  skills.StagedWrites
	fail  func(op, path string, n int) error // n counts the calls of this op so far, from 1
	ops   []string
	perms []fs.FileMode
	n     map[string]int
}

func newStagedSpy(fail func(op, path string, n int) error) *stagedSpy {
	return &stagedSpy{real: skillsfs.Project{}, fail: fail, n: map[string]int{}}
}

func (s *stagedSpy) check(op, path string) error {
	s.n[op]++
	s.ops = append(s.ops, op+" "+path)
	if s.fail != nil {
		return s.fail(op, path, s.n[op])
	}
	return nil
}

func (s *stagedSpy) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	s.perms = append(s.perms, perm)
	if err := s.check("writetemp", dir); err != nil {
		return "", err
	}
	return s.real.WriteTemp(dir, data, perm)
}

func (s *stagedSpy) Rename(oldPath, newPath string) error {
	if err := s.check("rename", newPath); err != nil {
		return err
	}
	return s.real.Rename(oldPath, newPath)
}

func (s *stagedSpy) Remove(name string) error {
	if err := s.check("remove", name); err != nil {
		return err
	}
	return s.real.Remove(name)
}

var errInjected = errors.New("injected failure")

// failing is a spy that fails the call of op that is the nth (the one that renames path, when path
// is not empty) with err.
func failing(op, path string, nth int, err error) *stagedSpy {
	return newStagedSpy(func(o, p string, n int) error {
		if o == op && (path == "" || p == path) && (path != "" || n == nth) {
			return err
		}
		return nil
	})
}

func skillsApprovals() skills.ApprovalRecordStore { return skillsfs.Approvals{} }
