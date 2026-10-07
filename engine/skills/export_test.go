package skills

// The tests of this package read registries through the YAML adapter, as the program does. The
// adapter (engine/skills/registryyaml) imports this package, so a test file of package skills
// cannot import it: Go refuses an import cycle in a test. The way round is the one this file and
// registryyaml_hook_test.go make together. That file is in the external test package
// skills_test, which may import the adapter, and registers it here when the test binary starts;
// the helpers below hand it to the tests of this package. If it was not registered, every test
// that needs a registry fails at once and says why.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// yamlRepositoryOf builds the repository of the YAML adapter that reads through read.
var yamlRepositoryOf func(read func(path string) ([]byte, error)) RegistryRepository

// UseYAMLRegistries is how registryyaml_hook_test.go registers the adapter.
func UseYAMLRegistries(of func(read func(path string) ([]byte, error)) RegistryRepository) {
	yamlRepositoryOf = of
}

// testRegistries is the registry repository of a test: the YAML adapter, reading files through
// read (os.ReadFile, or the in-memory reader of the test).
func testRegistries(read FileReader) RegistryRepository {
	if yamlRepositoryOf == nil {
		panic("skills tests: the YAML adapter was not registered; see registryyaml_hook_test.go")
	}
	return yamlRepositoryOf(read)
}

// parseRegistry reads a registry from the bytes of its YAML file, as the program does: decoded by
// the adapter and judged by the domain.
func parseRegistry(data []byte) (Registry, error) {
	return DecodeRegistry(testRegistries(nil), data)
}

// mustParseRegistry is parseRegistry for a registry that is known to be fine.
func mustParseRegistry(t *testing.T, yaml string) Registry {
	t.Helper()
	reg, err := parseRegistry([]byte(yaml))
	if err != nil {
		t.Fatalf("the registry does not read: %v\n%s", err, yaml)
	}
	return reg
}

// serializeRegistry is the YAML file of a registry, as the adapter writes it.
func serializeRegistry(reg Registry) ([]byte, error) {
	return testRegistries(nil).Encode(reg)
}

// The file system adapter (engine/skills/skillsfs) imports this package too, so the tests of
// this package reach it the way they reach the YAML adapter: skillsfs_hook_test.go, in the
// external test package, registers it here when the test binary starts. The helpers below are
// the verbs and the functions of this package as the program wires them, so that a test that
// needs a real directory says nothing of how it is walked.

// osTree is the SkillTree of the real file system, registered by skillsfs_hook_test.go.
var osTree SkillTree

// UseOSTree is how skillsfs_hook_test.go registers the adapter.
func UseOSTree(tree SkillTree) { osTree = tree }

// testTree is the tree of a test: the file system adapter.
func testTree() SkillTree {
	if osTree == nil {
		panic("skills tests: the file system adapter was not registered; see skillsfs_hook_test.go")
	}
	return osTree
}

// osProject is the ProjectFS of the real file system, registered by skillsfs_hook_test.go.
var osProject ProjectFS

// UseOSProject is how skillsfs_hook_test.go registers the adapter.
func UseOSProject(project ProjectFS) { osProject = project }

// testProjectFS is the project file system of a test: the file system adapter.
func testProjectFS() ProjectFS {
	if osProject == nil {
		panic("skills tests: the file system adapter was not registered; see skillsfs_hook_test.go")
	}
	return osProject
}

// osExists gives a fake locker the answer of the file system to 'is this path there'.
type osExists struct{}

func (osExists) Exists(path string) error {
	_, err := os.Stat(path)
	return err
}

// scanSkillFiles lists the files of a skills tree on disk, as the program does.
func scanSkillFiles(dir string) ([]string, error) { return testTree().ScanSkillFiles(dir) }

// validateFile compares a registry with the manifest at path, as 'skills validate' does.
func validateFile(reg Registry, path string) ([]Divergence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ValidateAgainstManifest(reg, data)
}

// loadManifestViewFile parses the manifest at path.
func loadManifestViewFile(path string) (ManifestView, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadManifestViewReader(bytes.NewReader(data))
}

// fileApprovals is the ApprovalRecordStore of a test: the SKILL.md and the record are read from
// the paths the domain names (SkillMDPath, ApprovalRecordPath) through read, which is os.ReadFile
// or the in-memory reader of the test, so a reader that gates or counts reads sees these too.
func fileApprovals(read FileReader) ApprovalRecordStore { return readerApprovals{read} }

type readerApprovals struct{ read FileReader }

func (r readerApprovals) ReadSkill(sourceRoot, path string) ([]byte, error) {
	return r.read(SkillMDPath(sourceRoot, path))
}

func (r readerApprovals) ReadRecord(sourceRoot, id string) ([]byte, error) {
	return r.read(ApprovalRecordPath(sourceRoot, id))
}

// writeTestFile writes content to path, making the directories above it.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
