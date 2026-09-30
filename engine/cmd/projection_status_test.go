package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

func TestCheckProjectionHooks(t *testing.T) {
	const bin = "/x/.claude/bin/gentle-ai-overlay"
	full := buildSettingsWithHooks(bin)
	if c := checkProjectionHooks(full, nil, "s.json", bin); !c.ok || c.degraded {
		t.Errorf("full family: %+v", c)
	}

	// A machine installed before the projection family existed: nothing owned.
	hooks := full["hooks"].(map[string]interface{})
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		var kept []interface{}
		for _, e := range hooks[key].([]interface{}) {
			if !strings.Contains(mustJSONString(t, e), settings.LabdrianProjectionIdentity) {
				kept = append(kept, e)
			}
		}
		hooks[key] = kept
	}
	c := checkProjectionHooks(full, nil, "s.json", bin)
	if !c.ok || !c.degraded || !strings.Contains(c.note, remediationNote) || !strings.Contains(c.note, "UserPromptSubmit") || !strings.Contains(c.note, "restart Claude Code") {
		t.Errorf("missing family should be WARN naming the fix and the restart: %+v", c)
	}

	if c := checkProjectionHooks(nil, nil, "s.json", bin); !c.ok || !c.degraded {
		t.Errorf("absent settings: %+v", c)
	}
	if c := checkProjectionHooks(nil, errors.New("boom"), "s.json", bin); c.ok {
		t.Errorf("unreadable settings must FAIL like the other hook checks: %+v", c)
	}
}

func mustJSONString(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
