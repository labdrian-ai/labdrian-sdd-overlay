package presence_test

// Tests for the presence prober. Every file the prober is pointed at is a
// fixture under t.TempDir(); no test names the real home directory, and none
// looks for a real credentials file. The prober's contract is that it only ever
// stats, so most tests also inject a recording StatFS to see exactly which
// paths were touched and how.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// secretMarker is written into every fixture file. The prober cannot read it;
// the tests check that it never appears in an observation, and the static test
// proves the source has no way to open a file at all.
const secretMarker = "SECRET-CONTENT-MUST-NEVER-BE-READ"

func writeFixture(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(secretMarker), mode); err != nil {
		t.Fatal(err)
	}
}

// fixtureHome returns a home directory holding every file signal.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, rel := range [][]string{
		{".engram", "engram.db"},
		{".labdrian-overlay", "longterm-mem-registration.json"},
		{".claude", ".credentials.json"},
		{".codex", "auth.json"},
		{".pi", "agent", "auth.json"},
	} {
		writeFixture(t, filepath.Join(append([]string{home}, rel...)...), 0o600)
	}
	return home
}

// binDir returns a directory holding an executable gentle-ai.
func binDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "gentle-ai"
	if runtime.GOOS == "windows" {
		name = "gentle-ai.exe"
	}
	writeFixture(t, filepath.Join(dir, name), 0o755)
	return dir
}

// recordingFS forwards to the operating system and records every call.
type recordingFS struct {
	calls []string
}

func (r *recordingFS) Lstat(name string) (fs.FileInfo, error) {
	r.calls = append(r.calls, "lstat "+name)
	return os.Lstat(name)
}

func (r *recordingFS) Stat(name string) (fs.FileInfo, error) {
	r.calls = append(r.calls, "stat "+name)
	return os.Stat(name)
}

// failingFS answers every call with err.
type failingFS struct {
	err   error
	calls int
}

func (f *failingFS) Lstat(string) (fs.FileInfo, error) { f.calls++; return nil, f.err }
func (f *failingFS) Stat(string) (fs.FileInfo, error)  { f.calls++; return nil, f.err }

func probeOne(t *testing.T, p presence.Prober, name string) workflow.Observation {
	t.Helper()
	got, err := p.Probe(context.Background(), []string{name})
	if err != nil || len(got) != 1 {
		t.Fatalf("Probe(%q) = %v, %v, want one observation and no error", name, got, err)
	}
	if got[0].Capability != name {
		t.Fatalf("observation capability = %q, want %q", got[0].Capability, name)
	}
	return got[0]
}

func TestPresenceProberReportsEachFileSignalFromTheFixtureHome(t *testing.T) {
	home := fixtureHome(t)
	p := presence.Prober{Home: home}
	for name, want := range map[string]string{
		"memory:engram":           "the Engram database file is present (not opened, so its contents and health are unverified)",
		"memory:longterm-mem":     "the longterm-mem registration record is present (not opened; whether a runtime has the MCP server loaded is unverified)",
		"credentials:claude-code": "the Claude Code credentials file is present (not opened; this does not prove the runtime is authenticated)",
		"credentials:codex":       "the Codex credentials file is present (not opened; this does not prove the runtime is authenticated)",
		"credentials:pi":          "the Pi credentials file is present (not opened; this does not prove the runtime is authenticated)",
	} {
		t.Run(name, func(t *testing.T) {
			got := probeOne(t, p, name)
			if got.Status != workflow.ObservationAvailable || got.Detail != want {
				t.Errorf("observation = %+v, want available with detail %q", got, want)
			}
		})
	}
}

func TestPresenceProberNeverReportsAuthenticatedOrHealthy(t *testing.T) {
	p := presence.Prober{Home: fixtureHome(t), Path: binDir(t)}
	names := []string{"memory:engram", "memory:longterm-mem", "gentle-ai-review", "credentials:claude-code", "credentials:codex", "credentials:pi"}
	got, err := p.Probe(context.Background(), names)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got {
		if o.Status != workflow.ObservationAvailable {
			t.Fatalf("observation %+v is not available; the fixture has every signal", o)
		}
		lower := strings.ToLower(o.Detail)
		if !strings.Contains(lower, "not opened") && !strings.Contains(lower, "not executed") {
			t.Errorf("detail %q does not state that nothing was opened or executed", o.Detail)
		}
		// The one permitted mention of authentication is the disclaimer itself.
		claim := strings.ReplaceAll(lower, "this does not prove the runtime is authenticated", "")
		for _, banned := range []string{"authenticated", "is healthy", "logged in", secretMarker} {
			if strings.Contains(claim, strings.ToLower(banned)) {
				t.Errorf("detail %q claims or leaks %q", o.Detail, banned)
			}
		}
	}
}

