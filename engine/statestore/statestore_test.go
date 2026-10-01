package statestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func skipUnlessSupported(t *testing.T) {
	t.Helper()
	if !Supported {
		t.Skipf("the state store is only implemented on linux and darwin, not %s", runtime.GOOS)
	}
}

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestHomeFromResolvesTheStateHome(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		want    string
		wantErr string
	}{
		{"an absolute XDG_STATE_HOME wins", map[string]string{"XDG_STATE_HOME": "/srv/state", "HOME": "/home/u"}, "/srv/state", ""},
		{"it is cleaned", map[string]string{"XDG_STATE_HOME": "/srv//state/./x/../"}, "/srv/state", ""},
		{"HOME is the fallback", map[string]string{"HOME": "/home/u"}, "/home/u/.local/state", ""},
		{"an empty XDG_STATE_HOME counts as unset", map[string]string{"XDG_STATE_HOME": "", "HOME": "/home/u"}, "/home/u/.local/state", ""},
		{"a relative XDG_STATE_HOME is refused", map[string]string{"XDG_STATE_HOME": "state", "HOME": "/home/u"}, "", `XDG_STATE_HOME "state" is not absolute`},
		{"no HOME is refused", map[string]string{}, "", `XDG_STATE_HOME is unset and HOME "" is not an absolute path`},
		{"a relative HOME is refused", map[string]string{"HOME": "home/u"}, "", `XDG_STATE_HOME is unset and HOME "home/u" is not an absolute path`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := HomeFrom(env(tc.vars))
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q (no store name: each caller adds its own)", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("HomeFrom = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestHomeReadsTheProcessEnvironment(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	got, err := Home()
	if err != nil || got != root {
		t.Errorf("Home = %q, %v, want %q", got, err, root)
	}
}

func mode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func TestEnsureDirsCreatesTheChainPrivately(t *testing.T) {
	skipUnlessSupported(t)
	home := filepath.Join(t.TempDir(), "not", "yet", "state")
	parts := []string{home, "labdrian", "workflows", "proj"}

	if err := EnsureDirs(parts); err != nil {
		t.Fatal(err)
	}
	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if got := mode(t, current); !got.IsDir() || got.Perm() != 0o700 {
			t.Errorf("%s: mode = %v, want a directory with 0700", current, got)
		}
	}
	if got := mode(t, home); got.Perm() != 0o700 {
		t.Errorf("the state home was created %v, want 0700", got.Perm())
	}
	if err := EnsureDirs(parts); err != nil {
		t.Errorf("a second EnsureDirs: %v", err)
	}
}

// What exists is accepted as it is: the store never loosens or tightens a
// directory it did not create.
func TestEnsureDirsLeavesExistingDirectoriesAlone(t *testing.T) {
	skipUnlessSupported(t)
	home := t.TempDir()
	existing := filepath.Join(home, "labdrian")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDirs([]string{home, "labdrian", "bindings"}); err != nil {
		t.Fatal(err)
	}
	if got := mode(t, existing).Perm(); got != 0o755 {
		t.Errorf("an existing directory was changed to %v", got)
	}
}

func TestEnsureDirsRefusesASymlinkedOrNonDirectoryComponent(t *testing.T) {
	skipUnlessSupported(t)
	t.Run("symlinked component", func(t *testing.T) {
		home := t.TempDir()
		elsewhere := t.TempDir()
		if err := os.Symlink(elsewhere, filepath.Join(home, "labdrian")); err != nil {
			t.Fatal(err)
		}
		err := EnsureDirs([]string{home, "labdrian", "workflows"})
		if !errors.Is(err, ErrSymlink) || !strings.Contains(err.Error(), "refusing symlinked store component") {
			t.Fatalf("error = %v, want ErrSymlink naming the component", err)
		}
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Errorf("something was created through the link: %v", entries)
		}
	})
	t.Run("a file in the way", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "labdrian"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		err := EnsureDirs([]string{home, "labdrian"})
		if !errors.Is(err, ErrNotDir) || !strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("error = %v, want ErrNotDir", err)
		}
	})
	t.Run("the state home is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "home")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		err := EnsureDirs([]string{file, "labdrian"})
		if err == nil || !strings.Contains(err.Error(), "state home") || !strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("error = %v, want a refusal naming the state home", err)
		}
	})
}

