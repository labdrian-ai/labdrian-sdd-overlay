package atomicfile

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeFixture(t *testing.T, path, content string, perm fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// names lists a directory, so a test can assert that nothing but the expected
// files is in it: a temporary file left behind is a failure.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func assertOnly(t *testing.T, dir string, want ...string) {
	t.Helper()
	got := names(t, dir)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("directory holds %v, want exactly %v", got, want)
	}
}

// spy records the order of the operations a write performs and lets a test fail
// any of them, or look at the directory at the moment one runs.
type spy struct {
	events      []string
	failFile    error
	failDir     error
	atFileSync  func()
	atDirSync   func()
	syncedFiles int
}

func (s *spy) ops() ops {
	return ops{
		syncFile: func(f *os.File) error {
			s.events = append(s.events, "sync-file")
			s.syncedFiles++
			if s.atFileSync != nil {
				s.atFileSync()
			}
			return s.failFile
		},
		syncDir: func(dir string) error {
			s.events = append(s.events, "sync-dir:"+filepath.Base(dir))
			if s.atDirSync != nil {
				s.atDirSync()
			}
			return s.failDir
		},
	}
}

func TestWriteFileCreatesTheFileWithTheRequestedContentAndMode(t *testing.T) {
	for _, perm := range []fs.FileMode{0o600, 0o640, 0o644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.json")
		if err := WriteFile(path, []byte("hello"), Options{Perm: perm}); err != nil {
			t.Fatalf("WriteFile with %v: %v", perm, err)
		}
		if got := readFixture(t, path); got != "hello" {
			t.Errorf("perm %v: content = %q, want %q", perm, got, "hello")
		}
		if got := modeOf(t, path); got != perm {
			t.Errorf("perm %v: mode = %v, want exactly %v whatever the umask", perm, got, perm)
		}
		assertOnly(t, dir, "out.json")
	}
}

func TestTheZeroPermIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	if err := WriteFile(path, []byte("x"), Options{}); err != nil {
		t.Fatal(err)
	}
	if got := modeOf(t, path); got != 0o600 {
		t.Errorf("mode = %v, want 0600 when no Perm is asked for", got)
	}
}

func TestWriteFileReplacesTheContentAndTheModeOfAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	writeFixture(t, path, "old", 0o600)

	if err := WriteFile(path, []byte("new"), Options{Perm: 0o644}); err != nil {
		t.Fatal(err)
	}
	if got := readFixture(t, path); got != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
	if got := modeOf(t, path); got != 0o644 {
		t.Errorf("mode = %v, want the requested 0644, not the old file's", got)
	}
	assertOnly(t, dir, "out")
}

func TestPermMustBePermissionBitsOnly(t *testing.T) {
	dir := t.TempDir()
	for _, perm := range []fs.FileMode{fs.ModeSetuid | 0o755, fs.ModeDir | 0o755, fs.ModeSymlink} {
		err := WriteFile(filepath.Join(dir, "out"), []byte("x"), Options{Perm: perm})
		if err == nil || !strings.Contains(err.Error(), "permission bits") {
			t.Errorf("Perm %v: error = %v, want a refusal naming the permission bits", perm, err)
		}
	}
	assertOnly(t, dir)
}

// A reader that opens the file while a writer replaces it sees all of the old
// content or all of the new, never a prefix of either and never nothing.
func TestAReaderNeverSeesAPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	payloads := make([][]byte, 8)
	valid := map[string]bool{}
	for i := range payloads {
		payloads[i] = bytes.Repeat([]byte{byte('a' + i)}, 32*1024*(i+1))
		valid[string(payloads[i])] = true
	}
	if err := WriteFile(path, payloads[0], Options{Perm: 0o600}); err != nil {
		t.Fatal(err)
	}

	var writers sync.WaitGroup
	stop := make(chan struct{})
	for i := range payloads {
		i := i
		writers.Add(1)
		go func() {
			defer writers.Done()
			for n := 0; n < 20; n++ {
				if err := WriteFile(path, payloads[i], Options{Perm: 0o600}); err != nil {
					t.Errorf("writer %d: %v", i, err)
					return
				}
			}
		}()
	}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("read: %v", err)
				return
			}
			if !valid[string(data)] {
				t.Errorf("read %d bytes that are neither payload: a partial write was visible", len(data))
				return
			}
		}
	}()
	writers.Wait()
	close(stop)
	<-readerDone

	if !valid[readFixture(t, path)] {
		t.Error("the final content is not one of the payloads")
	}
	assertOnly(t, dir, "out")
}

