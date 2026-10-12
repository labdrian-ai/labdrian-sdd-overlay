package vaultfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// Vault is the adapter behind the ports promote owns for the catalog and the log: the one a promotion registers
// a page through, and the read half the page check reads the catalog through.
var (
	_ promote.IndexRepository = (*Vault)(nil)
	_ promote.IndexReader     = (*Vault)(nil)
	_ promote.LogRepository   = (*Vault)(nil)
)

// A file of the vault the catalog tests are about: the catalog and the log are one file each, read and written
// on the same terms, so each test runs over both. The paths are written out as the literals a vault written by an
// earlier build holds, not derived from the layout under test.
type wikiFile struct {
	name     string
	rel      string
	read     func(*Vault) (string, error)
	write    func(*Vault, string) error
	location func(*Vault) string
}

var wikiFiles = []wikiFile{
	{"the catalog", "wiki/index.md", (*Vault).ReadIndex, (*Vault).WriteIndex, (*Vault).IndexLocation},
	{"the log", "wiki/log.md", (*Vault).ReadLog, (*Vault).WriteLog, (*Vault).LogLocation},
}

// writeVaultFile leaves content, at mode, as the vault-relative file rel.
func writeVaultFile(t *testing.T, root, rel, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestTheCatalogAndTheLogAreWhereTheVaultKeepsThem(t *testing.T) {
	root := filepath.Join("vaults", "brain")
	v := New(root)
	if got, want := v.IndexLocation(), filepath.Join(root, "wiki", "index.md"); got != want {
		t.Errorf("IndexLocation = %q, want %q", got, want)
	}
	if got, want := v.LogLocation(), filepath.Join(root, "wiki", "log.md"); got != want {
		t.Errorf("LogLocation = %q, want %q", got, want)
	}
}

// Reading returns the file byte for byte, through a link; a file that is not there is the file system's error
// for it, and any other failure is the file system's own: both name the file, and the adapter adds no word of its
// own to either, because the callers word the failure around the file they were reading.
func TestReadingTheCatalogAndTheLog(t *testing.T) {
	for _, file := range wikiFiles {
		t.Run(file.name, func(t *testing.T) {
			const content = "# Title\r\n\r\n  indented\n\ttabbed \n\n\n"

			t.Run("the content comes back whole", func(t *testing.T) {
				root := t.TempDir()
				writeVaultFile(t, root, file.rel, content, 0o644)
				got, err := file.read(New(root))
				if err != nil || got != content {
					t.Fatalf("read = (%q, %v), want %q", got, err, content)
				}
			})

			t.Run("through a link", func(t *testing.T) {
				root := t.TempDir()
				writeVaultFile(t, root, "notes/kept.md", content, 0o644)
				if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, "../notes/kept.md", filepath.Join(root, filepath.FromSlash(file.rel)))
				got, err := file.read(New(root))
				if err != nil || got != content {
					t.Fatalf("read = (%q, %v), want %q", got, err, content)
				}
			})

			t.Run("a file that is not there", func(t *testing.T) {
				_, err := file.read(New(t.TempDir()))
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("error = %v, want one for which errors.Is(err, fs.ErrNotExist) holds", err)
				}
				if strings.HasPrefix(err.Error(), "promote:") {
					t.Errorf("error %q carries a consumer's name: a read returns the file system's own error", err)
				}
			})

			for _, tc := range []struct {
				name  string
				setup func(t *testing.T, root, path string)
				want  string
			}{
				{"a directory", func(t *testing.T, _, path string) {
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Fatal(err)
					}
				}, "is a directory"},
				{"a link to nothing", func(t *testing.T, _, path string) {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					symlinkOrSkip(t, "../notes/absent.md", path)
				}, "no such file or directory"},
				{"the wiki directory a file", func(t *testing.T, root, _ string) {
					writeVaultFile(t, root, "wiki", "not a directory", 0o644)
				}, "not a directory"},
			} {
				t.Run("fails on "+tc.name, func(t *testing.T) {
					root := t.TempDir()
					path := filepath.Join(root, filepath.FromSlash(file.rel))
					tc.setup(t, root, path)
					_, err := file.read(New(root))
					if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), path) {
						t.Fatalf("error = %v, want the file system's, naming %s and saying %q", err, path, tc.want)
					}
					if strings.HasPrefix(err.Error(), "promote:") {
						t.Errorf("error %q carries a consumer's name: a read returns the file system's own error", err)
					}
				})
			}
		})
	}
}

