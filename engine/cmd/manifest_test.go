package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// Tests of the command line of 'manifest routes' and of what it tells. What the routing decides is
// tested where it is decided (engine/skills); here is what the command takes and what it says.

// manifestPorts is a manifest at /o/overlay.manifest, and nothing else.
func manifestPorts(manifest string) app.RoutesPorts {
	return app.RoutesPorts{Manifest: func(name string) ([]byte, error) {
		if name != "/o/overlay.manifest" {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return []byte(manifest), nil
	}}
}

// manifestRun is one run of the command: what it wrote on each stream and the exit it asked for.
type manifestRun struct {
	stdout, stderr string
	exit           int
}

func runManifestWith(ports app.RoutesPorts, args ...string) manifestRun {
	var stdout, stderr bytes.Buffer
	exit := -1
	runManifestCore(ports, args, &stdout, &stderr, func(code int) { exit = code })
	return manifestRun{stdout.String(), stderr.String(), exit}
}

// claudeArgs are the arguments of a run for Claude over the manifest of manifestPorts.
var claudeArgs = []string{"routes", "--overlay-root", "/o", "--target", "claude", "--skills-root", "/h/.claude/skills", "--agents-root", "/h/.claude/agents"}

func TestManifestRoutesTellsEachRowInFiveColumnsSeparatedByATab(t *testing.T) {
	run := runManifestWith(manifestPorts("# rows\n\na/SKILL.md managed\nGADU.md custom agent\nengine/go.mod managed\nlongterm-mem/go.mod custom mcp\n"), claudeArgs...)
	want := "a/SKILL.md\tskill\tskills/a/SKILL.md\t/o/skills/a/SKILL.md\t/h/.claude/skills/a/SKILL.md\n" +
		"GADU.md\tagent\tagents/GADU.md\t/o/agents/GADU.md\t/h/.claude/agents/GADU.md\n" +
		"engine/go.mod\tnon-deployable\tengine/go.mod\t\t\n" +
		"longterm-mem/go.mod\tmcp\tlongterm-mem/go.mod\t/o/longterm-mem/go.mod\t\n"
	if run.stdout != want || run.stderr != "" || run.exit != 0 {
		t.Errorf("run = %q, %q, exit %d, want %q on stdout, nothing on stderr and exit 0", run.stdout, run.stderr, run.exit, want)
	}
}

func TestManifestRoutesTellsNothingForAManifestWithNoRow(t *testing.T) {
	run := runManifestWith(manifestPorts("# nothing here\n\n"), claudeArgs...)
	if run.stdout != "" || run.stderr != "" || run.exit != 0 {
		t.Errorf("run = %+v, want an empty answer and exit 0", run)
	}
}

func TestManifestRoutesReadsAManifestThatIsNotThereAsOneWithNoRowAndSaysSo(t *testing.T) {
	args := []string{"routes", "--overlay-root", "/elsewhere", "--target", "claude", "--skills-root", "/s"}
	run := runManifestWith(manifestPorts(""), args...)
	want := "warning: manifest routes: no manifest at \"/elsewhere/overlay.manifest\": it has no row to route\n"
	if run.stdout != "" || run.stderr != want || run.exit != 0 {
		t.Errorf("run = %+v, want nothing on stdout, %q on stderr and exit 0: the script always read a manifest that is not there as one with no row", run, want)
	}
}

func TestManifestRoutesKeepsTheManagedRowsWhenAsked(t *testing.T) {
	run := runManifestWith(manifestPorts("a/SKILL.md managed\nb/SKILL.md custom\nc/SKILL.md\n"), append(claudeArgs, "--managed")...)
	if run.stdout != "a/SKILL.md\tskill\tskills/a/SKILL.md\t/o/skills/a/SKILL.md\t/h/.claude/skills/a/SKILL.md\n" || run.exit != 0 {
		t.Errorf("run = %+v, want the managed row alone", run)
	}
}

func TestManifestRoutesAsksForTheDestinationsOfTheTarget(t *testing.T) {
	args := []string{"routes", "--overlay-root", "/o", "--target", "opencode", "--skills-root", "/h/oc/skills", "--agents-root", "/h/oc/agents"}
	run := runManifestWith(manifestPorts("a/SKILL.md managed\nGADU.md custom agent\nopencode/agents/G.md custom opencode-agent\n"), args...)
	want := "a/SKILL.md\tskill\tskills/a/SKILL.md\t/o/skills/a/SKILL.md\t/h/oc/skills/a/SKILL.md\n" +
		"GADU.md\tagent\tagents/GADU.md\t/o/agents/GADU.md\t\n" +
		"opencode/agents/G.md\topencode-agent\topencode/agents/G.md\t/o/opencode/agents/G.md\t/h/oc/agents/G.md\n"
	if run.stdout != want || run.exit != 0 {
		t.Errorf("run = %+v, want %q", run, want)
	}
}

func TestManifestRoutesReadsTheManifestItIsNamedOrThatOfTheOverlay(t *testing.T) {
	ports := app.RoutesPorts{Manifest: func(name string) ([]byte, error) {
		return []byte(name + " managed\n"), nil
	}}
	named := runManifestWith(ports, append(claudeArgs, "--manifest", "/m/other.manifest")...)
	if !strings.HasPrefix(named.stdout, "/m/other.manifest\t") {
		t.Errorf("with --manifest the command read %q", named.stdout)
	}
	if byDefault := runManifestWith(ports, claudeArgs...); !strings.HasPrefix(byDefault.stdout, "/o/overlay.manifest\t") {
		t.Errorf("without --manifest the command read %q, want the overlay.manifest of the overlay root", byDefault.stdout)
	}
}

func TestManifestRoutesRefusesWhatItCannotPlaceAndTellsNothingOnStdout(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ports      app.RoutesPorts
		args       []string
		wantStderr string
	}{
		{"a longterm-mem row with no route", manifestPorts("a/SKILL.md managed\nlongterm-mem/internal/foo.go custom\n"), claudeArgs,
			"route_resolve: longterm-mem row 'longterm-mem/internal/foo.go' declares no route (missing third column); must be one of: skill, agent, opencode-agent, mcp\n"},
		{"a longterm-mem row with a route nobody knows", manifestPorts("a/SKILL.md managed\nlongterm-mem/internal/bar.go custom bogus\n"), claudeArgs,
			"route_resolve: longterm-mem row 'longterm-mem/internal/bar.go' declares an unrecognized route 'bogus'; must be one of: skill, agent, opencode-agent, mcp\n"},
		{"a manifest that is there and cannot be read", app.RoutesPorts{Manifest: func(name string) ([]byte, error) {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
		}}, claudeArgs, `error: manifest routes: reading manifest "/o/overlay.manifest": open /o/overlay.manifest: permission denied` + "\n"},
		{"a target nobody knows", manifestPorts("a/SKILL.md managed\n"), []string{"routes", "--overlay-root", "/o", "--target", "windsurf", "--skills-root", "/s"},
			"error: manifest routes: unknown target \"windsurf\"; must be one of: claude, codex, pi, opencode\n"},
		{"an agent row and no agents directory", manifestPorts("GADU.md custom agent\n"), []string{"routes", "--overlay-root", "/o", "--target", "claude", "--skills-root", "/s"},
			"error: manifest routes: manifest row 'GADU.md' is copied to the agents directory of claude, and none was given\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runManifestWith(tc.ports, tc.args...)
			if run.stdout != "" || run.stderr != tc.wantStderr || run.exit != 1 {
				t.Errorf("run = %+v, want nothing on stdout, %q on stderr and exit 1", run, tc.wantStderr)
			}
		})
	}
}