// The state home is the user's own choice and may be a link into another volume;
// the store components below it are ours and may not.
func TestTheStateHomeItselfMayBeASymlink(t *testing.T) {
	skipUnlessSupported(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDirs([]string{link, "labdrian"}); err != nil {
		t.Fatalf("EnsureDirs under a symlinked state home: %v", err)
	}
	if err := CheckDirs([]string{link, "labdrian"}); err != nil {
		t.Fatalf("CheckDirs under a symlinked state home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(real, "labdrian")); err != nil {
		t.Errorf("the component was not created behind the link: %v", err)
	}
}

func TestCheckDirsReadsAndNeverCreates(t *testing.T) {
	skipUnlessSupported(t)
	home := t.TempDir()
	parts := []string{home, "labdrian", "bindings"}

	if err := CheckDirs(parts); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("CheckDirs on an empty home = %v, want fs.ErrNotExist (absent)", err)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("CheckDirs created %v", entries)
	}

	if err := EnsureDirs(parts); err != nil {
		t.Fatal(err)
	}
	if err := CheckDirs(parts); err != nil {
		t.Errorf("CheckDirs on a made chain = %v", err)
	}
}

func TestCheckDirsRefusesASymlinkedOrNonDirectoryComponent(t *testing.T) {
	skipUnlessSupported(t)
	home := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "labdrian")); err != nil {
		t.Fatal(err)
	}
	if err := CheckDirs([]string{home, "labdrian", "bindings"}); !errors.Is(err, ErrSymlink) {
		t.Errorf("symlinked component: error = %v, want ErrSymlink", err)
	}

	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "labdrian"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckDirs([]string{other, "labdrian"}); !errors.Is(err, ErrNotDir) {
		t.Errorf("file component: error = %v, want ErrNotDir", err)
	}
}

func TestReadFileReturnsTheContent(t *testing.T) {
	skipUnlessSupported(t)
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(path, 0)
	if err != nil || string(got) != "hello world" {
		t.Errorf("ReadFile(no limit) = %q, %v", got, err)
	}
	got, err = ReadFile(path, 5)
	if err != nil || string(got) != "hello" {
		t.Errorf("ReadFile(limit 5) = %q, %v, want the first 5 bytes (a caller reads one past its cap to see an overflow)", got, err)
	}
}

func TestReadFileRefusesWhatIsNotAPlainFile(t *testing.T) {
	skipUnlessSupported(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{link, dangling} {
		if _, err := ReadFile(path, 0); !errors.Is(err, ErrSymlink) {
			t.Errorf("ReadFile(%s) = %v, want ErrSymlink", filepath.Base(path), err)
		}
	}
	if _, err := ReadFile(dir, 0); !errors.Is(err, ErrNotRegular) {
		t.Errorf("ReadFile(directory) = %v, want ErrNotRegular", err)
	}
	if _, err := ReadFile(filepath.Join(dir, "absent"), 0); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(absent) = %v, want fs.ErrNotExist", err)
	}
}