func TestWriteFileFailsInAMissingDirectoryAndCreatesNothing(t *testing.T) {
	root := t.TempDir()
	err := WriteFile(filepath.Join(root, "missing", "out"), []byte("x"), Options{})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
	assertOnly(t, root)
}

// When the rename fails the old state is untouched and the temporary file is
// gone: a directory in the way is the simplest honest rename failure.
func TestWriteFileCleansUpWhenTheRenameFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out")
	if err := os.MkdirAll(filepath.Join(target, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(target, []byte("x"), Options{Perm: 0o600}); err == nil {
		t.Fatal("WriteFile over a directory = nil, want an error")
	}
	assertOnly(t, dir, "out")
	assertOnly(t, target, "keep")
}

func TestWriteFileRefusesASymlinkTargetAndLeavesBothEndsAlone(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	writeFixture(t, real, "precious", 0o600)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(link, []byte("clobber"), Options{Perm: 0o600})
	if !errors.Is(err, ErrSymlink) {
		t.Fatalf("error = %v, want ErrSymlink", err)
	}
	if got := readFixture(t, real); got != "precious" {
		t.Errorf("the link target was written: %q", got)
	}
	if info, lerr := os.Lstat(link); lerr != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the symlink was replaced: %v, %v", info, lerr)
	}
	assertOnly(t, dir, "link", "real")
}

func TestOptionsTempPatternNamesTheTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	staged, err := Stage(dir, []byte("x"), Options{TempPattern: ".tmp-skills-*"})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	got := names(t, dir)
	if len(got) != 1 || !strings.HasPrefix(got[0], ".tmp-skills-") {
		t.Errorf("temporary files = %v, want one named .tmp-skills-*", got)
	}
}

func TestTheDefaultTemporaryNameIsHidden(t *testing.T) {
	dir := t.TempDir()
	staged, err := Stage(dir, []byte("x"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	got := names(t, dir)
	if len(got) != 1 || !strings.HasPrefix(got[0], ".") || !strings.HasSuffix(got[0], ".tmp") {
		t.Errorf("temporary files = %v, want one hidden .tmp file", got)
	}
}

func TestSyncMakesTheFileDurableBeforeTheRenameAndTheRenameDurableAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	writeFixture(t, path, "old", 0o600)

	s := &spy{}
	s.atFileSync = func() {
		if got := readFixture(t, path); got != "old" {
			t.Errorf("the file was synced after the target was replaced (target = %q)", got)
		}
	}
	s.atDirSync = func() {
		if got := readFixture(t, path); got != "new" {
			t.Errorf("the directory was synced before the target was replaced (target = %q)", got)
		}
	}
	if err := writeFile(s.ops(), path, []byte("new"), Options{Perm: 0o600, Sync: true}); err != nil {
		t.Fatal(err)
	}

	want := []string{"sync-file", "sync-dir:" + filepath.Base(dir)}
	if strings.Join(s.events, ",") != strings.Join(want, ",") {
		t.Errorf("operations = %v, want %v", s.events, want)
	}
}

func TestWithoutSyncNothingIsFlushed(t *testing.T) {
	s := &spy{}
	path := filepath.Join(t.TempDir(), "out")
	if err := writeFile(s.ops(), path, []byte("x"), Options{Perm: 0o600}); err != nil {
		t.Fatal(err)
	}
	if len(s.events) != 0 {
		t.Errorf("operations = %v, want none when Sync is false", s.events)
	}
}

func TestAFailedFileSyncAbortsBeforeAnythingIsReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	writeFixture(t, path, "old", 0o600)

	s := &spy{failFile: errors.New("disk on fire")}
	err := writeFile(s.ops(), path, []byte("new"), Options{Perm: 0o600, Sync: true})
	if err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("error = %v, want the sync failure", err)
	}
	if got := readFixture(t, path); got != "old" {
		t.Errorf("target = %q, want the old content untouched", got)
	}
	assertOnly(t, dir, "out")
}

