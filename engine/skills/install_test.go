package skills

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ---------------------------------------------------------------------------
// T-04: ExecuteInstall tests (SC-15, SC-16, SC-19)
// ---------------------------------------------------------------------------

// makeSourceSkill creates a skill directory with the given files under overlayDir/skillID/.
func makeSourceSkill(t *testing.T, overlayDir, skillID string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(overlayDir, skillID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("makeSourceSkill MkdirAll: %v", err)
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("makeSourceSkill WriteFile %s: %v", name, err)
		}
	}
}

// makeInstallRegistryYAML builds a minimal valid registry YAML with one project-scoped entry.
func makeInstallRegistryYAML(skillID string, allowedProjects []string) string {
	allowed := ""
	for _, p := range allowedProjects {
		allowed += fmt.Sprintf("\n        - %s", p)
	}
	return fmt.Sprintf(`version: "1"
skills:
  - id: %s
    path: %s
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:%s
    lifecycle:
      updateStrategy: overlay-only
`, skillID, skillID, allowed)
}

// TestExecuteInstall covers the I/O executor directly (SC-15, SC-16, SC-19, SC-21).
func TestExecuteInstall(t *testing.T) {
	t.Run("SC_15_copies_skill_file", func(t *testing.T) {
		// SC-15: valid install copies SKILL.md into <dst>/.
		overlayDir := t.TempDir()
		targetDir := t.TempDir()

		makeSourceSkill(t, overlayDir, "my-skill", map[string]string{
			"SKILL.md": "# My Skill",
		})

		plan := []CopyOp{{
			SkillID: "my-skill",
			Src:     filepath.Join(overlayDir, "my-skill"),
			Dst:     filepath.Join(targetDir, ".claude", "skills", "my-skill"),
		}}

		var out, errBuf bytes.Buffer
		err := ExecuteInstall(plan, &out, &errBuf)
		if err != nil {
			t.Fatalf("ExecuteInstall error: %v\nstderr: %s", err, errBuf.String())
		}

		installed := filepath.Join(targetDir, ".claude", "skills", "my-skill", "SKILL.md")
		data, rerr := os.ReadFile(installed)
		if rerr != nil {
			t.Fatalf("installed file not found: %v", rerr)
		}
		if string(data) != "# My Skill" {
			t.Errorf("content mismatch: got %q", string(data))
		}
		if !strings.Contains(out.String(), "installed: my-skill") {
			t.Errorf("stdout %q missing 'installed: my-skill'", out.String())
		}
	})

	t.Run("SC_16_idempotent_overwrite", func(t *testing.T) {
		// SC-16: second run overwrites cleanly; no error.
		overlayDir := t.TempDir()
		targetDir := t.TempDir()

		makeSourceSkill(t, overlayDir, "my-skill", map[string]string{
			"SKILL.md": "# Canonical Content",
		})

		dst := filepath.Join(targetDir, ".claude", "skills", "my-skill")
		plan := []CopyOp{{SkillID: "my-skill", Src: filepath.Join(overlayDir, "my-skill"), Dst: dst}}

		// First run
		var out1, err1 bytes.Buffer
		if err := ExecuteInstall(plan, &out1, &err1); err != nil {
			t.Fatalf("first run: %v", err)
		}

		// Pre-modify destination to detect overwrite
		if err := os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte("# Stale"), 0644); err != nil {
			t.Fatalf("pre-modify: %v", err)
		}

		// Second run
		var out2, err2 bytes.Buffer
		if err := ExecuteInstall(plan, &out2, &err2); err != nil {
			t.Fatalf("second run: %v\nstderr: %s", err, err2.String())
		}

		data, rerr := os.ReadFile(filepath.Join(dst, "SKILL.md"))
		if rerr != nil {
			t.Fatalf("file not found after second run: %v", rerr)
		}
		if string(data) != "# Canonical Content" {
			t.Errorf("overwrite failed: got %q, want canonical content", string(data))
		}
	})

	t.Run("SC_19_missing_source_dir_fail_loud", func(t *testing.T) {
		// SC-19: missing source dir → non-zero via error, stderr contains skill id and path.
		targetDir := t.TempDir()

		plan := []CopyOp{{
			SkillID: "ghost-skill",
			Src:     "/nonexistent/path/ghost-skill",
			Dst:     filepath.Join(targetDir, ".claude", "skills", "ghost-skill"),
		}}

		var out, errBuf bytes.Buffer
		err := ExecuteInstall(plan, &out, &errBuf)
		if err == nil {
			t.Fatal("expected error for missing source dir, got nil")
		}
		if !strings.Contains(errBuf.String(), "ghost-skill") {
			t.Errorf("stderr %q should contain skill id 'ghost-skill'", errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "/nonexistent/path/ghost-skill") {
			t.Errorf("stderr %q should contain expected source path", errBuf.String())
		}
	})

	t.Run("SC_19_all_missing_reported", func(t *testing.T) {
		// SC-19 extension: all missing source dirs reported before returning.
		targetDir := t.TempDir()

		plan := []CopyOp{
			{SkillID: "ghost-a", Src: "/no/ghost-a", Dst: filepath.Join(targetDir, ".claude", "skills", "ghost-a")},
			{SkillID: "ghost-b", Src: "/no/ghost-b", Dst: filepath.Join(targetDir, ".claude", "skills", "ghost-b")},
		}

		var out, errBuf bytes.Buffer
		err := ExecuteInstall(plan, &out, &errBuf)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(errBuf.String(), "ghost-a") {
			t.Errorf("stderr %q should contain 'ghost-a'", errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "ghost-b") {
			t.Errorf("stderr %q should contain 'ghost-b'", errBuf.String())
		}
	})

	t.Run("SC_21_writes_only_under_claude_skills", func(t *testing.T) {
		// SC-21: install writes nothing outside <cwd>/.claude/skills/.
		overlayDir := t.TempDir()
		targetDir := t.TempDir()

		makeSourceSkill(t, overlayDir, "my-skill", map[string]string{
			"SKILL.md": "# My Skill",
		})

		plan := []CopyOp{{
			SkillID: "my-skill",
			Src:     filepath.Join(overlayDir, "my-skill"),
			Dst:     filepath.Join(targetDir, ".claude", "skills", "my-skill"),
		}}

		var out, errBuf bytes.Buffer
		if err := ExecuteInstall(plan, &out, &errBuf); err != nil {
			t.Fatalf("ExecuteInstall: %v", err)
		}

		// Walk targetDir; every file must be under .claude/skills/
		skillsBase := filepath.Join(targetDir, ".claude", "skills") + string(os.PathSeparator)
		err := filepath.WalkDir(targetDir, func(path string, d os.DirEntry, werr error) error {
			if werr != nil || d.IsDir() {
				return werr
			}
			if !strings.HasPrefix(path, skillsBase) {
				return fmt.Errorf("file outside .claude/skills/: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Errorf("SC-21 violation: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// T-04: RenderInstallCore tests (SC-17, SC-18, SC-20, SC-22)
// ---------------------------------------------------------------------------

// installCwdFn returns a cwdFn that always returns the given directory.
func installCwdFn(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

// failCwdFn returns a cwdFn that always returns an error.
func failCwdFn() func() (string, error) {
	return func() (string, error) { return "", errors.New("cwd resolution failed") }
}

func TestRenderInstallCore(t *testing.T) {
	t.Run("SC_17_filter_by_allowedProjects", func(t *testing.T) {
		// SC-17: skill-A not in allowedProjects → not copied; skill-B is → copied.
		overlayDir := t.TempDir()
		cwdDir := t.TempDir()

		makeSourceSkill(t, overlayDir, "skill-b", map[string]string{"SKILL.md": "B"})
		// skill-a dir not created intentionally — it should be excluded before copy.

		regYAML := `version: "1"
skills:
  - id: skill-a
    path: skill-a
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - other-repo
    lifecycle:
      updateStrategy: overlay-only
  - id: skill-b
    path: skill-b
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - target-repo
    lifecycle:
      updateStrategy: overlay-only
`
		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderInstallCore(
			[]string{"--registry", "reg.yaml", "--source-root", overlayDir, "--project-id", "target-repo"},
			func(_ string) ([]byte, error) { return []byte(regYAML), nil },
			installCwdFn(cwdDir),
			&out, &errBuf,
			func(c int) { exitCode = c },
		)

		if exitCode != 0 {
			t.Fatalf("exit code = %d, stderr: %q", exitCode, errBuf.String())
		}
		skillB := filepath.Join(cwdDir, ".claude", "skills", "skill-b")
		if _, err := os.Stat(skillB); err != nil {
			t.Errorf("skill-b should be installed: %v", err)
		}
		skillA := filepath.Join(cwdDir, ".claude", "skills", "skill-a")
		if _, err := os.Stat(skillA); err == nil {
			t.Error("skill-a should NOT be installed")
		}
	})

	t.Run("SC_18_global_skills_excluded", func(t *testing.T) {
		// SC-18: global-scoped skill excluded; project-scoped admitted.
		overlayDir := t.TempDir()
		cwdDir := t.TempDir()

		makeSourceSkill(t, overlayDir, "project-skill", map[string]string{"SKILL.md": "P"})

		regYAML := `version: "1"
skills:
  - id: global-skill
    path: global-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: project-skill
    path: project-skill
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - target-repo
    lifecycle:
      updateStrategy: overlay-only
`
		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderInstallCore(
			[]string{"--registry", "reg.yaml", "--source-root", overlayDir, "--project-id", "target-repo"},
			func(_ string) ([]byte, error) { return []byte(regYAML), nil },
			installCwdFn(cwdDir),
			&out, &errBuf,
			func(c int) { exitCode = c },
		)

		if exitCode != 0 {
			t.Fatalf("exit code = %d, stderr: %q", exitCode, errBuf.String())
		}
		projSkill := filepath.Join(cwdDir, ".claude", "skills", "project-skill")
		if _, err := os.Stat(projSkill); err != nil {
			t.Errorf("project-skill should be installed: %v", err)
		}
		globalSkill := filepath.Join(cwdDir, ".claude", "skills", "global-skill")
		if _, err := os.Stat(globalSkill); err == nil {
			t.Error("global-skill should NOT be installed")
		}
	})

	t.Run("SC_20_cwd_fails_exit_nonzero", func(t *testing.T) {
		// SC-20: cwd resolution fails → exit non-zero, stderr contains reason, no files written.
		cwdDir := t.TempDir()

		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderInstallCore(
			[]string{"--registry", "reg.yaml", "--source-root", "/overlay"},
			func(_ string) ([]byte, error) { return []byte(`version: "1"`), nil },
			failCwdFn(),
			&out, &errBuf,
			func(c int) { exitCode = c },
		)

		if exitCode == 0 {
			t.Fatal("expected non-zero exit when cwd fails")
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty when cwd fails")
		}
		// Verify no files written under cwdDir (which the cwd fn would have returned)
		claudeDir := filepath.Join(cwdDir, ".claude")
		if _, err := os.Stat(claudeDir); err == nil {
			t.Error("no files should be written when cwd fails")
		}
	})

	t.Run("SC_22_empty_install_set_exit_0", func(t *testing.T) {
		// SC-22: no skills admitted → exit 0 with notice containing project id.
		cwdDir := t.TempDir()

		regYAML := makeInstallRegistryYAML("some-skill", []string{"other-repo"})

		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderInstallCore(
			[]string{"--registry", "reg.yaml", "--source-root", "/overlay", "--project-id", "target-repo"},
			func(_ string) ([]byte, error) { return []byte(regYAML), nil },
			installCwdFn(cwdDir),
			&out, &errBuf,
			func(c int) { exitCode = c },
		)

		if exitCode != 0 {
			t.Fatalf("exit code = %d; expected 0 for empty plan", exitCode)
		}
		if !strings.Contains(out.String(), "target-repo") {
			t.Errorf("stdout %q should contain project id 'target-repo'", out.String())
		}
		if !strings.Contains(out.String(), "no project-scoped skills") {
			t.Errorf("stdout %q should contain empty-plan notice", out.String())
		}
	})

	t.Run("SC_15_full_pipeline", func(t *testing.T) {
		// SC-15 via RenderInstallCore: copies SKILL.md into cwdDir/.claude/skills/my-skill/.
		overlayDir := t.TempDir()
		cwdDir := t.TempDir()

		makeSourceSkill(t, overlayDir, "my-skill", map[string]string{"SKILL.md": "# My Skill"})

		regYAML := makeInstallRegistryYAML("my-skill", []string{"target-repo"})

		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderInstallCore(
			[]string{"--registry", "reg.yaml", "--source-root", overlayDir, "--project-id", "target-repo"},
			func(_ string) ([]byte, error) { return []byte(regYAML), nil },
			installCwdFn(cwdDir),
			&out, &errBuf,
			func(c int) { exitCode = c },
		)

		if exitCode != 0 {
			t.Fatalf("exit code = %d, stderr: %q", exitCode, errBuf.String())
		}
		installed := filepath.Join(cwdDir, ".claude", "skills", "my-skill", "SKILL.md")
		data, rerr := os.ReadFile(installed)
		if rerr != nil {
			t.Fatalf("installed file not found: %v", rerr)
		}
		if string(data) != "# My Skill" {
			t.Errorf("content mismatch: %q", string(data))
		}
		if !strings.Contains(out.String(), "installed: my-skill") {
			t.Errorf("stdout %q missing 'installed: my-skill'", out.String())
		}
	})
}
