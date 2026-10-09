//go:build unix

package settingsfile_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// goldenFile is what the files of testdata/golden-v1 record for one file: the bytes, by digest into the
// blobs, and the permission bits.
type goldenFile struct {
	Blob string `json:"blob"`
	Mode string `json:"mode"`
}

// goldenState is the part of a world a case looks at: settings.json, its backup, and the names in
// the directory that holds them.
type goldenState struct {
	Settings  *goldenFile `json:"settings"`
	Bak       *goldenFile `json:"bak"`
	DirFiles  []string    `json:"dir_files"`
	ClaudeDir bool        `json:"claude_dir"`
}

type goldenStep struct {
	Op    string      `json:"op"`
	RC    int         `json:"rc"`
	Error string      `json:"error"`
	After goldenState `json:"after"`
}

type goldenCase struct {
	Name   string       `json:"name"`
	Before goldenState  `json:"before"`
	Steps  []goldenStep `json:"steps"`
}

type goldenDocument struct {
	RecordedFrom string            `json:"recorded_from"`
	HookCommand  string            `json:"hook_command"`
	Blobs        map[string]string `json:"blobs"`
	Cases        []goldenCase      `json:"cases"`
}

var (
	// writeTemp names the temporary files a write leaves while it runs; none may be left after.
	writeTemp = regexp.MustCompile(`^\.(settings-\d+\.json|atomicfile-\d+)\.tmp$`)
	// tempInMessage is the same name inside an error message, where the digits are random.
	tempInMessage = regexp.MustCompile(`\.settings-\d+\.json\.tmp`)
)

// failureWording lists the openings of the errors that report a failure of the file system. The
// words between the opening and the rest are not pinned: they name the package that made the call
// (the old program said `settings: create temp: open <path>: <reason>`, the adapter says
// `settings: create temp: atomicfile: create temporary file: open <path>: <reason>`). The rest, the
// call that failed, the file and the reason the system gave, is: it is what the old message said
// after its opening, and the new one must end with it.
var failureWording = []string{"settings: create temp: "}

// sameFailure reports whether a failure the adapter reports is the one the program recorded: the
// same words, or the same opening and, after any words of the adapter's own, the same ending. A
// reason that holds ": " itself, or a message with no separator at all, is compared whole, because
// the ending is everything the old message said after its opening and not the text after some
// separator.
func sameFailure(want, got string) bool {
	if want == got {
		return true
	}
	for _, opening := range failureWording {
		if strings.HasPrefix(want, opening) && strings.HasPrefix(got, opening) {
			return strings.HasSuffix(got, ": "+strings.TrimPrefix(want, opening))
		}
	}
	return false
}

// backupAfter is the backup a step must leave. A step that wrote settings.json (its content changed)
// kept the file it replaced, so the backup holds the content of the file before the step at that
// file's mode. One that wrote nothing leaves the backup it found, with the mode it had (kept); the
// recording shows 0o644 for every backup, which is what is no longer true.
func backupAfter(before goldenState, kept *goldenFile, after goldenState) *goldenFile {
	if after.Bak == nil {
		return nil
	}
	if before.Settings != nil && after.Settings != nil &&
		after.Settings.Blob != before.Settings.Blob && after.Bak.Blob == before.Settings.Blob {
		return &goldenFile{Blob: after.Bak.Blob, Mode: before.Settings.Mode}
	}
	if kept != nil && kept.Blob == after.Bak.Blob {
		return kept
	}
	return after.Bak
}

// keepsAReadOnlyBackup reports whether a case starts with a backup its owner cannot write, the
// recording of which is a refusal that no longer happens.
func keepsAReadOnlyBackup(c goldenCase) bool {
	return c.Before.Bak != nil && modeBits(c.Before.Bak.Mode)&0o200 == 0
}

// goldenRoot stands for `$W` in the recorded texts: the root the recording's hook command lived
// under (<root>/home/.claude/bin/gentle-ai-overlay). It is a fixed name, not the temporary
// directory of the replay, because the hook command is what the program tells its own entries by:
// the identity tokens (`review-receipt`, `sync-trigger`, ...) are substrings of the entries' commands,
// and a directory named after the case or the test, as t.TempDir() makes it on a Go that does not
// shorten the name, carries a token into every command and makes the program see entries that are
// not there. Only the messages of errors, which name the real files, use the real directory.
const goldenRoot = "/opt/labdrian-golden"

