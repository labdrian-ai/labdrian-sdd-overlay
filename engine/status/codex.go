package status

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/codexhooks"
)

func (s Service) codexHook(req Request, binary string) *Check {
	home := req.CodexHome
	if home == "" {
		home = filepath.Join(req.Home, ".codex")
	}
	mode, err := s.Files.Stat(home)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !mode.IsDir()) {
		return nil
	}
	check := &Check{Label: "hook: Codex memory (SessionStart, PreToolUse, PostToolUse)", Level: Warn}
	if err != nil {
		check.Note = "cannot inspect Codex home: " + err.Error()
		return check
	}
	data, err := s.Files.ReadFile(filepath.Join(home, "hooks.json"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		check.Note = "cannot read hooks.json: " + err.Error()
		return check
	}
	missing, err := codexhooks.Missing(data, binary)
	if err != nil {
		check.Note = "cannot parse hooks.json: " + err.Error()
		return check
	}
	if len(missing) > 0 {
		check.Note = "missing or drifted: " + strings.Join(missing, ", ") + "; run 'labdrian install-hooks'"
		return check
	}
	check.Level = OK
	check.Note = "present; still needs the human's trust in the Codex TUI (/hooks, key t); trust state is not checked"
	return check
}