func TestPresenceProberReportsAMissingFileAsUnavailableWithoutThePath(t *testing.T) {
	home := t.TempDir()
	p := presence.Prober{Home: home}
	for name, subject := range map[string]string{
		"memory:engram":           "the Engram database file was not found",
		"memory:longterm-mem":     "the longterm-mem registration record was not found",
		"credentials:claude-code": "the Claude Code credentials file was not found",
		"credentials:codex":       "the Codex credentials file was not found",
		"credentials:pi":          "the Pi credentials file was not found",
	} {
		got := probeOne(t, p, name)
		if got.Status != workflow.ObservationUnavailable || !strings.HasPrefix(got.Detail, subject) {
			t.Errorf("%s: observation = %+v, want unavailable starting %q", name, got, subject)
		}
		if strings.Contains(got.Detail, home) || strings.Contains(got.Detail, "auth.json") || strings.Contains(got.Detail, ".credentials.json") {
			t.Errorf("%s: detail %q names a path", name, got.Detail)
		}
	}
}

func TestPresenceProberFixedSignalsAndUnknownCapabilities(t *testing.T) {
	p := presence.Prober{Home: fixtureHome(t), Path: binDir(t)}
	if got := probeOne(t, p, "memory:procedural-skills"); got.Status != workflow.ObservationUnavailable || got.Detail != "no presence check exists for procedural skills" {
		t.Errorf("procedural skills = %+v, want unavailable with the fixed detail even when everything else is present", got)
	}
	for _, name := range []string{"memory:other", "credentials:opencode", "credentials:", "", "gentle-ai", "GENTLE-AI-REVIEW"} {
		got := probeOne(t, p, name)
		if got.Status != workflow.ObservationUnavailable || got.Detail != "no presence check exists for this capability" {
			t.Errorf("%q = %+v, want unavailable with the fixed detail", name, got)
		}
	}
}

func TestPresenceProberAnswersInTheOrderAndLengthOfTheRequest(t *testing.T) {
	p := presence.Prober{Home: fixtureHome(t)}
	names := []string{"credentials:pi", "nope", "memory:engram", "credentials:pi"}
	got, err := p.Probe(context.Background(), names)
	if err != nil || len(got) != len(names) {
		t.Fatalf("Probe() = %v, %v, want %d observations", got, err, len(names))
	}
	for i, name := range names {
		if got[i].Capability != name {
			t.Errorf("observation %d is for %q, want %q", i, got[i].Capability, name)
		}
	}
	empty, err := p.Probe(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("Probe(nil) = %v, %v, want no observations and no error", empty, err)
	}
}

func TestPresenceProberWithoutAHomeReportsEveryHomeSignalUnavailable(t *testing.T) {
	for name, home := range map[string]string{"empty": "", "relative": "some/relative/home"} {
		t.Run(name, func(t *testing.T) {
			rec := &recordingFS{}
			p := presence.Prober{Home: home, ProbeFS: rec}
			for _, signal := range []string{"memory:engram", "memory:longterm-mem", "credentials:claude-code", "credentials:codex", "credentials:pi"} {
				got := probeOne(t, p, signal)
				if got.Status != workflow.ObservationUnavailable || !strings.Contains(got.Detail, "home directory is unknown") {
					t.Errorf("%s = %+v, want unavailable because the home is unknown", signal, got)
				}
			}
			if len(rec.calls) != 0 {
				t.Errorf("the prober touched %v with no usable home; it must not guess a location", rec.calls)
			}
		})
	}
}

