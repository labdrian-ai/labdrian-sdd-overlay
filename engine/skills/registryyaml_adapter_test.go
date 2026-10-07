package skills_test

// The tests of the domain that need the YAML file of a registry and the file system adapter, and so
// cannot sit in package skills: the adapters import it, and Go refuses an import cycle in a test.
// They are in the external test package, which imports the adapters as the program does and hands
// them to nothing: no test of package skills reaches an adapter through a hook or a package
// variable.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// repository is the registry repository of the YAML adapter, as the program builds it.
func repository() skills.RegistryRepository { return registryyaml.NewRepository(os.ReadFile) }

// encode is the YAML file of a registry, as the adapter writes it.
func encode(reg skills.Registry) ([]byte, error) { return repository().Encode(reg) }

// decode reads a registry from the bytes of its YAML file, as the program does: decoded by the
// adapter and judged by the domain.
func decode(data []byte) (skills.Registry, error) { return skills.DecodeRegistry(repository(), data) }

// repoRoot is the root of this repository: the tests run in engine/skills.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving the repository root: %v", err)
	}
	return root
}

// TestAddEntryNilAllowedProjects verifies ADR-8: AllowedProjects MUST be nil (not
// []string{}) so the round-trip through encode → decode produces DeepEqual.
func TestAddEntryNilAllowedProjects(t *testing.T) {
	reg := skills.Registry{Version: "1"}

	got, err := skills.AddEntry(reg, "skill1", "", "")
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	if got.Skills[0].Install.AllowedProjects != nil {
		t.Errorf("AllowedProjects: expected nil, got %v", got.Skills[0].Install.AllowedProjects)
	}

	// Round-trip through serialize → parse must preserve DeepEqual (ADR-8, ADR-7).
	out, err := encode(got)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	reparsed, err := decode(out)
	if err != nil {
		t.Fatalf("decoding the encoded registry: %v", err)
	}
	if !reflect.DeepEqual(got, reparsed) {
		t.Errorf("round-trip not equal:\n  before: %+v\n  after:  %+v", got, reparsed)
	}
}

// TestAddEntryRoundTrip verifies that an entry added to a registry survives encoding and decoding:
// what the adapter writes reads back to a DeepEqual value.
func TestAddEntryRoundTrip(t *testing.T) {
	base := skills.Registry{
		Version: "1",
		Skills: []skills.Entry{
			{
				ID:   "base",
				Path: "base",
				Source: skills.Source{
					Type:     "core",
					Upstream: &skills.Upstream{Owner: "gentleman-programming"},
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "vendor-merge"},
			},
		},
	}

	got, err := skills.AddEntry(base, "new-skill", "", "")
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	out, err := encode(got)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	reparsed, err := decode(out)
	if err != nil {
		t.Fatalf("decoding after encoding: %v", err)
	}

	if !reflect.DeepEqual(got, reparsed) {
		t.Errorf("round-trip failed:\n  want: %+v\n  got:  %+v", got, reparsed)
	}
}

// TestApprovalBaseline_PinnedToTheRepositoryRegistry pins the baseline against this repository's
// real registry and real SKILL.md files, read-only.
//
// At the Phase 8 base every registered skill was global and unapproved, and the baseline
// grandfathers exactly those 37. So on the repository as it is:
//   - every baseline id is a registered global skill;
//   - every registered global skill is either grandfathered at its pinned digest or carries a valid
//     record (the latter is how a later, approved modification of a baseline skill or a newly added
//     skill stays green);
//   - CheckApprovals, the check behind `skills validate`, reports nothing.
func TestApprovalBaseline_PinnedToTheRepositoryRegistry(t *testing.T) {
	root := repoRoot(t)
	regData, err := os.ReadFile(filepath.Join(root, "skills.registry.yaml"))
	if err != nil {
		t.Fatalf("read real registry: %v", err)
	}
	reg, err := decode(regData)
	if err != nil {
		t.Fatalf("parse real registry: %v", err)
	}
	global := map[string]bool{}
	for _, e := range reg.Skills {
		if e.Install.DefaultScope == "global" {
			global[e.Path] = true
		}
	}
	for _, b := range skills.ApprovalBaseline() {
		if !global[b.ID] {
			t.Errorf("baseline id %q is not a registered global skill in skills.registry.yaml; a retired skill must be removed from the baseline deliberately", b.ID)
		}
	}

	divs, sum := skills.CheckApprovals(reg, filepath.Join(root, "skills"), skillsfs.Approvals{})
	for _, d := range divs {
		t.Errorf("[%s] %s: %s", d.Class, d.Path, d.Detail)
	}
	if sum.Global != len(global) {
		t.Errorf("CheckApprovals examined %d global skills, the registry has %d", sum.Global, len(global))
	}
	if sum.Approved+sum.Grandfathered != sum.Global {
		t.Errorf("summary = %+v: every global skill must be approved or grandfathered", sum)
	}
}

