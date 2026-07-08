package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const ooQualityContractMaxLines = 80

func TestOOQualityContractArtifact(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	contract := readOOQualityContract(t, repoRoot)

	t.Run("R001_local_concise_non_vendored_contract", func(t *testing.T) {
		if !strings.Contains(contract, "# OO Quality Contract") {
			t.Fatal("contract must have the expected title")
		}
		if len(strings.Split(strings.TrimSpace(contract), "\n")) > ooQualityContractMaxLines {
			t.Fatal("contract must stay concise")
		}
		for _, forbidden := range []string{"ramziddin/solid-skills", "github.com/ramziddin", "solid-skills"} {
			if strings.Contains(strings.ToLower(contract), forbidden) {
				t.Fatalf("contract must not vendor or reference external solid-skills content: %q", forbidden)
			}
		}
	})

	t.Run("R002_manifest_tracks_contract_once", func(t *testing.T) {
		manifest := readRepoFile(t, repoRoot, "overlay.manifest")
		row := "_shared/oo-quality-contract.md custom"
		if count := strings.Count(manifest, row); count != 1 {
			t.Fatalf("manifest row %q count = %d, want 1", row, count)
		}
	})

	t.Run("R003_frontmatter_scopes_phases", func(t *testing.T) {
		frontmatter := ooQualityFrontmatter(t, contract)
		assertStringList(t, parseInlineYAMLList(t, frontmatter, "applies_to_phases"), []string{"sdd-design", "sdd-tasks", "sdd-apply"})
		assertStringList(t, parseInlineYAMLList(t, frontmatter, "excluded_phases"), []string{"sdd-propose", "sdd-spec", "sdd-archive", "sdd-verify"})
		assertStringValue(t, parseYAMLScalar(t, frontmatter, "injection_point"), "## Skills to load before work")
		assertStringList(t, parseInlineYAMLList(t, frontmatter, "language_context"), []string{"typescript", "nestjs"})
		assertStringList(t, parseInlineYAMLList(t, frontmatter, "activation_context"), []string{"oo-domain-design", "domain-heavy-application-code", "review"})
	})

	t.Run("R004_precedence_is_explicit", func(t *testing.T) {
		for _, required := range []string{"specs", "design", "project conventions", "minimalism contract", "review budget", "win over this contract"} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must state precedence token %q", required)
			}
		}
	})

	t.Run("R005_context_gate_passes_through_non_domain_work", func(t *testing.T) {
		for _, required := range []string{"pass through", "Go", "shell", "docs", "config", "generated artifacts", "non-domain", "non-OO"} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must state context gate token %q", required)
			}
		}
	})

	t.Run("R006_advisory_guidance_requires_justification", func(t *testing.T) {
		for _, required := range []string{"diagnostic vocabulary", "invariants", "active variation", "tested seams", "advisory warnings"} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must state advisory guidance token %q", required)
			}
		}
	})

	t.Run("R007_tdd_not_imposed_globally", func(t *testing.T) {
		for _, required := range []string{"Do not impose TDD", "project or task config"} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must state TDD boundary token %q", required)
			}
		}
	})

	t.Run("R008_first_slice_does_not_add_runtime_wiring", func(t *testing.T) {
		for _, path := range []string{
			"engine/gate/gate.go",
			"engine/propagator/propagator.go",
			"engine/cmd/main.go",
		} {
			content := readRepoFile(t, repoRoot, path)
			if strings.Contains(content, "oo-quality-contract") || strings.Contains(content, "OO Quality Contract") {
				t.Fatalf("%s must not wire oo-quality-contract in this first slice", path)
			}
		}
	})
}

func ooQualityContractRepoRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	return repoRoot
}

func readOOQualityContract(t *testing.T, repoRoot string) string {
	t.Helper()
	return readRepoFile(t, repoRoot, "skills/_shared/oo-quality-contract.md")
}

func readRepoFile(t *testing.T, repoRoot, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

func ooQualityFrontmatter(t *testing.T, content string) string {
	t.Helper()
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		t.Fatal("contract must include YAML frontmatter")
	}
	return parts[1]
}

func parseInlineYAMLList(t *testing.T, frontmatter, key string) []string {
	t.Helper()
	value, ok := parseYAMLValue(frontmatter, key)
	if !ok {
		t.Fatalf("frontmatter key %q not found", key)
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		t.Fatalf("frontmatter key %q must be an inline YAML list", key)
	}

	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	if strings.TrimSpace(value) == "" {
		return nil
	}

	items := strings.Split(value, ",")
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	return items
}

func parseYAMLScalar(t *testing.T, frontmatter, key string) string {
	t.Helper()
	value, ok := parseYAMLValue(frontmatter, key)
	if !ok {
		t.Fatalf("frontmatter key %q not found", key)
	}
	return strings.Trim(value, `"'`)
}

func parseYAMLValue(frontmatter, key string) (string, bool) {
	prefix := key + ":"
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
	}
	return "", false
}

func assertStringValue(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("frontmatter value = %q, want %q", got, want)
	}
}

func assertStringList(t *testing.T, got, want []string) {
	t.Helper()

	got = sortedStrings(got)
	want = sortedStrings(want)

	if len(got) != len(want) {
		t.Fatalf("frontmatter list length = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frontmatter list = %v, want %v", got, want)
		}
	}
}

func sortedStrings(values []string) []string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted
}