func TestPresenceProberFindsGentleAIOnAnAbsolutePathEntry(t *testing.T) {
	good := binDir(t)
	empty := t.TempDir()
	list := string(os.PathListSeparator)
	for name, tc := range map[string]struct {
		path string
		want bool
	}{
		"first entry":            {good + list + empty, true},
		"later entry":            {empty + list + good, true},
		"an entry without it":    {empty, false},
		"empty PATH":             {"", false},
		"a missing directory":    {filepath.Join(empty, "missing"), false},
		"only relative entries":  {"." + list + "bin" + list + filepath.Join("a", "b"), false},
		"empty entries skipped":  {list + list + empty, false},
		"relative then absolute": {"bin" + list + good, true},
	} {
		t.Run(name, func(t *testing.T) {
			got := probeOne(t, presence.Prober{Path: tc.path}, "gentle-ai-review")
			wantStatus := workflow.ObservationUnavailable
			if tc.want {
				wantStatus = workflow.ObservationAvailable
			}
			if got.Status != wantStatus {
				t.Errorf("observation = %+v, want %s", got, wantStatus)
			}
			if tc.want && got.Detail != "the gentle-ai binary is on PATH (not executed; review mode and consent are unverified)" {
				t.Errorf("detail = %q, want the fixed stat-level sentence", got.Detail)
			}
			if !tc.want && !strings.HasPrefix(got.Detail, "the gentle-ai binary was not found in the absolute PATH directories") {
				t.Errorf("detail = %q, want the not-found reason", got.Detail)
			}
		})
	}
}

// TestPresenceProberDoesNotNeedAHomeToFindTheBinary: the binary check depends on
// PATH only, so an unknown home does not hide a binary that is plainly there.
func TestPresenceProberDoesNotNeedAHomeToFindTheBinary(t *testing.T) {
	got := probeOne(t, presence.Prober{Home: "", Path: binDir(t)}, "gentle-ai-review")
	if got.Status != workflow.ObservationAvailable {
		t.Errorf("observation = %+v, want available: PATH alone decides", got)
	}
}

func TestPresenceProberSkipsRelativePathEntriesWithoutTouchingThem(t *testing.T) {
	rec := &recordingFS{}
	list := string(os.PathListSeparator)
	p := presence.Prober{Path: "bin" + list + "." + list + list + filepath.Join("x", "y"), ProbeFS: rec}
	got := probeOne(t, p, "gentle-ai-review")
	if got.Status != workflow.ObservationUnavailable {
		t.Errorf("observation = %+v, want unavailable", got)
	}
	if len(rec.calls) != 0 {
		t.Errorf("the prober touched %v; a relative PATH entry depends on a working directory it does not know", rec.calls)
	}
}

func TestPresenceProberRequiresAnExecutableRegularFileForTheBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bits do not exist on this platform")
	}
	notExec := t.TempDir()
	writeFixture(t, filepath.Join(notExec, "gentle-ai"), 0o644)
	asDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(asDir, "gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{"a file without the execute bit": notExec, "a directory": asDir} {
		got := probeOne(t, presence.Prober{Path: dir}, "gentle-ai-review")
		if got.Status != workflow.ObservationUnavailable {
			t.Errorf("%s: observation = %+v, want unavailable", name, got)
		}
	}
	// A non-executable file earlier on PATH does not hide an executable later on.
	got := probeOne(t, presence.Prober{Path: notExec + string(os.PathListSeparator) + binDir(t)}, "gentle-ai-review")
	if got.Status != workflow.ObservationAvailable {
		t.Errorf("observation = %+v, want available: the scan goes on past a file that cannot run", got)
	}
}

// TestPresenceProberOnlyStatsTheExpectedPaths: the recorded calls are the whole
// of the prober's access to the disk. Each is a Lstat or a Stat of the exact
// file signal or of <dir>/gentle-ai, never a directory listing or a path
// derived from a file's contents.
func TestPresenceProberOnlyStatsTheExpectedPaths(t *testing.T) {
	home := fixtureHome(t)
	bin := binDir(t)
	rec := &recordingFS{}
	p := presence.Prober{Home: home, Path: bin, ProbeFS: rec}
	names := []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills", "gentle-ai-review", "credentials:claude-code", "credentials:codex", "credentials:pi", "unknown"}
	if _, err := p.Probe(context.Background(), names); err != nil {
		t.Fatal(err)
	}
	binary := "gentle-ai"
	if runtime.GOOS == "windows" {
		binary = "gentle-ai.exe"
	}
	want := []string{
		"lstat " + filepath.Join(home, ".engram", "engram.db"),
		"lstat " + filepath.Join(home, ".labdrian-overlay", "longterm-mem-registration.json"),
		"lstat " + filepath.Join(bin, binary),
		"lstat " + filepath.Join(home, ".claude", ".credentials.json"),
		"lstat " + filepath.Join(home, ".codex", "auth.json"),
		"lstat " + filepath.Join(home, ".pi", "agent", "auth.json"),
	}
	if strings.Join(rec.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls =\n%s\nwant\n%s", strings.Join(rec.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestPresenceProberTreatsASymlinkAsPresentOnlyIfItResolvesToARegularFile(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	writeFixture(t, target, 0o600)
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".codex", "auth.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(home, "missing-target"), filepath.Join(home, ".claude", ".credentials.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, ".pi", "agent", "auth.json")); err != nil {
		t.Fatal(err)
	}
	p := presence.Prober{Home: home}

	ok := probeOne(t, p, "credentials:codex")
	if ok.Status != workflow.ObservationAvailable || !strings.HasSuffix(ok.Detail, "; the checked path is a symlink") {
		t.Errorf("a symlink to a regular file = %+v, want available and marked as a symlink", ok)
	}
	dangling := probeOne(t, p, "credentials:claude-code")
	if dangling.Status != workflow.ObservationUnavailable || !strings.Contains(dangling.Detail, "symlink whose target cannot be followed") {
		t.Errorf("a dangling symlink = %+v, want unavailable naming the link", dangling)
	}
	toDir := probeOne(t, p, "credentials:pi")
	if toDir.Status != workflow.ObservationUnavailable {
		t.Errorf("a symlink to a directory = %+v, want unavailable", toDir)
	}
	for _, o := range []workflow.Observation{ok, dangling, toDir} {
		if strings.Contains(o.Detail, home) || strings.Contains(o.Detail, target) {
			t.Errorf("detail %q names a path", o.Detail)
		}
	}
}