// The rename is done by the time the directory sync fails. The error says so, so
// a caller never believes the old content is still the durable one.
func TestAFailedDirectorySyncIsReportedAfterTheReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	writeFixture(t, path, "old", 0o600)

	s := &spy{failDir: errors.New("no space")}
	err := writeFile(s.ops(), path, []byte("new"), Options{Perm: 0o600, Sync: true})
	if err == nil || !strings.Contains(err.Error(), "no space") {
		t.Fatalf("error = %v, want the directory sync failure", err)
	}
	if got := readFixture(t, path); got != "new" {
		t.Errorf("target = %q, want the new content: the rename had happened", got)
	}
	assertOnly(t, dir, "out")
}

func TestBackupKeepsTheReplacedContentAndModeBesideTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeFixture(t, path, "old", 0o640)

	if err := WriteFile(path, []byte("new"), Options{Perm: 0o600, Backup: true}); err != nil {
		t.Fatal(err)
	}
	if got := readFixture(t, path); got != "new" {
		t.Errorf("target = %q, want new", got)
	}
	bak := path + ".bak"
	if got := readFixture(t, bak); got != "old" {
		t.Errorf("backup = %q, want the replaced content", got)
	}
	if got := modeOf(t, bak); got != 0o640 {
		t.Errorf("backup mode = %v, want the replaced file's 0640, not wider", got)
	}
	assertOnly(t, dir, "settings.json", "settings.json.bak")
}

func TestBackupOfASecondWriteHoldsTheFirstWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	for _, content := range []string{"one", "two", "three"} {
		if err := WriteFile(path, []byte(content), Options{Perm: 0o600, Backup: true}); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFixture(t, path+".bak"); got != "two" {
		t.Errorf("backup = %q, want the content the last write replaced", got)
	}
}

func TestNoBackupIsMadeForAFileThatDidNotExistOrWhenNotAsked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := WriteFile(path, []byte("first"), Options{Perm: 0o600, Backup: true}); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, dir, "f")

	if err := WriteFile(path, []byte("second"), Options{Perm: 0o600}); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, dir, "f")
}

// A backup that cannot be made stops the write: the old content is the only copy.
func TestAFailedBackupLeavesTheTargetUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	writeFixture(t, path, "old", 0o600)
	if err := os.MkdirAll(filepath.Join(path+".bak", "keep"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, []byte("new"), Options{Perm: 0o600, Backup: true}); err == nil {
		t.Fatal("WriteFile with an unusable backup name = nil, want an error")
	}
	if got := readFixture(t, path); got != "old" {
		t.Errorf("target = %q, want the old content untouched", got)
	}
	assertOnly(t, dir, "f", "f.bak")
}

func TestBackupIsFlushedWhenSyncIsAsked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	writeFixture(t, path, "old", 0o600)

	s := &spy{}
	if err := writeFile(s.ops(), path, []byte("new"), Options{Perm: 0o600, Backup: true, Sync: true}); err != nil {
		t.Fatal(err)
	}
	if s.syncedFiles != 2 {
		t.Errorf("files synced = %d, want 2 (the backup and the new content)", s.syncedFiles)
	}
}

