package status

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/codexhooks"
)

func TestCodexHookStatusUsesDefaultOrExplicitHomeAndNeverClaimsTrust(t *testing.T) {
	for _, override := range []string{"", "/custom-codex"} {
		w := complete(t)
		service := Service{Files: w.files, Settings: w.settings, Blocks: testBlocks()}
		req := Request{Home: home, Cwd: project, CodexHome: override}
		codex := override
		if codex == "" {
			codex = "/h/.codex"
		}
		if got := service.Check(req); got.Outcome() != Healthy || len(got.Checks) != 10 {
			t.Fatalf("absent Codex changed the old outcome: %+v", got)
		}
		w.files.modes[codex] = fs.ModeDir | 0o700
		label := "hook: Codex SessionStart (memory instructions)"
		if got := service.Check(req); got.named(t, label).Level != Warn || got.Outcome() != Degraded {
			t.Fatalf("missing hook: %+v", got)
		}
		data, _, err := codexhooks.Merge(nil, binaryPath)
		if err != nil {
			t.Fatal(err)
		}
		w.files.contents[codex+"/hooks.json"] = string(data)
		got := service.Check(req)
		check := got.named(t, label)
		wantCheck(t, check, OK, "present; still needs the human's trust in the Codex TUI (/hooks, key t); trust state is not checked")
		if got.Outcome() != Healthy {
			t.Fatal(got)
		}
		for _, asked := range w.files.asked {
			if strings.Contains(asked, "config.toml") {
				t.Fatal("status read trust state")
			}
		}
		w.files.contents[codex+"/hooks.json"] = `{"hooks":[]}`
		check = service.Check(req).named(t, label)
		wantCheck(t, check, Warn, "cannot parse hooks.json: hooks must be an object")
	}
}
