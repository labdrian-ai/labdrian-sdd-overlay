package installer_test

// The commands that read a snapshot tarball (`capture --from-backup`) or a backup (`bootstrap`) read
// the managed rows from the paths of their route, including the opencode-agent rows under
// ~/.config/opencode. The sandbox and the tarball helpers are in overlay_sandbox_test.go.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// TestRouteTarPath_PlacesEachRouteWhereItsRuntimeKeepsItsFiles asks route_tar_path (bin/lib/
// manifest_routes.sh) where the snapshot of gentle-ai keeps the file of a row of each route: the
// skills and the agents of Claude Code under ~/.claude, the agents of OpenCode under
// ~/.config/opencode and, since slice 4 of the role agents, the agents of Codex under ~/.codex. The
// script defines its functions when it is sourced and does nothing else, so the test sources it in a
// home and an overlay of its own.
func TestRouteTarPath_PlacesEachRouteWhereItsRuntimeKeepsItsFiles(t *testing.T) {
	overlaytest.RequireBash(t)
	shell := overlaytest.Shell{Env: overlaytest.Env(t.TempDir(), t.TempDir())}
	for _, tc := range []struct{ route, repoRel, want string }{
		{"skill", "skills/test-skill/SKILL.md", "files/home/labdrian/.claude/skills/test-skill/SKILL.md"},
		{"agent", "agents/GADU.md", "files/home/labdrian/.claude/agents/GADU.md"},
		{"opencode-agent", "opencode/agents/GADU.md", "files/home/labdrian/.config/opencode/agents/GADU.md"},
		{"codex-agent", "codex/agents/builder.toml", "files/home/labdrian/.codex/agents/builder.toml"},
	} {
		t.Run(tc.route, func(t *testing.T) {
			out, err := overlaytest.Source(t, overlaytest.Script(t), shell, `route_tar_path "$@"`, tc.route, tc.repoRel)
			if err != nil {
				t.Fatalf("route_tar_path %s %s: %v\n%s", tc.route, tc.repoRel, err, out)
			}
			if out != tc.want {
				t.Errorf("route_tar_path %s %s = %q, want %q", tc.route, tc.repoRel, out, tc.want)
			}
		})
	}
}

// TestCaptureFromBackup_UsesRouteAwareTarPaths asserts that managed rows are
// restored from the correct route-specific paths inside the snapshot.
func TestCaptureFromBackup_UsesRouteAwareTarPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	overlayDir, env := setupCaptureFromBackupSandbox(t, home)

	createdBackup := createTarball(t, map[string]string{
		"files/home/labdrian/.claude/skills/test-skill/SKILL.md": "# backup skill\n",
		"files/home/labdrian/.claude/agents/GADU.md":             "---\nname: GADU\n---\n# backup claude agent\n",
		"files/home/labdrian/.config/opencode/agents/GADU.md":    "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n",
	})
	backup := filepath.Join(home, ".gentle-ai", "backups", "upgrade-20260615T175529Z", "snapshot.tar.gz")
	if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
		t.Fatalf("mkdir backup parent: %v", err)
	}
	backupBytes, err := os.ReadFile(createdBackup)
	if err != nil {
		t.Fatalf("read created backup: %v", err)
	}
	if err := os.WriteFile(backup, backupBytes, 0644); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	out, err := overlaytest.Combined(t, overlay, env, "", "capture", "--from-backup", backup)
	if err != nil {
		t.Fatalf("overlay capture --from-backup: %v\noutput:\n%s", err, out)
	}

	assertFileEquals := func(relPath, want string) {
		got := overlaytest.Git(t, overlayDir, nil, "show", "upstream:"+relPath)
		if got != want {
			t.Fatalf("upstream:%s content mismatch:\n  got:  %q\n  want: %q\noutput:\n%s", relPath, got, want, out)
		}
	}

	assertFileEquals("skills/test-skill/SKILL.md", "# backup skill\n")
	assertFileEquals("agents/GADU.md", "---\nname: GADU\n---\n# backup claude agent\n")
	assertFileEquals("opencode/agents/GADU.md", "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n")
}

// TestBootstrap_UsesRouteAwareTarPaths ensures bootstrap reads route-specific
// backup locations (including opencode-agent under ~/.config/opencode).
func TestBootstrap_UsesRouteAwareTarPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	overlayDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(overlayDir, "overlay.manifest"), []byte("test-skill/SKILL.md   managed\n"+
		"GADU.md   managed   agent\n"+
		"opencode/agents/GADU.md   managed   opencode-agent\n"), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	scriptDst := filepath.Join(overlayDir, "bin", "overlay")
	if err := os.MkdirAll(filepath.Dir(scriptDst), 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	scriptBody, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("read overlay script: %v", err)
	}
	if err := os.WriteFile(scriptDst, scriptBody, 0755); err != nil {
		t.Fatalf("write overlay script: %v", err)
	}

	createdBackup := createTarball(t, map[string]string{
		"files/home/labdrian/.claude/skills/test-skill/SKILL.md": "# backup skill\n",
		"files/home/labdrian/.claude/agents/GADU.md":             "---\nname: GADU\n---\n# backup claude agent\n",
		"files/home/labdrian/.config/opencode/agents/GADU.md":    "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n",
	})
	backup := filepath.Join(home, ".gentle-ai", "backups", "upgrade-20260615T175529Z", "snapshot.tar.gz")
	if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
		t.Fatalf("mkdir backup parent: %v", err)
	}
	backupBytes, err := os.ReadFile(createdBackup)
	if err != nil {
		t.Fatalf("read created backup: %v", err)
	}
	if err := os.WriteFile(backup, backupBytes, 0644); err != nil {
		t.Fatalf("write backup to default path: %v", err)
	}

	env := []string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"PATH=" + os.Getenv("PATH"),
	}
	enginetest.Provision(t, home)

	out, err := overlaytest.Combined(t, overlay, env, "", "bootstrap")
	if err != nil {
		t.Fatalf("overlay bootstrap: %v\noutput:\n%s", err, out)
	}

	assertBranchFileEquals := func(branch, relPath, want string) {
		got := overlaytest.Git(t, overlayDir, nil, "show", branch+":"+relPath)
		if got != want {
			t.Fatalf("%s:%s content mismatch:\n  got:  %q\n  want: %q\noutput:\n%s", branch, relPath, got, want, out)
		}
	}

	assertBranchFileEquals("upstream", "skills/test-skill/SKILL.md", "# backup skill\n")
	assertBranchFileEquals("upstream", "agents/GADU.md", "---\nname: GADU\n---\n# backup claude agent\n")
	assertBranchFileEquals("upstream", "opencode/agents/GADU.md", "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n")
	assertBranchFileEquals("main", "opencode/agents/GADU.md", "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n")
}