func TestManifestRoutesRefusesAnUnusableCommandLine(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"no verb", nil, "error: manifest requires a verb: routes\n"},
		{"a verb nobody knows", []string{"rows"}, "error: unknown manifest verb \"rows\"; expected routes\n"},
		{"no flags", []string{"routes"}, "error: manifest routes requires --overlay-root, --target, and --skills-root\n"},
		{"no target", []string{"routes", "--overlay-root", "/o", "--skills-root", "/s"}, "error: manifest routes requires --overlay-root, --target, and --skills-root\n"},
		{"a flag nobody named", append([]string{"routes", "--rows"}, claudeArgs[1:]...), "error: manifest routes: unknown argument \"--rows\"\n"},
		{"a word that is no flag", append([]string{"routes", "claude"}, claudeArgs[1:]...), "error: manifest routes: unknown argument \"claude\"\n"},
		{"a flag with no value", []string{"routes", "--overlay-root", "/o", "--target"}, "error: manifest routes: flag \"--target\" requires a value\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runManifestWith(manifestPorts("a/SKILL.md managed\n"), tc.args...)
			if run.stdout != "" || run.stderr != tc.wantStderr || run.exit != 1 {
				t.Errorf("run = %+v, want nothing on stdout, %q on stderr and exit 1", run, tc.wantStderr)
			}
		})
	}
}

func TestManifestRoutesIsReachedThroughTheProgramOverTheFilesOfTheMachine(t *testing.T) {
	overlay := t.TempDir()
	writeTestFile(t, filepath.Join(overlay, "overlay.manifest"), "a/SKILL.md managed\n")
	p := newCapturedProcess()
	run(p.process, testDeps(), []string{"manifest", "routes", "--overlay-root", overlay, "--target", "codex", "--skills-root", "/h/.codex/skills"})
	want := "a/SKILL.md\tskill\tskills/a/SKILL.md\t" + overlay + "/skills/a/SKILL.md\t/h/.codex/skills/a/SKILL.md\n"
	if p.out.String() != want || p.err.String() != "" || !equalInts(p.exits, []int{0}) {
		t.Errorf("run printed %q on stdout, %q on stderr, exits %v, want %q, nothing, [0]", p.out.String(), p.err.String(), p.exits, want)
	}
	if _, err := os.Stat(filepath.Join(overlay, "overlay.manifest")); err != nil {
		t.Errorf("the manifest is read and never written: %v", err)
	}
}

func TestUsageNamesTheRoutesManifestRoutesTells(t *testing.T) {
	// The routes the command tells are the domain's, and the usage text says each of them.
	section := docSection(t, captureUsage(t), "  engine manifest routes", "  engine skills <verb>")
	for _, route := range []string{skills.RouteSkill, skills.RouteAgent, skills.RouteOpencodeAgent, skills.RouteMCP, skills.RouteNonDeployable} {
		if !strings.Contains(section, route) {
			t.Errorf("the usage text of 'manifest routes' does not name the route %q", route)
		}
	}
}