// goldenDir holds the recorded files: each is a document of its own (cases and the texts they
// use), at most about 60 KB, and the cases of all of them are replayed.
const goldenDir = "testdata/golden-v1"

// goldenManifest records, for each file of goldenDir, the number of cases and steps it holds (156
// cases and 468 steps in all when it was written, the day the recording was split into files). A
// file lost from the directory, emptied or cut changes what the directory holds and no longer
// matches its entry, so a replay that passes cannot be a replay of less than was recorded. A file
// added on purpose is added to the manifest by running
//
//	go test ./settings/settingsfile -run TestTheGoldenFilesHoldWhatWasRecorded -update-golden-manifest
//
// and the diff of the manifest is read before it is committed, as the diff of the data is.
const goldenManifest = "testdata/golden-v1.manifest.json"

var updateGoldenManifest = flag.Bool("update-golden-manifest", false, "rewrite the manifest of the golden files")

// goldenCount is what one golden file holds.
type goldenCount struct {
	Cases int `json:"cases"`
	Steps int `json:"steps"`
}

func goldenFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden file in %s: %v", goldenDir, err)
	}
	return files
}

func readGoldenDocument(t *testing.T, file string) goldenDocument {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var doc goldenDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return doc
}

func loadGolden(t *testing.T) goldenDocument {
	t.Helper()
	merged := goldenDocument{Blobs: map[string]string{}}
	for _, file := range goldenFiles(t) {
		doc := readGoldenDocument(t, file)
		merged.RecordedFrom, merged.HookCommand = doc.RecordedFrom, doc.HookCommand
		for key, text := range doc.Blobs {
			if previous, ok := merged.Blobs[key]; ok && previous != text {
				t.Fatalf("%s: blob %s differs from the one an earlier file holds", file, key)
			}
			merged.Blobs[key] = text
		}
		merged.Cases = append(merged.Cases, doc.Cases...)
	}
	return merged
}

// fixedUmask sets the umask the recorded modes were made under (022) for the length of the test and
// restores it. The umask belongs to the process, so a test that sets it must not run beside another:
// TestNoTestOfThisPackageRunsInParallel keeps that true.
func fixedUmask(t *testing.T) {
	t.Helper()
	previous := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(previous) })
}

// rootDefeatsBackupProtection reports whether a case relies on a backup that its owner cannot write
// (a .bak of mode 0444): for root the open succeeds, so the recorded refusal does not happen and the
// case says nothing about the adapter.
func rootDefeatsBackupProtection(euid int, c goldenCase) bool {
	if euid != 0 || c.Before.Bak == nil {
		return false
	}
	return modeBits(c.Before.Bak.Mode)&0o200 == 0
}

func modeBits(text string) uint64 {
	bits, err := strconv.ParseUint(text, 0, 32)
	if err != nil {
		return 0
	}
	return bits
}

func modeOf(t *testing.T, text string) os.FileMode {
	t.Helper()
	bits, err := strconv.ParseUint(text, 0, 32)
	if err != nil {
		t.Fatalf("mode %q: %v", text, err)
	}
	return os.FileMode(bits)
}

func writeGoldenFile(t *testing.T, doc goldenDocument, root, path string, file *goldenFile) {
	t.Helper()
	if file == nil {
		return
	}
	text, ok := doc.Blobs[file.Blob]
	if !ok {
		t.Fatalf("blob %q is not in the golden", file.Blob)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(text, "$W", root)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, modeOf(t, file.Mode)); err != nil {
		t.Fatal(err)
	}
}

func readGoldenFile(t *testing.T, path string) (text string, mode os.FileMode, present bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), info.Mode().Perm(), true
}

// checkNothingNamesTheWorld fails when one of the files holds the directory of the replay. The
// world is where the test runs and its name may carry the name of the test, so a file that names it
// would hold, in a command, text the program tells its entries by that only this run has. The
// expected text is built on goldenRoot and the comparison is exact, so this is already true when
// the files match; the check says it in so many words, and a file that leaks fails with that
// reason and not as a difference of bytes.
func checkNothingNamesTheWorld(t *testing.T, label, world string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if text, _, present := readGoldenFile(t, path); present && strings.Contains(text, world) {
			t.Errorf("%s: %s holds the directory of the replay, %s, so its commands depend on where the test runs", label, path, world)
		}
	}
}

