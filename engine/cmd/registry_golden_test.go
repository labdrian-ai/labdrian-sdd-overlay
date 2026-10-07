package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The golden files under testdata/registry-golden record what every verb that reads or writes
// the skills registry (skills.registry.yaml) prints and leaves behind: 'skills list', 'status',
// 'validate', 'add', 'remove', 'sync-manifest', 'install', 'adopt', 'project-register',
// 'project-status' and 'project-retire' (and 'project-revise', which accepts a registry it does
// not read), 'pipkg build' and 'pipkg check', and the Pi runtime verbs, which build and compare
// the same package. They were recorded from the program as it was before Phase 9 unit H15
// (docs/architecture/hexagonal-target.md) put the registry behind a port and moved its YAML
// reader and writer to engine/skills/registryyaml, and they are the contract that move had to
// keep: what each verb says for a registry it can read, and the words of every refusal of one
// it cannot (a missing file, a line it does not understand, a value outside the vocabulary).
// Decisions of the owner have changed some of them since, on purpose (the Phase 9 ledger in
// odd/tasks says which and why), and each such change was read in its diff.
// A change to a byte of any of them fails here. Rewrite them deliberately with
//
//	go test ./cmd -run TestRegistryGolden -update-registry-golden
//
// and read the diff before committing it.
//
// Each case runs the built program in a throwaway world (the engine binary, once for the whole
// test run, as the review-receipt goldens do), records the command line, the exit code and both
// streams of every invocation, and, for a verb that writes, the files it left. The temporary
// directory is written as <WORLD>. Each case is its own subtest.
var updateRegistryGolden = flag.Bool("update-registry-golden", false, "rewrite the golden files of the registry-reading verbs")

// registryGoldenFileName is what a case name may not contain: it is the name of its file.
var registryGoldenFileName = regexp.MustCompile(`[^a-z0-9-]+`)

// registryRunTimeout is how long one invocation of the program may take: far longer than any case
// needs, and short enough that a hang fails its own case instead of the whole test run.
const registryRunTimeout = 2 * time.Minute

// registryWorld is the scratch space of one case: the directory the program runs in (an overlay,
// or a project), the program, and the transcript.
type registryWorld struct {
	t     *testing.T
	bin   string
	dir   string
	names map[string]string
	env   []string
	b     strings.Builder
	// filter, when it is not nil, rewrites the transcript: a case whose output holds what
	// differs from one run to the next (the digests of commits) says how to hide it.
	filter func(string) string
}

func newRegistryWorld(t *testing.T) *registryWorld {
	t.Helper()
	w := &registryWorld{t: t, bin: reviewReceiptBinary(t), dir: t.TempDir(), names: map[string]string{}, env: goldenEnvironment()}
	w.name(w.dir, "<WORLD>")
	// No case runs a Pi that is installed on the machine, or reads the home of the machine.
	w.setenv("LABDRIAN_PI_BIN", filepath.Join(w.dir, "no-pi"))
	w.setenv("HOME", filepath.Join(w.dir, "home"))
	if real, err := filepath.EvalSymlinks(w.dir); err == nil {
		w.name(real, "<WORLD>")
	}
	return w
}

// name registers path to be written as placeholder in the transcript.
func (w *registryWorld) name(path, placeholder string) { w.names[path] = placeholder }

// path is the place rel names in the world.
func (w *registryWorld) path(rel string) string { return filepath.Join(w.dir, filepath.FromSlash(rel)) }