func TestOpenNoFollowReportsTheSymlinkRefusal(t *testing.T) {
	skipUnlessSupported(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	f, err := OpenNoFollow(real)
	if err != nil {
		t.Fatalf("OpenNoFollow(plain file): %v", err)
	}
	f.Close()
	if f, err := OpenNoFollow(link); err == nil {
		f.Close()
		t.Fatal("OpenNoFollow(symlink) = nil, want a refusal")
	} else if !IsSymlinkRefusal(err) {
		t.Errorf("IsSymlinkRefusal(%v) = false, want true", err)
	}
	if IsSymlinkRefusal(errors.New("something else")) || IsSymlinkRefusal(nil) {
		t.Error("IsSymlinkRefusal is true for an error that is not the symlink refusal")
	}
}

func TestRecordsArePublishedPrivatelyAndDurably(t *testing.T) {
	opts := recordOptions()
	if opts.Perm != 0o600 || !opts.Sync {
		t.Errorf("record options = %+v, want Perm 0600 and Sync on", opts)
	}
	if opts.Backup {
		t.Error("a record is never backed up: it is never replaced")
	}
}

func TestPublishStoresANewRecordPrivately(t *testing.T) {
	skipUnlessSupported(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "000001.json")

	if err := Publish(path, []byte(`{"seq":1}`)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != `{"seq":1}` {
		t.Errorf("record = %q", got)
	}
	if got := mode(t, path).Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the record (no temporary file)", len(entries))
	}
}

func TestPublishingTheSameBytesAgainIsAnIdempotentNoOp(t *testing.T) {
	skipUnlessSupported(t)
	path := filepath.Join(t.TempDir(), "r.json")
	if err := Publish(path, []byte("same")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := Publish(path, []byte("same")); err != nil {
		t.Fatalf("second Publish of identical bytes: %v", err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Error("the record was replaced although the bytes were identical")
	}
}

func TestPublishNeverReplacesARecordWithDifferentBytes(t *testing.T) {
	skipUnlessSupported(t)
	path := filepath.Join(t.TempDir(), "r.json")
	if err := Publish(path, []byte("first")); err != nil {
		t.Fatal(err)
	}

	err := Publish(path, []byte("second"))
	if !errors.Is(err, ErrImmutable) || !strings.Contains(err.Error(), "refusing to replace immutable record") {
		t.Fatalf("error = %v, want ErrImmutable", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Errorf("record = %q, want the first, untouched", got)
	}
}

func TestPublishRefusesASymlinkAtTheRecordName(t *testing.T) {
	skipUnlessSupported(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "r.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := Publish(link, []byte("x")); !errors.Is(err, ErrSymlink) {
		t.Errorf("Publish over a symlink = %v, want ErrSymlink even when the bytes match", err)
	}
	dangling := filepath.Join(dir, "d.json")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}
	if err := Publish(dangling, []byte("x")); !errors.Is(err, ErrSymlink) {
		t.Errorf("Publish over a dangling symlink = %v, want ErrSymlink", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nowhere")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("Publish wrote through a dangling symlink")
	}
}

func TestPublishInAMissingDirectoryFails(t *testing.T) {
	skipUnlessSupported(t)
	err := Publish(filepath.Join(t.TempDir(), "absent", "r.json"), []byte("x"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want fs.ErrNotExist", err)
	}
}

// Racing publishers of different bytes: exactly one record is stored and every
// loser is told so. Racing publishers of the same bytes all succeed.
func TestRacingPublishersStoreOneRecord(t *testing.T) {
	skipUnlessSupported(t)
	const racers = 12

	race := func(name string, payload func(i int) []byte) (winners int, immutable int, other []error) {
		path := filepath.Join(t.TempDir(), name)
		results := make([]error, racers)
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = Publish(path, payload(i))
			}()
		}
		wg.Wait()
		for _, err := range results {
			switch {
			case err == nil:
				winners++
			case errors.Is(err, ErrImmutable):
				immutable++
			default:
				other = append(other, err)
			}
		}
		return winners, immutable, other
	}

	if winners, immutable, other := race("different", func(i int) []byte { return []byte{byte('a' + i)} }); len(other) != 0 || winners != 1 || immutable != racers-1 {
		t.Errorf("different bytes: %d winners, %d refused, other errors %v; want 1 and %d", winners, immutable, other, racers-1)
	}
	if winners, immutable, other := race("same", func(int) []byte { return []byte("same") }); len(other) != 0 || winners != racers || immutable != 0 {
		t.Errorf("same bytes: %d winners, %d refused, other errors %v; want all %d to succeed", winners, immutable, other, racers)
	}
}