func checkGoldenFile(t *testing.T, doc goldenDocument, root, label, path string, want *goldenFile) {
	t.Helper()
	text, mode, present := readGoldenFile(t, path)
	if want == nil {
		if present {
			t.Errorf("%s exists, the program left none", label)
		}
		return
	}
	if !present {
		t.Errorf("%s is missing", label)
		return
	}
	if wantText := strings.ReplaceAll(doc.Blobs[want.Blob], "$W", root); text != wantText {
		t.Errorf("%s differs from what the program wrote:\n got: %q\nwant: %q", label, text, wantText)
	}
	if wantMode := modeOf(t, want.Mode); mode != wantMode {
		t.Errorf("%s has mode %v, the program left %v", label, mode, wantMode)
	}
}

func dirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if writeTemp.MatchString(name) {
			name = ".TMP"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// OWNER DECISION 2 OF 2026-10-09 changes two things the recording holds, and this file says so in
// code and leaves the recorded data as it was: the backup is written by atomicfile at the mode of
// the file it keeps (the recording has 0o644 whatever the file's mode was), and a read-only backup
// is replaced (the recording has a refusal). backupAfter gives the backup a step must leave, and
// keepsAReadOnlyBackup names the cases whose recorded refusal no longer applies.
//
// The recorded modes (0o664 for a starting file the recording made, 0o600 for one the program wrote)
// are set on the starting files explicitly, under the umask fixedUmask sets, so the replay does not
// depend on the umask of whoever runs it. The cases pin what the program did, quirks included (a
// backup overwritten on each change, 0600 for the file and 0644 for its backup): a change that
// fixes one is a decision, and changes the file with its reason.
//
// TestSettingsJSONIsWhatTheProgramWroteBeforeH27 replays the cases recorded from the program as it
// was before settings was split (testdata/golden-v1): the same starting file, the same calls, and
// after each one the bytes of settings.json, its mode, its backup and its mode, the names left in
// the directory, and whether the call failed with the same words. It runs the file adapter; the
// Merger it replaced (settings, before the adapter existed) passed the same file.
func TestSettingsJSONIsWhatTheProgramWroteBeforeH27(t *testing.T) {
	fixedUmask(t)
	golden := loadGolden(t)
	if len(golden.Cases) == 0 {
		t.Fatal("the golden holds no case")
	}
	for _, c := range golden.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if rootDefeatsBackupProtection(os.Geteuid(), c) {
				t.Skip("root writes through a read-only backup, so the refusal the case records cannot happen")
			}
			if keepsAReadOnlyBackup(c) {
				t.Skip("recorded: a read-only backup stopped the write; since owner decision 2 it is replaced (TestAReadOnlyBackupIsReplacedAtTheModeOfTheOriginal)")
			}
			world := t.TempDir()
			claude := filepath.Join(world, "home", ".claude")
			path := filepath.Join(claude, "settings.json")
			if c.Before.ClaudeDir {
				if err := os.MkdirAll(claude, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeGoldenFile(t, golden, goldenRoot, path, c.Before.Settings)
			writeGoldenFile(t, golden, goldenRoot, path+".bak", c.Before.Bak)
			installer := settingsfile.Installer{}
			hookCommand := strings.ReplaceAll(golden.HookCommand, "$W", goldenRoot)

			previous, kept := c.Before, c.Before.Bak
			for n, step := range c.Steps {
				var err error
				switch step.Op {
				case "merge":
					err = installer.Install(path, hookCommand)
				case "uninstall":
					err = installer.Uninstall(path, hookCommand)
				default:
					t.Fatalf("step %d: unknown op %q", n, step.Op)
				}
				if (err == nil) != (step.RC == 0) {
					t.Fatalf("step %d (%s): error = %v, the program exited %d", n, step.Op, err, step.RC)
				}
				if err != nil {
					want := strings.ReplaceAll(step.Error, "$W", world)
					want = strings.TrimPrefix(strings.TrimPrefix(want, "error: merge-settings: "), "error: uninstall-hooks: ")
					got := tempInMessage.ReplaceAllString(err.Error(), ".settings-N.json.tmp")
					if !sameFailure(want, got) {
						t.Errorf("step %d (%s): error\n got: %s\nwant: %s", n, step.Op, got, want)
					}
				}
				label := "step " + strconv.Itoa(n) + " (" + step.Op + ")"
				checkGoldenFile(t, golden, goldenRoot, label+" settings.json", path, step.After.Settings)
				kept = backupAfter(previous, kept, step.After)
				checkGoldenFile(t, golden, goldenRoot, label+" settings.json.bak", path+".bak", kept)
				checkNothingNamesTheWorld(t, label, world, path, path+".bak")
				previous = step.After
				if got := dirFiles(t, claude); strings.Join(got, "|") != strings.Join(step.After.DirFiles, "|") {
					t.Errorf("%s: the directory holds %q, the program left %q", label, got, step.After.DirFiles)
				}
			}
		})
	}
}