// Writing replaces the file whole and atomically, the way the vault's other files are written: the wiki
// directory is created, a file that is there keeps its mode, a link is written through, a file the module
// creates is its owner's alone, and nothing is left beside the file.
func TestWritingTheCatalogAndTheLog(t *testing.T) {
	for _, file := range wikiFiles {
		t.Run(file.name, func(t *testing.T) {
			t.Run("creates the wiki directory and a file readable by its owner alone", func(t *testing.T) {
				root := t.TempDir()
				if err := file.write(New(root), "first\n"); err != nil {
					t.Fatalf("write: %v", err)
				}
				path := filepath.Join(root, filepath.FromSlash(file.rel))
				if data, err := os.ReadFile(path); err != nil || string(data) != "first\n" {
					t.Fatalf("file = (%q, %v), want %q", data, err, "first\n")
				}
				if got := modeOf(t, path); got != 0o600 {
					t.Errorf("mode of the new file = %v, want 0600", got)
				}
			})

			t.Run("replaces whole and keeps the mode of the file that is there", func(t *testing.T) {
				root := t.TempDir()
				path := writeVaultFile(t, root, file.rel, "a much longer old content than the new one\n", 0o644)
				if err := file.write(New(root), "new\n"); err != nil {
					t.Fatalf("write: %v", err)
				}
				if data, err := os.ReadFile(path); err != nil || string(data) != "new\n" {
					t.Fatalf("file = (%q, %v), want %q", data, err, "new\n")
				}
				if got := modeOf(t, path); got != 0o644 {
					t.Errorf("mode = %v, want the 0644 it had", got)
				}
				entries, err := os.ReadDir(filepath.Dir(path))
				if err != nil || len(entries) != 1 {
					t.Errorf("the wiki directory holds %v (%v), want the file alone: a temporary file was left behind", entries, err)
				}
			})

			t.Run("writes through a link and leaves the link", func(t *testing.T) {
				root := t.TempDir()
				target := writeVaultFile(t, root, "notes/kept.md", "old\n", 0o644)
				if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(root, filepath.FromSlash(file.rel))
				symlinkOrSkip(t, "../notes/kept.md", link)

				if err := file.write(New(root), "new\n"); err != nil {
					t.Fatalf("write: %v", err)
				}
				if data, _ := os.ReadFile(target); string(data) != "new\n" {
					t.Errorf("the file the link names holds %q, want %q", data, "new\n")
				}
				if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("the link was replaced by a file (%v, %v)", info, err)
				}
			})

			t.Run("a failure names what failed and leaves the file as it was", func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, filepath.FromSlash(file.rel))
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
				err := file.write(New(root), "new\n")
				if err == nil || !strings.HasPrefix(err.Error(), "durable: ") {
					t.Fatalf("error = %v, want one that begins with what failed", err)
				}
				if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
					t.Errorf("the directory in the file's place was disturbed (%v, %v)", info, statErr)
				}
			})

			t.Run("a file in the place of the wiki directory fails with what could not be created", func(t *testing.T) {
				root := t.TempDir()
				writeVaultFile(t, root, "wiki", "not a directory", 0o644)
				err := file.write(New(root), "new\n")
				if err == nil || strings.HasPrefix(err.Error(), "promote:") || !strings.HasPrefix(err.Error(), "create directory for ") {
					t.Fatalf("error = %v, want %q first", err, "create directory for ")
				}
			})
		})
	}
}