// put writes content to rel, making the directories on the way.
func (w *registryWorld) put(rel, content string) {
	w.t.Helper()
	p := w.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
	// The mode is set, not asked for: a mask of the process must not change the modes a case
	// records of the files it copies.
	if err := os.Chmod(p, 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// mkdir makes the directory rel.
func (w *registryWorld) mkdir(rel string) {
	w.t.Helper()
	if err := os.MkdirAll(w.path(rel), 0o755); err != nil {
		w.t.Fatal(err)
	}
}

// read is the content of rel, or "" when there is none.
func (w *registryWorld) read(rel string) string {
	data, err := os.ReadFile(w.path(rel))
	if err != nil {
		return ""
	}
	return string(data)
}

// setenv adds name=value to the environment of the program.
func (w *registryWorld) setenv(name, value string) { w.env = append(w.env, name+"="+value) }

func (w *registryWorld) write(format string, args ...any) { fmt.Fprintf(&w.b, format, args...) }

// label puts a line in the transcript that says what the next invocation shows.
func (w *registryWorld) label(format string, args ...any) {
	w.write("# "+format+"\n", args...)
}

// registryAt writes registry text to rel in the world and returns the path the program is
// given for it: an absolute one, which the transcript shows as <WORLD>/rel.
func (w *registryWorld) registryAt(rel, text string) string {
	w.t.Helper()
	w.put(rel, text)
	return w.path(rel)
}

// run records one invocation of the program, started in the directory cwd of the world ("." for
// the world itself), with arguments in which the world is written as it is: the arguments are
// the real ones, and the transcript shows them with <WORLD> in its place.
func (w *registryWorld) runIn(cwd string, args ...string) {
	w.t.Helper()
	code, stdout, stderr, err := runWithin(registryRunTimeout, w.bin, w.path(cwd), w.env, args)
	if errors.Is(err, errRunTimedOut) {
		w.t.Fatalf("run %v: the program did not finish in %v (stdout %q, stderr %q)", args, registryRunTimeout, stdout, stderr)
	}
	if err != nil {
		w.t.Fatalf("run %v: %v", args, err)
	}
	where := ""
	if cwd != "." {
		where = " (in " + cwd + ")"
	}
	w.write("$ %s%s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s\n", strings.Join(args, " "), where, code, ensureNewline(stdout), ensureNewline(stderr))
}

// errRunTimedOut is what runWithin says of a program that was still running at its deadline.
var errRunTimedOut = errors.New("the program did not finish in time")

// runWithin runs bin with args in dir and env and reports its exit code and both streams. A program
// that is still running after timeout is killed and reported as errRunTimedOut, with what it had
// printed; a program that exits with a code other than 0 is not an error.
func runWithin(timeout time.Duration, bin, dir string, env, args []string) (code int, stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runUnder(ctx, bin, dir, env, args)
}

// runUnder is runWithin for a program that is killed when ctx ends, whether its deadline came or
// the caller cancelled it (a test that kills the program once it has seen it is ready).
func runUnder(ctx context.Context, bin, dir string, env, args []string) (code int, stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	// The kill at the deadline reaches what the program started (ownProcessGroup), and a
	// descendant that still holds the output pipes does not hold the run for longer than this.
	ownProcessGroup(cmd)
	cmd.WaitDelay = 10 * time.Second
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	runErr := cmd.Run()
	switch {
	case ctx.Err() != nil:
		return 0, out.String(), errOut.String(), errRunTimedOut
	case runErr == nil:
		return 0, out.String(), errOut.String(), nil
	}
	var exit *exec.ExitError
	if !errors.As(runErr, &exit) {
		return 0, out.String(), errOut.String(), runErr
	}
	return exit.ExitCode(), out.String(), errOut.String(), nil
}

// A program that hangs fails its own run, with what it had printed, instead of holding the whole
// test run until the blanket timeout of go test. The program here would sleep for an hour: that the
// run returns at all, with errRunTimedOut, is what shows the deadline ended it, so nothing compares
// a duration with a margin; the wait for the answer is a bound on a failure and no part of a pass.
func TestARunThatHangsIsKilledAtItsDeadline(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to stand for a program that hangs")
	}
	type answer struct {
		code   int
		stdout string
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		code, stdout, _, err := runWithin(200*time.Millisecond, sh, t.TempDir(), nil, []string{"-c", "echo started; exec sleep 3600"})
		answered <- answer{code, stdout, err}
	}()
	select {
	case got := <-answered:
		if !errors.Is(got.err, errRunTimedOut) {
			t.Fatalf("runWithin() = %d, %q, %v, want errRunTimedOut", got.code, got.stdout, got.err)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("runWithin() did not return for a program that sleeps for an hour: the deadline did not end it")
	}
	if code, _, _, err := runWithin(time.Minute, sh, t.TempDir(), nil, []string{"-c", "exit 3"}); err != nil || code != 3 {
		t.Errorf("runWithin() of a program that exits 3 = %d, %v, want the code and no error", code, err)
	}
}

// run records one invocation started in the world itself.
func (w *registryWorld) run(args ...string) {
	w.t.Helper()
	w.runIn(".", args...)
}

// show records the content of the file rel, or that there is none.
func (w *registryWorld) show(rel string) {
	w.t.Helper()
	data, err := os.ReadFile(w.path(rel))
	switch {
	case err == nil:
		w.write("--- %s ---\n%s\n", rel, ensureNewline(string(data)))
	case errors.Is(err, fs.ErrNotExist):
		w.write("--- %s ---\n(absent)\n\n", rel)
	default:
		w.write("--- %s ---\n(%v)\n\n", rel, err)
	}
}

// tree records the files under rel (names and sizes), in order: what a verb left.
func (w *registryWorld) tree(rel string) { w.t.Helper(); w.treeOf(rel, true) }

func (w *registryWorld) treeOf(rel string, sizes bool) {
	w.t.Helper()
	root := w.path(rel)
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(root, p)
		name = filepath.ToSlash(name)
		if d.IsDir() {
			if name != "." {
				lines = append(lines, name+"/")
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if sizes {
			name = fmt.Sprintf("%s (%d bytes)", name, info.Size())
		}
		lines = append(lines, name)
		return nil
	})
	if err != nil {
		w.write("--- tree of %s ---\n(%v)\n\n", rel, err)
		return
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		lines = []string{"(empty)"}
	}
	w.write("--- tree of %s ---\n%s\n\n", rel, strings.Join(lines, "\n"))
}

// text is the transcript, with the places of the world written as placeholders, longest first.
func (w *registryWorld) text() string {
	text := w.b.String()
	paths := make([]string, 0, len(w.names))
	for p := range w.names {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		text = strings.ReplaceAll(text, p, w.names[p])
	}
	if w.filter != nil {
		text = w.filter(text)
	}
	return visibleControls(text)
}

// registryGoldenCase is one scenario and the golden file its transcript is compared with.
type registryGoldenCase struct {
	name string
	run  func(w *registryWorld)
}

// TestRegistryGolden runs every case and compares its transcript with its golden file.
func TestRegistryGolden(t *testing.T) {
	for _, tc := range registryGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newRegistryWorld(t)
			tc.run(w)
			checkRegistryGolden(t, tc.name, w.text())
		})
	}
}

