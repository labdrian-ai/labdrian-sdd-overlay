package main

// The documentation of the builder guard (own-rdd T11f, S14 h and i): the usage text of the engine, the README, the
// wrapper's install-hooks and uninstall-hooks summaries and help, the table of verbs, the shaper's role files and the
// CHANGELOG. Like the documentation tests of the approve guard and the review stop-hook, they pin what a user must be
// told: what the guard denies and why, what it cites, that it is a speed bump and not a boundary, that Codex and Pi are
// not guarded, that install-hooks must be re-run and Claude Code restarted, and that no hook family is left out of the
// summaries of the verbs that manage them.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The engine usage says the guard is installed by install-hooks, what it cites, what it denies and its limits.
func TestUsageDocumentsTheBuilderGuardAsInstalled(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	text := strings.Join(strings.Fields(out.String()), " ")
	block := docSection(t, text, "engine roles builder-guard", "engine shaper assess")

	for _, want := range []string{
		"internal Claude Code PreToolUse hook command for the Agent tool (install-hooks installs it",
		"re-run install-hooks and restart Claude Code",
		"shaped: L<n>",
		"launch the shaper first",
		"4194304 bytes",
		"speed bump against forgetting the shaper, not a security boundary",
		"exit 0 always, 1 usage error",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the engine usage of the builder guard lacks %q", want)
		}
	}
	if strings.Contains(block, "not installed yet") {
		t.Error("the engine usage still says the builder guard is not installed")
	}
}

// The help of the engine, the wrapper and the README say that the guard also requires the running workflow of the
// cited feature, and what workflow open, its remedy, does (own-rdd T17c, S19 d).
func TestDocsDescribeTheWorkflowTheBuilderGuardRequires(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	engine := strings.Join(strings.Fields(out.String()), " ")
	readme := strings.Join(strings.Fields(readRepositoryFile(t, "README.md")), " ")
	wrapper := strings.Join(strings.Fields(readRepositoryFile(t, "bin/lib/usage.sh")), " ")

	for _, doc := range []struct {
		name, text string
		wants      []string
	}{
		{"the engine usage of the builder guard", docSection(t, engine, "engine roles builder-guard", "engine shaper assess"), []string{
			"bound to the running workflow of the cited feature", "hands the work to the builder",
			"labdrian workflow open --feature <feature> --task <T>",
		}},
		{"the engine usage of workflow open", docSection(t, engine, "engine workflow open", "engine projection hook"), []string{
			"--feature <f> --task <T> [--goal <path>]", "odd/goals/<f>.json", "shaped: L<n>", "<f>-<t>",
			"authorize, explore, resolve-uncertainty, classify and track-if-substantial",
			`{"workflow":"<f>-<t>","chain":"<f>-<t>","goal":"<f>","project":"<P>","status":"running","bound":true}`,
			"exit 0 opened, 2 refused, 1 usage error or a failure after a write",
		}},
		{"the README section of the builder guard", docSection(t, readme, "gentle-ai-overlay roles builder-guard", "overlay --help"), []string{
			"to a workflow that is owned and running, whose goal is the cited feature",
			"Every feature runs inside its workflow: run labdrian workflow open --feature <f> --task <T>",
		}},
		{"the README of workflow open", docSection(t, readme, "open --feature <f> --task <T> [--goal <path>]", "The lifecycle verbs print"), []string{
			"odd/goals/<f>.json", "shaped: L<n>", "<f>-<t>", "exit 0 opened, 2 refused",
		}},
		{"the wrapper help", wrapper, []string{
			"outside the running workflow of the cited feature", "open --feature F --task T [--goal PATH]",
		}},
	} {
		for _, want := range doc.wants {
			if !strings.Contains(doc.text, want) {
				t.Errorf("%s does not say %q", doc.name, want)
			}
		}
	}
}

