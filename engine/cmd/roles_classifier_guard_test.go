package main

// Tests for 'roles classifier-guard', the PreToolUse hook that keeps the classifier to its page and its script.
// The decision is engine/classifierguard's and is tested there; these pin the contract with Claude Code through
// the public verb: it answers by exit status, and every failure blocks.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/classifierguard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// runClassifierGuard runs the verb on stdin and returns its exit codes, stdout and stderr.
func runClassifierGuard(stdin string, args ...string) (codes []int, stdout, stderr string) {
	var out, errBuf bytes.Buffer
	runRolesCore(append([]string{"classifier-guard"}, args...), strings.NewReader(stdin), &out, &errBuf,
		func(c int) { codes = append(codes, c) }, nil)
	return codes, out.String(), errBuf.String()
}

func TestRolesClassifierGuardAnswersByExitStatusAndFailsClosed(t *testing.T) {
	codes, out, errText := runClassifierGuard(`{"tool_name":"Read","tool_input":{"file_path":"/r/x"}}`)
	if len(codes) != 1 || codes[0] != 0 || out != "" || errText != "" {
		t.Errorf("Read: codes=%v stdout=%q stderr=%q, want exit 0 and no output", codes, out, errText)
	}

	codes, out, errText = runClassifierGuard(`{"tool_name":"Bash","tool_input":{"command":"git push"}}`)
	if len(codes) != 1 || codes[0] != 2 || out != "" || errText != classifierguard.ReasonCommand+"\n" {
		t.Errorf("git push: codes=%v stdout=%q stderr=%q, want exit 2 and the command reason", codes, out, errText)
	}

	failures := []struct {
		name, stdin string
		args        []string
	}{
		{"not JSON", "not json", nil},
		{"tool_input not an object", `{"tool_name":"Read","tool_input":"x"}`, nil},
		{"over the bound", strings.Repeat(" ", hookwire.MaxToolCallBytes+1), nil},
		{"an argument", `{"tool_name":"Read","tool_input":{}}`, []string{"x"}},
	}
	for _, f := range failures {
		codes, out, errText := runClassifierGuard(f.stdin, f.args...)
		if len(codes) != 1 || codes[0] != 2 || out != "" {
			t.Errorf("%s: codes=%v stdout=%q, want exit 2 and no stdout", f.name, codes, out)
		}
		if strings.Count(errText, "\n") != 1 || !strings.HasSuffix(errText, "\n") || !strings.Contains(errText, "classifier-guard") {
			t.Errorf("%s: stderr %q is not one line naming the verb", f.name, errText)
		}
	}
}

// A panic inside the verb blocks the call with one line, as every other failure does.
func TestRolesClassifierGuardBlocksARecoveredPanic(t *testing.T) {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesClassifierGuard(nil, panickingReader{}, &out, &errBuf, func(c int) { codes = append(codes, c) })
	if len(codes) != 1 || codes[0] != 2 || strings.Count(errBuf.String(), "\n") != 1 {
		t.Errorf("codes=%v stderr=%q, want one exit 2 and one line", codes, errBuf.String())
	}
}

type panickingReader struct{}

func (panickingReader) Read([]byte) (int, error) { panic("boom") }

func TestDocsDescribeTheClassifierGuard(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	engine := strings.Join(strings.Fields(out.String()), " ")
	readme := strings.Join(strings.Fields(readRepositoryFile(t, "README.md")), " ")
	wrapper := strings.Join(strings.Fields(readRepositoryFile(t, "bin/lib/usage.sh")), " ")
	for _, doc := range []struct{ name, text string }{
		{"the engine usage", docSection(t, engine, "engine roles classifier-guard", "engine shaper assess")},
		{"the wrapper help", docSection(t, wrapper, "classifier-guard", "review <verb>")},
		{"the README section", docSection(t, readme, "gentle-ai-overlay roles classifier-guard", "overlay --help")},
	} {
		for _, want := range []string{"classifier-guard", "agents/classifier.md", "fails closed"} {
			if !strings.Contains(doc.text, want) {
				t.Errorf("%s does not say %q", doc.name, want)
			}
		}
	}
}