// The files are the ones that were recorded, all of them read: the cases and steps add up, and none
// is empty, so a lost or emptied file fails here and not by shrinking the replay.
func TestTheGoldenFilesHoldWhatWasRecorded(t *testing.T) {
	held := map[string]goldenCount{}
	for _, file := range goldenFiles(t) {
		doc := readGoldenDocument(t, file)
		count := goldenCount{Cases: len(doc.Cases)}
		for _, c := range doc.Cases {
			count.Steps += len(c.Steps)
		}
		held[filepath.Base(file)] = count
	}
	if *updateGoldenManifest {
		raw, err := json.MarshalIndent(held, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenManifest, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(goldenManifest)
	if err != nil {
		t.Fatal(err)
	}
	var recorded map[string]goldenCount
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatalf("%s: %v", goldenManifest, err)
	}
	for _, problem := range manifestProblems(recorded, held) {
		t.Error(problem)
	}
}

// manifestProblems says where the files a directory holds differ from the manifest: a file the
// manifest names and the directory lacks, a file the directory has and the manifest does not
// name, a file with no case, and a file whose cases or steps are not the recorded number. The
// answer is in the order of the names, so a failure reads the same on every run.
func manifestProblems(recorded, held map[string]goldenCount) []string {
	names := map[string]bool{}
	for name := range recorded {
		names[name] = true
	}
	for name := range held {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var problems []string
	for _, name := range sorted {
		want, isRecorded := recorded[name]
		got, isHeld := held[name]
		switch {
		case !isHeld:
			problems = append(problems, name+" is in the manifest and not in the directory")
		case !isRecorded:
			problems = append(problems, name+" is in the directory and not in the manifest")
		case got.Cases == 0:
			problems = append(problems, name+" holds no case")
		case got != want:
			problems = append(problems, fmt.Sprintf("%s holds %d cases and %d steps, the manifest has %d and %d",
				name, got.Cases, got.Steps, want.Cases, want.Steps))
		}
	}
	return problems
}

func TestManifestProblemsNamesEachWayAFileDiffers(t *testing.T) {
	recorded := map[string]goldenCount{"a.json": {2, 6}, "b.json": {1, 3}, "c.json": {4, 12}, "d.json": {1, 1}}
	held := map[string]goldenCount{"a.json": {2, 6}, "c.json": {4, 11}, "d.json": {0, 0}, "e.json": {1, 1}}
	want := []string{
		"b.json is in the manifest and not in the directory",
		"c.json holds 4 cases and 11 steps, the manifest has 4 and 12",
		"d.json holds no case",
		"e.json is in the directory and not in the manifest",
	}
	got := manifestProblems(recorded, held)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems\n got: %q\nwant: %q", got, want)
	}
	if problems := manifestProblems(recorded, recorded); len(problems) != 0 {
		t.Errorf("a directory that is the manifest has problems: %q", problems)
	}
	if problems := manifestProblems(map[string]goldenCount{"a.json": {2, 6}}, map[string]goldenCount{"a.json": {3, 6}}); len(problems) != 1 {
		t.Errorf("a file with another number of cases and the same steps has problems %q, want one", problems)
	}
}

func TestSameFailurePinsTheEndingAndNotTheWordsBetween(t *testing.T) {
	old := "settings: create temp: open /d/.settings-N.json.tmp: permission denied"
	cases := []struct {
		name, want, got string
		same            bool
	}{
		{"identical", old, old, true},
		{"the adapter's words between", old, "settings: create temp: atomicfile: create temporary file: open /d/.settings-N.json.tmp: permission denied", true},
		{"another reason", old, "settings: create temp: atomicfile: create temporary file: open /d/.settings-N.json.tmp: no such file or directory", false},
		{"the same reason of another file", old, "settings: create temp: atomicfile: create temporary file: open /e/.settings-N.json.tmp: permission denied", false},
		{"the same reason of another call", old, "settings: create temp: atomicfile: create temporary file: mkdir /d/.settings-N.json.tmp: permission denied", false},
		{"only the reason", old, "settings: create temp: permission denied", false},
		{"another opening", old, "settings: replace /d/x: permission denied", false},
		{"no opening of the three", old, "something else: permission denied", false},
		{"a reason that holds the separator", "settings: create temp: open /d/t: bad: thing", "settings: create temp: atomicfile: open /d/t: bad: thing", true},
		{"a reason that holds the separator, cut", "settings: create temp: open /d/t: bad: thing", "settings: create temp: atomicfile: open /d/t: other: thing", false},
		{"a message with no separator after its opening", "settings: create temp: boom", "settings: create temp: atomicfile: boom", true},
		{"a message with no separator, another word", "settings: create temp: boom", "settings: create temp: atomicfile: bang", false},
		{"an ending that is part of a longer word", "settings: create temp: boom", "settings: create temp: atomicfile: kaboom", false},
	}
	for _, c := range cases {
		if got := sameFailure(c.want, c.got); got != c.same {
			t.Errorf("%s: sameFailure = %v, want %v", c.name, got, c.same)
		}
	}
}

func TestRootDefeatsOnlyABackupItsOwnerCannotWrite(t *testing.T) {
	readOnly := goldenCase{Before: goldenState{Bak: &goldenFile{Mode: "0o444"}}}
	writable := goldenCase{Before: goldenState{Bak: &goldenFile{Mode: "0o644"}}}
	none := goldenCase{}
	for _, c := range []struct {
		name string
		euid int
		c    goldenCase
		want bool
	}{
		{"root, read-only backup", 0, readOnly, true},
		{"not root, read-only backup", 1000, readOnly, false},
		{"root, writable backup", 0, writable, false},
		{"root, no backup", 0, none, false},
	} {
		if got := rootDefeatsBackupProtection(c.euid, c.c); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// fixedUmask changes a setting of the whole process, so no test of this package may run beside
// another. This scan keeps it true: it fails on a test that asks for it.
func TestNoTestOfThisPackageRunsInParallel(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test file found: %v", err)
	}
	call := "t.Para" + "llel("
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), call) {
			t.Errorf("%s runs a test in parallel; fixedUmask would race with it", file)
		}
	}
}

