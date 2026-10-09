package fsstore_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/fsstore"
)

// The adapter answers the port the use case owns.
var _ app.RegistryStore = fsstore.Registry{}

func TestReadReturnsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-registry.md")
	if err := os.WriteFile(path, []byte("# registry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fsstore.Registry{}.Read(path)
	if err != nil || string(got) != "# registry\n" {
		t.Fatalf("Read = %q, %v", got, err)
	}
}

// A registry that is not there is a NotFoundError that says what the system said, so the use case
// can tell it from a failure and the person still reads the system's reason.
func TestReadReportsAMissingRegistryInTheWordsOfTheSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-registry.md")
	_, err := fsstore.Registry{}.Read(path)
	var missing *app.NotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("Read = %T %v, want a NotFoundError", err, err)
	}
	_, system := os.ReadFile(path)
	if err.Error() != system.Error() {
		t.Errorf("the error says %q, the system said %q", err.Error(), system.Error())
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the error does not unwrap to fs.ErrNotExist: %v", err)
	}
}

// What is not a missing file is a failure to read, not a registry that is absent: a directory where
// the file should be, and a path through something that is not a directory.
func TestReadDoesNotMistakeAFailureForAnAbsentRegistry(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "plain")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"a directory":                 dir,
		"a path through a plain file": filepath.Join(file, "skill-registry.md"),
	} {
		_, err := fsstore.Registry{}.Read(path)
		if err == nil {
			t.Errorf("%s: Read succeeded", name)
			continue
		}
		var missing *app.NotFoundError
		if errors.As(err, &missing) {
			t.Errorf("%s: Read = %v, a failure reported as an absent registry", name, err)
		}
	}
}

// TC-RACE-2: Write puts the exact content in place and leaves no temporary file behind, whether it
// creates the file or replaces one.
func TestWriteContentAndNoLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill-registry.md")
	store := fsstore.Registry{}

	if err := store.Write(path, []byte("first version\n")); err != nil {
		t.Fatalf("Write (create): %v", err)
	}
	if err := store.Write(path, []byte("second version\n")); err != nil {
		t.Fatalf("Write (replace): %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "second version\n" {
		t.Errorf("content mismatch: got %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "skill-registry.md" {
			t.Errorf("leftover file in dir: %s", e.Name())
		}
	}
}

// The registry is readable by the people who read the repository whatever the mode of the
// temporary file it was written through or the umask of the process: 0644.
func TestWriteLeavesTheRegistryAtMode0644(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-registry.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (fsstore.Registry{}).Write(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want -rw-r--r--", info.Mode().Perm())
	}
}

// A write that cannot be made says why in the words of the system and leaves nothing behind.
func TestWriteIntoAMissingDirectoryFailsWithTheSystemsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-such-dir", "skill-registry.md")
	err := fsstore.Registry{}.Write(path, []byte("x"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write = %v, want the system's not-exist error", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}

// A write that fails after the temporary file exists (here the rename, because a directory stands
// where the registry should be) removes the temporary file and leaves what was there.
func TestAWriteThatFailsLateLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill-registry.md")
	if err := os.MkdirAll(filepath.Join(path, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (fsstore.Registry{}).Write(path, []byte("x")); err == nil {
		t.Fatal("Write over a directory succeeded")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "skill-registry.md" {
		t.Errorf("the directory holds %v, want only what was there", entries)
	}
}

// A reader running beside the writer never sees a torn registry: every read is whole old or whole
// new content (the rename is the only moment the file changes).
func TestAReaderBesideTheWriterNeverSeesAHalf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-registry.md")
	oldText := strings.Repeat("old line\n", 4000)
	newText := strings.Repeat("new line\n", 4000)
	if err := os.WriteFile(path, []byte(oldText), 0o644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	torn := make(chan string, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			got, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if !bytes.Equal(got, []byte(oldText)) && !bytes.Equal(got, []byte(newText)) {
				select {
				case torn <- string(got[:20]):
				default:
				}
				return
			}
		}
	}()
	store := fsstore.Registry{}
	for i := 0; i < 200; i++ {
		text := newText
		if i%2 == 1 {
			text = oldText
		}
		if err := store.Write(path, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	wg.Wait()
	select {
	case start := <-torn:
		t.Errorf("a read saw neither the old nor the new registry; it began %q", start)
	default:
	}
}