// TestValidateRealRegistryAndManifest is the integration check that the committed
// skills.registry.yaml and overlay.manifest are aligned. A future registry or manifest change that
// creates a divergence (MISSING_IN_REGISTRY, MISSING_IN_MANIFEST, TAG_MISMATCH) is caught by `go
// test` rather than silently regretting at runtime.
func TestValidateRealRegistryAndManifest(t *testing.T) {
	root := repoRoot(t)
	registryPath, manifestPath := filepath.Join(root, "skills.registry.yaml"), filepath.Join(root, "overlay.manifest")

	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("reading real registry %s: %v", registryPath, err)
	}
	reg, err := decode(data)
	if err != nil {
		t.Fatalf("parsing real registry: %v", err)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading real manifest %s: %v", manifestPath, err)
	}

	divs, err := skills.ValidateAgainstManifest(reg, manifest)
	if err != nil {
		t.Errorf("real registry/manifest diverged (%d divergence(s)):", len(divs))
		for _, d := range divs {
			t.Errorf("  [%s] %s: %s", d.Class, d.Path, d.Detail)
		}
		t.Errorf("error: %v", err)
	}
	t.Logf("real registry and manifest aligned (%d skills)", len(reg.Skills))
}

// A registry read from a file is matched like any other: a candidate that is an id matches, and a
// near miss that only shares a prefix does not.
func TestMatchCandidateOverARegistryReadFromAFile(t *testing.T) {
	reg, err := decode([]byte(`version: "1"
skills:
  - id: sdd-spec
    path: sdd-spec
    source:
      type: core
      upstream:
        owner: gentleman-programming
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
        - codex
    lifecycle:
      updateStrategy: vendor-merge
  - id: prespec-malandra
    path: prespec-malandra
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
        - codex
    lifecycle:
      updateStrategy: overlay-only
`))
	if err != nil {
		t.Fatalf("the registry does not read: %v", err)
	}

	matched, path := skills.MatchCandidate(reg, "sdd-spec")
	if !matched || path != "sdd-spec" {
		t.Fatalf("MatchCandidate(sdd-spec) over parsed registry = (%v, %q), want (true, %q)", matched, path, "sdd-spec")
	}

	matched, path = skills.MatchCandidate(reg, "sdd-spec-review")
	if matched || path != "" {
		t.Fatalf("MatchCandidate(sdd-spec-review) over parsed registry = (%v, %q), want (false, \"\") — substring near-miss must not match", matched, path)
	}
}

// The registry files that the end-to-end tests write by hand are what the encoder of the program
// writes, byte for byte: a fixture that drifted from the format would test a file the program never
// produces.
func TestTheRegistryFixturesAreWhatTheEncoderWrites(t *testing.T) {
	for name, fixture := range map[string]string{
		"the project CLI registry":     skills.ProjectCLIRegistryFixture,
		"the install fixture registry": skills.InstallRegistryFixture,
	} {
		t.Run(name, func(t *testing.T) {
			reg, err := decode([]byte(fixture))
			if err != nil {
				t.Fatalf("the fixture does not decode: %v", err)
			}
			got, err := encode(reg)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != fixture {
				t.Errorf("the encoder writes\n%s\nthe fixture is\n%s", got, fixture)
			}
		})
	}
}