func TestBackupAfterFollowsTheModeOfTheFileItKept(t *testing.T) {
	file := func(blob, mode string) *goldenFile { return &goldenFile{Blob: blob, Mode: mode} }
	state := func(settings, bak *goldenFile) goldenState { return goldenState{Settings: settings, Bak: bak} }
	recorded := file("old", "0o644")

	cases := []struct {
		name         string
		before, post goldenState
		kept         *goldenFile
		want         *goldenFile
	}{
		{"a write keeps the file it replaced at its mode",
			state(file("old", "0o664"), nil), state(file("new", "0o600"), recorded), nil, file("old", "0o664")},
		{"a step that wrote nothing leaves the backup it found",
			state(file("new", "0o600"), file("old", "0o664")), state(file("new", "0o600"), recorded), file("old", "0o664"), file("old", "0o664")},
		{"no backup stays none", state(nil, nil), state(file("new", "0o600"), nil), nil, nil},
		{"a backup the recording shows and nothing explains is taken as recorded",
			state(nil, nil), state(file("new", "0o600"), recorded), nil, recorded},
	}
	for _, c := range cases {
		got := backupAfter(c.before, c.kept, c.post)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: backupAfter = %v, want %v", c.name, got, c.want)
		}
	}
}

// The root stands in for the recording's directory, and the program tells its entries by substrings
// of their commands: a root that held an identity token would be read as part of every entry.
func TestTheGoldenRootCarriesNoIdentityToken(t *testing.T) {
	for _, token := range []string{
		settings.LabdrianMinimalismIdentity, settings.LabdrianDesignIdentity, settings.LabdrianSyncTriggerIdentity,
		settings.LabdrianReviewReceiptIdentity, settings.LabdrianShaperGuardIdentity,
		settings.LabdrianProjectionIdentity, settings.LabdrianApproveGuardIdentity,
		"skill-discovery-safety", "review-projection-contract",
	} {
		if strings.Contains(goldenRoot, token) {
			t.Errorf("goldenRoot %q contains the identity token %q", goldenRoot, token)
		}
	}
}