// Stage and Replace are the two halves of WriteFile, for a caller that must stage
// two files and commit both (the skills manifest and registry).
func TestTwoStagedFilesCanBeCommittedOneAfterTheOther(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	writeFixture(t, a, "a-old", 0o600)
	writeFixture(t, b, "b-old", 0o600)

	stagedA, err := Stage(dir, []byte("a-new"), Options{Perm: 0o600})
	if err != nil {
		t.Fatal(err)
	}
	defer stagedA.Discard()
	stagedB, err := Stage(dir, []byte("b-new"), Options{Perm: 0o600})
	if err != nil {
		t.Fatal(err)
	}
	defer stagedB.Discard()

	if got := readFixture(t, a) + readFixture(t, b); got != "a-oldb-old" {
		t.Fatalf("staging changed the targets: %q", got)
	}
	if err := stagedA.Replace(a); err != nil {
		t.Fatal(err)
	}
	if err := stagedB.Replace(b); err != nil {
		t.Fatal(err)
	}
	if got := readFixture(t, a) + readFixture(t, b); got != "a-newb-new" {
		t.Errorf("targets = %q, want both replaced", got)
	}
	stagedA.Discard()
	stagedB.Discard()
	assertOnly(t, dir, "a", "b")
}

func TestDiscardRemovesAStagedFileThatWasNeverCommitted(t *testing.T) {
	dir := t.TempDir()
	staged, err := Stage(dir, []byte("x"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	staged.Discard()
	assertOnly(t, dir)
	staged.Discard() // twice is harmless
}

func TestAStagedFileCanBeCommittedOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	staged, err := Stage(dir, []byte("x"), Options{Perm: 0o600})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if err := staged.Replace(filepath.Join(dir, "one")); err != nil {
		t.Fatal(err)
	}
	if err := staged.Replace(filepath.Join(dir, "two")); err == nil {
		t.Error("second Replace = nil, want an error: the staged file was consumed")
	}
	if err := staged.Create(filepath.Join(dir, "three")); err == nil {
		t.Error("Create after Replace = nil, want an error: the staged file was consumed")
	}
	assertOnly(t, dir, "one")
}

func TestCreatePublishesANewNameAndNeverReplacesAnExistingOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")

	first, err := Stage(dir, []byte("first"), Options{Perm: 0o600, Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Discard()
	if err := first.Create(path); err != nil {
		t.Fatalf("Create on a free name: %v", err)
	}
	first.Discard()
	if got := modeOf(t, path); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}

	second, err := Stage(dir, []byte("second"), Options{Perm: 0o600})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Discard()
	err = second.Create(path)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Create over an existing name = %v, want fs.ErrExist", err)
	}
	second.Discard()
	if got := readFixture(t, path); got != "first" {
		t.Errorf("record = %q, want the first, untouched", got)
	}
	assertOnly(t, dir, "record.json")
}

func TestCreateFlushesTheDirectoryWhenSyncIsAsked(t *testing.T) {
	dir := t.TempDir()
	s := &spy{}
	staged, err := stage(s.ops(), dir, []byte("x"), Options{Perm: 0o600, Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if err := staged.Create(filepath.Join(dir, "r")); err != nil {
		t.Fatal(err)
	}
	want := []string{"sync-file", "sync-dir:" + filepath.Base(dir)}
	if strings.Join(s.events, ",") != strings.Join(want, ",") {
		t.Errorf("operations = %v, want %v", s.events, want)
	}
}

// Two writers racing for one name: exactly one publishes, the other is told the
// name is taken, and the winner's bytes are what is stored.
func TestRacingCreatesPublishExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record")
	const racers = 16

	var wg sync.WaitGroup
	results := make([]error, racers)
	for i := 0; i < racers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			staged, err := Stage(dir, []byte{byte('A' + i)}, Options{Perm: 0o600})
			if err != nil {
				results[i] = err
				return
			}
			defer staged.Discard()
			results[i] = staged.Create(path)
		}()
	}
	wg.Wait()

	winners := 0
	for i, err := range results {
		switch {
		case err == nil:
			winners++
		case !errors.Is(err, fs.ErrExist):
			t.Errorf("racer %d: %v, want success or fs.ErrExist", i, err)
		}
	}
	if winners != 1 {
		t.Errorf("%d racers published, want exactly 1", winners)
	}
	assertOnly(t, dir, "record")
}