func TestPresenceProberNamesTheErrorClassNotThePath(t *testing.T) {
	home := t.TempDir()
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"permission denied": {fs.ErrPermission, "permission denied"},
		"other error":       {errors.New("input/output error on " + home + "/.codex/auth.json"), "the path could not be examined"},
	} {
		t.Run(name, func(t *testing.T) {
			p := presence.Prober{Home: home, ProbeFS: &failingFS{err: tc.err}}
			got := probeOne(t, p, "credentials:codex")
			if got.Status != workflow.ObservationUnavailable || !strings.HasSuffix(got.Detail, tc.want) {
				t.Errorf("observation = %+v, want unavailable ending %q", got, tc.want)
			}
			if strings.Contains(got.Detail, home) || strings.Contains(got.Detail, "auth.json") || strings.Contains(got.Detail, "input/output") {
				t.Errorf("detail %q repeats the error text or the path", got.Detail)
			}
		})
	}
}

func TestPresenceProberCountsPathDirectoriesItCouldNotCheck(t *testing.T) {
	fsys := &failingFS{err: fs.ErrPermission}
	dirs := []string{filepath.Join(string(filepath.Separator), "a"), filepath.Join(string(filepath.Separator), "b")}
	p := presence.Prober{Path: strings.Join(dirs, string(os.PathListSeparator)), ProbeFS: fsys}
	got := probeOne(t, p, "gentle-ai-review")
	if got.Status != workflow.ObservationUnavailable || !strings.Contains(got.Detail, "(2 could not be checked)") {
		t.Errorf("observation = %+v, want unavailable and a count of the directories that could not be checked", got)
	}
	// A directory that simply lacks the binary is not "could not be checked".
	missing := probeOne(t, presence.Prober{Path: t.TempDir()}, "gentle-ai-review")
	if strings.Contains(missing.Detail, "could not be checked") {
		t.Errorf("detail %q counts a plain absence as unchecked", missing.Detail)
	}
}

func TestPresenceProberHonorsAContextThatIsAlreadyDone(t *testing.T) {
	rec := &recordingFS{}
	p := presence.Prober{Home: fixtureHome(t), Path: binDir(t), ProbeFS: rec}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := p.Probe(ctx, []string{"memory:engram", "gentle-ai-review"})
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Errorf("Probe() = %v, %v, want no observations and the context's error", got, err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("the prober touched %v after its context was done", rec.calls)
	}
}

// cancelingFS cancels its context on the first call, so the prober is stopped
// while a probe is under way.
type cancelingFS struct {
	cancel context.CancelFunc
	calls  int
}