// The guard is installed by install-hooks and run by Claude Code on every Agent call, so the README says what it
// denies, what the prompt must cite, how the shaper marks a task, what it cannot do, and what a user must do to get it.
func TestDocsDescribeTheBuilderGuard(t *testing.T) {
	section := docSection(t, readRepositoryFile(t, "README.md"), "gentle-ai-overlay roles builder-guard\n", "overlay --help\n")

	for _, want := range []string{
		"roles builder-guard",
		"PreToolUse",
		"Agent",
		"denies launching the builder",
		`Spec: odd/tasks/<feature>.md, T<n>`,
		"shaped: L<n>",
		"(<date>, shaper)",
		"launch the shaper first",
		"speed bump",
		"not a security boundary",
		"Codex and Pi",
		"4 MiB",
		"never blocks by its exit code",
		"re-run install-hooks",
		"restart Claude Code",
		"status-hooks",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the README section of the builder guard does not say %q", want)
		}
	}
}

// Every summary of the verbs that manage the hook families names the builder guard, with the other families, and the
// summaries of the verbs that list the verbs of the engine name its verb.
func TestInstallHooksSummariesNameTheBuilderGuard(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	overlay := overlaytest.ScriptText(t)
	sections := map[string]string{
		"README install-hooks":         docSection(t, readme, "overlay install-hooks\n", "overlay uninstall-hooks\n"),
		"README uninstall-hooks":       docSection(t, readme, "overlay uninstall-hooks\n", "overlay status-hooks\n"),
		"README status-hooks":          docSection(t, readme, "overlay status-hooks\n", "overlay doctor"),
		"README table install-hooks":   docSection(t, readme, "| `install-hooks` |", "| `uninstall-hooks` |"),
		"README table uninstall-hooks": docSection(t, readme, "| `uninstall-hooks` |", "| `status-hooks` |"),
		"README table status-hooks":    docSection(t, readme, "| `status-hooks` |", "| `doctor [--fix]` |"),
		"install-hooks summary":        docSection(t, overlay, `echo "install-hooks complete."`, "Run 'labdrian doctor'"),
		"uninstall-hooks summary":      docSection(t, overlay, `echo "uninstall-hooks complete."`, "Run 'labdrian doctor'"),
		"--help for install-hooks":     docSection(t, overlay, "  install-hooks                    Build + deploy", "  uninstall-hooks "),
		"--help for uninstall-hooks":   docSection(t, overlay, "  uninstall-hooks                  Remove our hook entries", "  status-hooks "),
		"--help for status-hooks":      docSection(t, overlay, "  status-hooks                     Read-only check", "  doctor "),
	}
	for name, text := range sections {
		if !strings.Contains(text, "builder guard") {
			t.Errorf("%s does not mention the builder guard", name)
		}
	}
	for _, name := range []string{"README install-hooks", "install-hooks summary", "--help for install-hooks"} {
		if text := sections[name]; !strings.Contains(text, "Agent") {
			t.Errorf("%s does not say the guard is wired into the Agent tool", name)
		}
	}
	for name, text := range map[string]string{
		"README roles verbs": docSection(t, readme, "overlay roles <verb>\n", "overlay review <verb>\n"),
		"--help for roles":   docSection(t, overlay, "  roles <verb>                     Forward reusable-role", "  review <verb> "),
	} {
		if !strings.Contains(text, "builder-guard") {
			t.Errorf("%s does not mention the verb builder-guard", name)
		}
	}
}

// The shaper marks the tasks it designed, and the three role files carry the line.
func TestTheShaperRoleFilesTellItToMarkTheTasksItDesigned(t *testing.T) {
	for _, name := range []string{"roles/shaper/body.md", "agents/shaper.md", "pi/agents/shaper.md", "codex/agents/shaper.toml"} {
		text := readRepositoryFile(t, name)
		for _, want := range []string{"shaped: L<n>", "(<date>, shaper)", "never mark a task you did not design"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", name, want)
			}
		}
	}
}

// The CHANGELOG entry says what changed for a user of an existing install: the new Agent entry, the verb, the rule the
// shaper and the document follow, the limits, and that status-hooks warns until install-hooks is run again.
func TestChangelogRecordsTheBuilderGuard(t *testing.T) {
	entry := docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- `labdrian roles builder-guard`", "\n- ")

	for _, want := range []string{
		"own-rdd T11",
		"PreToolUse",
		"Agent",
		"shaped: L<n>",
		"install-hooks",
		"uninstall-hooks",
		"status-hooks",
		"restart Claude Code",
		"speed bump",
		"Codex and Pi",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("the CHANGELOG entry of the builder guard does not say %q", want)
		}
	}
}