func checkRegistryGolden(t *testing.T, name, got string) {
	t.Helper()
	checkGoldenIn(t, "registry-golden", name, got, updateRegistryGolden, "-update-registry-golden")
}

// checkGoldenIn compares got with the golden file testdata/<dir>/<name>.golden, or rewrites the
// file when update is set. It is the comparison of every golden suite that runs the built program
// in a world; updateFlag is the name of the flag that sets update, quoted when a golden is missing.
func checkGoldenIn(t *testing.T, dir, name, got string, update *bool, updateFlag string) {
	t.Helper()
	if registryGoldenFileName.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", dir, name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with %s)", err, updateFlag)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}

// TestRegistryGoldenCasesAreDistinctFiles guards the case list itself: two cases of one name
// would share a golden file and each pass against the other's recording, and a file no case
// owns is a recording nothing checks.
func TestRegistryGoldenCasesAreDistinctFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range registryGoldenCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "registry-golden"))
	if err != nil {
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/registry-golden/%s belongs to no case", e.Name())
		}
	}
}

// The header of this file says which verbs the golden files are the contract of. A claim that is
// not checked drifts into one that outruns its cases (a verb named here whose cases were never
// written would look pinned and be free to change), so each command line the header names is
// looked for in the transcripts: a verb with no case fails here by name.
func TestEveryVerbTheGoldenHeaderNamesHasAGoldenCase(t *testing.T) {
	claimed := []string{
		"skills list", "skills status", "skills validate", "skills add", "skills remove",
		"skills sync-manifest", "skills install", "skills adopt", "skills project-register",
		"skills project-status", "skills project-retire", "skills project-revise",
		"pipkg build", "pipkg check", "runtime install --target pi", "runtime status --target pi",
	}
	checkTheListIsTheHeader(t, claimed)
	entries, err := os.ReadDir(filepath.Join("testdata", "registry-golden"))
	if err != nil {
		t.Fatal(err)
	}
	var transcripts strings.Builder
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("testdata", "registry-golden", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		transcripts.Write(data)
	}
	for _, verb := range claimed {
		if !strings.Contains(transcripts.String(), "\n$ "+verb+" ") && !strings.Contains(transcripts.String(), "\n$ "+verb+"\n") && !strings.HasPrefix(transcripts.String(), "$ "+verb) {
			t.Errorf("no golden file records a run of %q, which the header of registry_golden_test.go names", verb)
		}
	}
}

