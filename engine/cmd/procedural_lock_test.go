package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// TestTheLockReaderGivesTheAbsoluteSkillFilesOfTheProceduralSkillsOnly (C23 M4 decision 5): each
// target of a procedural skill becomes the SKILL.md under the worktree root; a target that is
// absolute or climbs out of the root names no file; the installs are not procedural.
func TestTheLockReaderGivesTheAbsoluteSkillFilesOfTheProceduralSkillsOnly(t *testing.T) {
	root := t.TempDir()
	lock := `{"version":1,"skills":[{"id":"deploy-checks","provenance":"p","candidate":"c","sha256":"` + strings.Repeat("a", 64) +
		`","revision":1,"targets":[".claude/skills/deploy-checks/SKILL.md","/etc/SKILL.md","../out/SKILL.md",".agents/skills/x/../deploy-checks/SKILL.md"]}],` +
		`"installs":[{"id":"go-testing","files":[{"path":"SKILL.md","sha256":"` + strings.Repeat("b", 64) + `"}]}]}`
	writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(skills.ProjectLockRelPath)), lock)
	reader := proceduralLockReader{worktreeRoot: func(string) string { return root }}

	got := reader.Load("/any")
	want := projection.ProceduralSkills{Skills: []projection.ProceduralSkill{{ID: "deploy-checks", SkillFiles: []string{
		filepath.Join(root, ".claude", "skills", "deploy-checks", "SKILL.md"),
		filepath.Join(root, ".agents", "skills", "deploy-checks", "SKILL.md"),
	}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestTheLockReaderSaysNothingWithoutAWorktreeOrALock(t *testing.T) {
	none := proceduralLockReader{worktreeRoot: func(string) string { return "" }}
	if got := none.Load("/any"); !reflect.DeepEqual(got, projection.ProceduralSkills{}) {
		t.Errorf("no worktree: Load = %+v, want the zero value", got)
	}
	empty := proceduralLockReader{worktreeRoot: func(string) string { return t.TempDir() }}
	if got := empty.Load("/any"); !reflect.DeepEqual(got, projection.ProceduralSkills{}) {
		t.Errorf("no lock: Load = %+v, want the zero value", got)
	}
}

func TestTheLockReaderReportsALockItCannotRead(t *testing.T) {
	invalid := t.TempDir()
	writeFixtureFile(t, filepath.Join(invalid, filepath.FromSlash(skills.ProjectLockRelPath)), `{"version":2,"skills":[]}`)
	if got := (proceduralLockReader{worktreeRoot: func(string) string { return invalid }}).Load("/any"); !strings.Contains(got.Unreadable, "unsupported version 2") || got.Skills != nil {
		t.Errorf("an invalid lock: Load = %+v, want it unreadable with the reason", got)
	}
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, filepath.FromSlash(skills.ProjectLockRelPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := (proceduralLockReader{worktreeRoot: func(string) string { return directory }}).Load("/any"); got.Unreadable == "" {
		t.Errorf("a lock that is a directory: Load = %+v, want it unreadable", got)
	}
}
