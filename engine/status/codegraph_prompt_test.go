package status

import (
	"testing"
)

const labelCodegraphPrompt = `settings: UserPromptSubmit "codegraph prompt-hook" (task notifications skipped)`

// codegraphEntry appends a UserPromptSubmit entry with one hook object that runs command.
func codegraphEntry(command string) func(root map[string]interface{}) {
	return func(root map[string]interface{}) {
		hooks := root["hooks"].(map[string]interface{})
		hooks["UserPromptSubmit"] = append(hooks["UserPromptSubmit"].([]interface{}), map[string]interface{}{
			"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command}},
		})
	}
}

func TestTheCodegraphPromptHookIsReportedWrapped(t *testing.T) {
	w := complete(t)
	w.editSettings(t, codegraphEntry("command -v "+binaryPath+" >/dev/null 2>&1 || exec codegraph prompt-hook; exec "+binaryPath+" codegraph-prompt-hook"))
	wantCheck(t, w.check().named(t, labelCodegraphPrompt), OK, "wrapped by labdrian: task notifications do not reach codegraph")
}

func TestTheCodegraphPromptHookIsReportedOKWhenThereIsNoCodegraphEntry(t *testing.T) {
	wantCheck(t, complete(t).check().named(t, labelCodegraphPrompt), OK, "no codegraph entry, nothing to wrap")
}

func TestABareCodegraphPromptHookIsReportedWithTheInstallRemedy(t *testing.T) {
	w := complete(t)
	w.editSettings(t, codegraphEntry("codegraph prompt-hook"))
	wantCheck(t, w.check().named(t, labelCodegraphPrompt), Warn,
		`the bare "codegraph prompt-hook" entry is back (a codegraph reinstall rewrites it), so codegraph explores every task notification; run 'labdrian install-hooks'; `+restart)
}

func TestTheCodegraphPromptCheckDoesNotChangeTheOutcomeOfAHealthyInstall(t *testing.T) {
	if got := complete(t).check().Outcome(); got != Healthy {
		t.Errorf("outcome = %v, want Healthy", got)
	}
}