// checkTheListIsTheHeader holds the list of claimed command lines to the header of this file, so
// that neither can drift from the other: every verb the header names in quotes ('skills list',
// 'status', 'pipkg build') ends one of the command lines, and every command line, but the
// ones of the Pi runtime (which the header names as a group, "the Pi runtime verbs"), ends in a
// verb the header names.
func checkTheListIsTheHeader(t *testing.T, claimed []string) {
	const (
		headerStart = "// The golden files under"
		headerEnd   = "var updateRegistryGolden"
	)
	t.Helper()
	source, err := os.ReadFile("registry_golden_test.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	from, to := strings.Index(text, headerStart), strings.Index(text, headerEnd)
	if from < 0 || to < from {
		t.Fatalf("registry_golden_test.go has no header between %q and %q: the check that holds the list to the header cannot find it, so say where the header is here", headerStart, headerEnd)
	}
	header := text[from:to]
	named := regexp.MustCompile(`'([a-z -]+)'`).FindAllStringSubmatch(header, -1)
	if len(named) == 0 {
		t.Fatal("the header of registry_golden_test.go names no verb in quotes")
	}
	for _, m := range named {
		found := false
		for _, line := range claimed {
			found = found || strings.HasSuffix(line, m[1])
		}
		if !found {
			t.Errorf("the header names %q, which no command line of the claimed list ends with: add it to the list", m[1])
		}
	}
	for _, line := range claimed {
		if strings.HasPrefix(line, "runtime ") {
			continue
		}
		found := false
		for _, m := range named {
			found = found || strings.HasSuffix(line, m[1])
		}
		if !found {
			t.Errorf("the claimed list has %q, which the header does not name in quotes: name it there or drop it", line)
		}
	}
}

// --- the fixtures every case builds its world from ---------------------------------------

// worldRegistry is where a world keeps its registry, and worldManifest its manifest.
const (
	worldRegistry = "skills.registry.yaml"
	worldManifest = "overlay.manifest"
)

// goldenRegistryYAML is a registry of four entries that exercises every field the format has:
// a custom skill, a core one with an upstream, an external one with a repository and a ref,
// and a project-scoped one with its projects. The entries are not in order of their ids.
const goldenRegistryYAML = `version: "1"
skills:
  - id: beta
    path: beta
    source:
      type: core
      upstream:
        owner: gentleman-programming
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
        - codex
        - pi
    lifecycle:
      updateStrategy: vendor-merge
  - id: alpha
    path: alpha
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
    lifecycle:
      updateStrategy: overlay-only
  - id: ext-one
    path: ext-one
    source:
      type: external
      repo: https://example.test/org/ext-one
      ref: v1.2.0
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: tidy-notes
    path: tidy-notes
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - demo
        - other
      targets:
        - claude
        - pi
    lifecycle:
      updateStrategy: overlay-only
`

// goldenManifest lists the SKILL.md row of each entry of goldenRegistryYAML, with the rows of
// files that are not skills: the engine's own (never deployed) and a comment.
const goldenManifest = `# the overlay manifest
engine/go.mod managed
alpha/SKILL.md custom
beta/SKILL.md managed
ext-one/SKILL.md custom
tidy-notes/SKILL.md custom
`

// skillFile is the smallest SKILL.md that passes the lint that 'add' applies.
func skillFile(id string) string {
	return "---\n" +
		"name: " + id + "\n" +
		"description: A concise procedural skill for " + id + ".\n" +
		"license: MIT\n" +
		"metadata:\n" +
		"  author: tester\n" +
		"  version: \"1.0\"\n" +
		"---\n" +
		"## Activation Contract\n" +
		"Load this skill for its documented procedure.\n\n" +
		"## Hard Rules\n" +
		"- Keep the procedure explicit.\n\n" +
		"## Execution Steps\n" +
		"1. Follow the procedure.\n"
}

// putSkill writes skills/<id>/SKILL.md, and, for a skill that is global, the approval record that
// the exact bytes need to be added to or validated in a registry.
func (w *registryWorld) putSkill(id string, approved bool) {
	w.t.Helper()
	w.putSkillBytes(id, skillFile(id), approved)
}

func (w *registryWorld) putSkillBytes(id, content string, approved bool) {
	w.t.Helper()
	w.put("skills/"+id+"/SKILL.md", content)
	record := w.path("skills/" + id + "/" + skills.ApprovalRecordName)
	if !approved {
		if err := os.Remove(record); err != nil && !errors.Is(err, fs.ErrNotExist) {
			w.t.Fatal(err)
		}
		return
	}
	data, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{
		Skill:      id,
		SHA256:     skills.SkillDigest([]byte(content)),
		ApprovedAt: "2026-09-30T12:00:00Z",
		Approver:   "fixture-reviewer",
	})
	if err != nil {
		w.t.Fatalf("serialize the approval record of %q: %v", id, err)
	}
	w.put("skills/"+id+"/"+skills.ApprovalRecordName, string(data))
}

// overlay is a world that holds an overlay: the golden registry and manifest, and a skill for each
// of its entries (approved when it is global).
func (w *registryWorld) overlay() {
	w.t.Helper()
	w.put(worldRegistry, goldenRegistryYAML)
	w.put(worldManifest, goldenManifest)
	w.putSkill("alpha", true)
	w.putSkill("beta", true)
	w.putSkill("ext-one", true)
	w.putSkill("tidy-notes", false)
}

// registryOf is a registry of the given entries, written as the program writes one: the
// four-field shape of a custom skill that is global.
func registryOf(ids ...string) string {
	var b strings.Builder
	b.WriteString("version: \"1\"\nskills:\n")
	for _, id := range ids {
		b.WriteString("  - id: " + id + "\n    path: " + id + "\n    source:\n      type: custom\n")
		b.WriteString("    install:\n      defaultScope: global\n      targets:\n        - claude\n")
		b.WriteString("    lifecycle:\n      updateStrategy: overlay-only\n")
	}
	return b.String()
}
