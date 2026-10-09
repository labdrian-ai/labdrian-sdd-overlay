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
// words after them come from the system call or from the package that made it, and are not part of
// what the cases pin.
var failureWording = []string{"settings: create temp: ", "settings: backup to ", "settings: rename temp to "}

func sameFailure(want, got string) bool {
	if want == got {
		return true
	}
	for _, opening := range failureWording {
		if strings.HasPrefix(want, opening) && strings.HasPrefix(got, opening) {
			return true
		}
	}
	return false
}

// goldenDir holds the recorded files: each is a document of its own (cases and the texts they
// use), at most about 60 KB, and the cases of all of them are replayed.
const goldenDir = "testdata/golden-v1"

func loadGolden(t *testing.T) goldenDocument {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden file in %s: %v", goldenDir, err)
	}
	merged := goldenDocument{Blobs: map[string]string{}}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc goldenDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
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

// TestSettingsJSONIsWhatTheProgramWroteBeforeH27 replays the cases recorded from the program as it
// was before settings was split (testdata/golden-v1): the same starting file, the same calls, and
// after each one the bytes of settings.json, its mode, its backup and its mode, the names left in
// the directory, and whether the call failed with the same words. It runs the file adapter; the
// Merger it replaced (settings, before the adapter existed) passed the same file.
func TestSettingsJSONIsWhatTheProgramWroteBeforeH27(t *testing.T) {
	previous := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(previous) })
	golden := loadGolden(t)
	if len(golden.Cases) == 0 {
		t.Fatal("the golden holds no case")
	}
	for _, c := range golden.Cases {
		t.Run(c.Name, func(t *testing.T) {
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
				checkGoldenFile(t, golden, world, label+" settings.json.bak", path+".bak", step.After.Bak)
				if got := dirFiles(t, claude); strings.Join(got, "|") != strings.Join(step.After.DirFiles, "|") {
					t.Errorf("%s: the directory holds %q, the program left %q", label, got, step.After.DirFiles)
				}
			}
		})
	}
}
