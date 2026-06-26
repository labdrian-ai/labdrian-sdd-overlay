package skills

import (
	"path/filepath"
	"testing"
)

// buildRegistry is a test helper that constructs a Registry from a slice of
// (id, scope, allowedProjects) tuples. All entries get minimal valid fields.
func buildRegistry(entries []struct {
	id              string
	scope           string
	allowedProjects []string
}) Registry {
	var skills []Entry
	for _, e := range entries {
		skills = append(skills, Entry{
			ID:   e.id,
			Path: e.id,
			Source: Source{
				Type: "custom",
			},
			Install: Install{
				DefaultScope:    e.scope,
				Targets:         []string{"claude"},
				AllowedProjects: e.allowedProjects,
			},
			Lifecycle: Lifecycle{
				UpdateStrategy: "overlay-only",
			},
		})
	}
	return Registry{Version: "1", Skills: skills}
}

// TestPlanInstall is the table-driven suite for the pure planner (T-03).
func TestPlanInstall(t *testing.T) {
	const sourceRoot = "/overlay/skills"
	const targetRoot = "/target-repo"

	t.Run("project_skill_admitted", func(t *testing.T) {
		// Project-scoped entry with matching allowedProjects → one CopyOp.
		reg := buildRegistry([]struct {
			id              string
			scope           string
			allowedProjects []string
		}{
			{"my-skill", "project", []string{"target-repo"}},
		})
		ops, err := PlanInstall(reg, "target-repo", sourceRoot, targetRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 1 {
			t.Fatalf("len(ops) = %d, want 1", len(ops))
		}
		wantSrc := filepath.Join(sourceRoot, "my-skill")
		wantDst := filepath.Join(targetRoot, ".claude", "skills", "my-skill")
		if ops[0].Src != wantSrc {
			t.Errorf("Src = %q, want %q", ops[0].Src, wantSrc)
		}
		if ops[0].Dst != wantDst {
			t.Errorf("Dst = %q, want %q", ops[0].Dst, wantDst)
		}
		if ops[0].SkillID != "my-skill" {
			t.Errorf("SkillID = %q, want my-skill", ops[0].SkillID)
		}
	})

	t.Run("project_skill_excluded_mismatch", func(t *testing.T) {
		// allowedProjects does not include projectID → empty plan.
		reg := buildRegistry([]struct {
			id              string
			scope           string
			allowedProjects []string
		}{
			{"other-skill", "project", []string{"other-repo"}},
		})
		ops, err := PlanInstall(reg, "target-repo", sourceRoot, targetRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 0 {
			t.Errorf("expected empty plan, got %d ops", len(ops))
		}
	})

	t.Run("global_skill_excluded", func(t *testing.T) {
		// global-scoped entry is always excluded.
		reg := buildRegistry([]struct {
			id              string
			scope           string
			allowedProjects []string
		}{
			{"global-skill", "global", nil},
		})
		ops, err := PlanInstall(reg, "target-repo", sourceRoot, targetRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 0 {
			t.Errorf("expected empty plan for global skill, got %d ops", len(ops))
		}
	})

	t.Run("project_skill_nil_allowedProjects_excluded", func(t *testing.T) {
		// project-scoped entry with nil AllowedProjects → excluded (no match possible).
		reg := buildRegistry([]struct {
			id              string
			scope           string
			allowedProjects []string
		}{
			{"proj-skill", "project", nil},
		})
		ops, err := PlanInstall(reg, "target-repo", sourceRoot, targetRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 0 {
			t.Errorf("expected empty plan for project skill with nil allowedProjects, got %d ops", len(ops))
		}
	})

	t.Run("multiple_entries_correct_filter_order_preserved", func(t *testing.T) {
		// Multiple entries: only project-scoped + matching are admitted; declaration order preserved.
		reg := buildRegistry([]struct {
			id              string
			scope           string
			allowedProjects []string
		}{
			{"skill-a", "project", []string{"target-repo"}},
			{"skill-b", "global", nil},
			{"skill-c", "project", []string{"other-repo"}},
			{"skill-d", "project", []string{"target-repo", "other-repo"}},
		})
		ops, err := PlanInstall(reg, "target-repo", sourceRoot, targetRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 2 {
			t.Fatalf("expected 2 ops, got %d", len(ops))
		}
		if ops[0].SkillID != "skill-a" {
			t.Errorf("ops[0].SkillID = %q, want skill-a", ops[0].SkillID)
		}
		if ops[1].SkillID != "skill-d" {
			t.Errorf("ops[1].SkillID = %q, want skill-d", ops[1].SkillID)
		}
	})

	t.Run("empty_registry", func(t *testing.T) {
		// Empty registry → empty plan, no error.
		reg := Registry{Version: "1"}
		ops, err := PlanInstall(reg, "target-repo", sourceRoot, targetRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 0 {
			t.Errorf("expected empty plan for empty registry, got %d ops", len(ops))
		}
	})
}
