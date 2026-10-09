//go:build unix

package settingsfile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"

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
// words between the opening and the reason are not pinned: they name the package that made the call
// (the old program said `settings: create temp: open <path>: <reason>`, the adapter says
// `settings: create temp: atomicfile: create temporary file: open <path>: <reason>`). The reason, the
// words the system gave after the last colon, is.
var failureWording = []string{"settings: create temp: "}

// sameFailure reports whether a failure the adapter reports is the one the program recorded: the
// same words, or the same opening and the same reason.
func sameFailure(want, got string) bool {
	if want == got {
		return true
	}
	reason := func(message string) string { return message[strings.LastIndex(message, ": ")+2:] }
	for _, opening := range failureWording {
		if strings.HasPrefix(want, opening) && strings.HasPrefix(got, opening) {
			return reason(want) == reason(got)
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

// goldenDir holds the recorded files: each is a document of its own (cases and the texts they
// use), at most about 60 KB, and the cases of all of them are replayed.
const goldenDir = "testdata/golden-v1"

// The number of cases and steps the files hold. A file lost from the directory, or emptied, changes
// them, so a replay that passes cannot be a replay of less than was recorded.
const (
	goldenCaseCount = 156
	goldenStepCount = 468
)

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

func writeGoldenFile(t *testing.T, doc goldenDocument, world, path string, file *goldenFile) {
	t.Helper()
	if file == nil {
		return
	}
	text, ok := doc.Blobs[file.Blob]
	if !ok {
		t.Fatalf("blob %q is not in the golden", file.Blob)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(text, "$W", world)), 0o600); err != nil {
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

func checkGoldenFile(t *testing.T, doc goldenDocument, world, label, path string, want *goldenFile) {
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
	if wantText := strings.ReplaceAll(doc.Blobs[want.Blob], "$W", world); text != wantText {
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
			writeGoldenFile(t, golden, world, path, c.Before.Settings)
			writeGoldenFile(t, golden, world, path+".bak", c.Before.Bak)
			installer := settingsfile.Installer{}
			hookCommand := strings.ReplaceAll(golden.HookCommand, "$W", world)

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
				checkGoldenFile(t, golden, world, label+" settings.json", path, step.After.Settings)
				kept = backupAfter(previous, kept, step.After)
				checkGoldenFile(t, golden, world, label+" settings.json.bak", path+".bak", kept)
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
	cases, steps := 0, 0
	for _, file := range goldenFiles(t) {
		doc := readGoldenDocument(t, file)
		if len(doc.Cases) == 0 {
			t.Errorf("%s holds no case", file)
		}
		for _, c := range doc.Cases {
			cases++
			steps += len(c.Steps)
		}
	}
	if cases != goldenCaseCount || steps != goldenStepCount {
		t.Errorf("the golden files hold %d cases and %d steps, want %d and %d", cases, steps, goldenCaseCount, goldenStepCount)
	}
}

func TestSameFailurePinsTheReasonAndNotTheWordsBetween(t *testing.T) {
	old := "settings: create temp: open /d/.settings-N.json.tmp: permission denied"
	cases := []struct {
		name, got string
		want      bool
	}{
		{"identical", old, true},
		{"the adapter's words between", "settings: create temp: atomicfile: create temporary file: open /d/.settings-N.json.tmp: permission denied", true},
		{"another reason", "settings: create temp: atomicfile: create temporary file: open /d/.settings-N.json.tmp: no such file or directory", false},
		{"another opening", "settings: replace /d/x: permission denied", false},
		{"no opening of the three", "something else: permission denied", false},
	}
	for _, c := range cases {
		if got := sameFailure(old, c.got); got != c.want {
			t.Errorf("%s: sameFailure = %v, want %v", c.name, got, c.want)
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