func (c *cancelingFS) Lstat(name string) (fs.FileInfo, error) {
	c.calls++
	c.cancel()
	return os.Lstat(name)
}
func (c *cancelingFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

func TestPresenceProberStopsBetweenCapabilitiesAndBetweenPathDirectories(t *testing.T) {
	t.Run("between capabilities", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fsys := &cancelingFS{cancel: cancel}
		p := presence.Prober{Home: fixtureHome(t), ProbeFS: fsys}
		got, err := p.Probe(ctx, []string{"memory:engram", "memory:longterm-mem", "credentials:pi"})
		if !errors.Is(err, context.Canceled) || got != nil {
			t.Errorf("Probe() = %v, %v, want the context's error", got, err)
		}
		if fsys.calls != 1 {
			t.Errorf("the prober made %d calls, want it to stop after the one that was under way", fsys.calls)
		}
	})
	t.Run("between PATH directories", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fsys := &cancelingFS{cancel: cancel}
		list := string(os.PathListSeparator)
		p := presence.Prober{Path: t.TempDir() + list + t.TempDir() + list + binDir(t), ProbeFS: fsys}
		got, err := p.Probe(ctx, []string{"gentle-ai-review"})
		if err != nil || len(got) != 1 {
			t.Fatalf("Probe() = %v, %v, want one observation", got, err)
		}
		if got[0].Status != workflow.ObservationUnavailable || !strings.Contains(got[0].Detail, "cut short") {
			t.Errorf("observation = %+v, want unavailable, cut short by the context", got[0])
		}
		if fsys.calls != 1 {
			t.Errorf("the scan made %d calls, want it to stop after the first directory", fsys.calls)
		}
	})
}

func TestPresenceProberDetailsStayWithinTheObservationBounds(t *testing.T) {
	home := fixtureHome(t)
	for _, p := range []presence.Prober{
		{Home: home, Path: binDir(t)},
		{},
		{Home: home, ProbeFS: &failingFS{err: errors.New(strings.Repeat("x", 10000))}},
	} {
		names := []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills", "gentle-ai-review", "credentials:claude-code", "credentials:codex", "credentials:pi", "other"}
		got, err := p.Probe(context.Background(), names)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range got {
			if len([]rune(o.Capability)) > workflow.MaxObservationCapabilityLength || len([]rune(o.Detail)) > workflow.MaxObservationDetailLength || o.Detail == "" {
				t.Errorf("observation %+v is outside the workflow bounds or has no detail", o)
			}
			if o.Status != workflow.ObservationAvailable && o.Status != workflow.ObservationUnavailable {
				t.Errorf("observation %+v has an unknown status", o)
			}
		}
	}
}

// TestPresenceCapabilityNamesMatchWhatTheLifecycleRequests pins the literals
// against the vocabulary they mirror: the memory names are "memory:" plus a
// memoryscope source.
func TestPresenceCapabilityNamesMatchWhatTheLifecycleRequests(t *testing.T) {
	for source, name := range map[memoryscope.Source]string{
		memoryscope.SourceEngram:           presence.CapabilityMemoryEngram,
		memoryscope.SourceLongtermMem:      presence.CapabilityMemoryLongtermMem,
		memoryscope.SourceProceduralSkills: presence.CapabilityMemoryProcedural,
	} {
		if want := "memory:" + string(source); name != want {
			t.Errorf("capability name %q, want %q", name, want)
		}
	}
	if presence.CapabilityGentleAIReview != "gentle-ai-review" {
		t.Errorf("gentle-ai-review name = %q", presence.CapabilityGentleAIReview)
	}
}

func TestCredentialsCapabilityMapsTheThreeRuntimesThatHaveOne(t *testing.T) {
	for target, want := range map[string]string{
		capability.TargetClaude: "credentials:claude-code",
		capability.TargetCodex:  "credentials:codex",
		capability.TargetPi:     "credentials:pi",
	} {
		if got, ok := presence.CredentialsCapability(target); !ok || got != want {
			t.Errorf("CredentialsCapability(%q) = %q, %v, want %q", target, got, ok, want)
		}
	}
	for _, target := range []string{capability.TargetOpenCode, "", "all", "claude-code"} {
		if got, ok := presence.CredentialsCapability(target); ok || got != "" {
			t.Errorf("CredentialsCapability(%q) = %q, %v, want none", target, got, ok)
		}
	}
}

// TestPresenceProberSatisfiesTheLifecycleDependencyProber is checked at compile
// time by the assertion in the source; this test keeps the behavior of the zero
// value, which is the safe default when nothing is configured.
func TestPresenceProberZeroValueReportsEverythingUnavailable(t *testing.T) {
	names := []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills", "gentle-ai-review", "credentials:claude-code"}
	got, err := presence.Prober{}.Probe(context.Background(), names)
	if err != nil {
		t.Fatal(err)
	}
	var _ workflow.DependencyProber = presence.Prober{}
	for _, o := range got {
		if o.Status != workflow.ObservationUnavailable {
			t.Errorf("zero prober reported %+v, want everything unavailable", o)
		}
	}
}
