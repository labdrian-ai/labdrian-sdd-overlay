package pipkg

// Internal (package pipkg) tests for extractTar, which is unexported and
// only otherwise exercised indirectly through exportGitTree/Check (see
// pipkg_provenance_test.go's TestExportGitTree_DrainsPipeOnExtractionError).
// These tests build a tar stream directly, the same shape `git archive`
// produces, to verify extractTar's symlink/hard-link handling without a
// real git repository.

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeTarEntries builds a tar stream from the given headers (Linkname and
// Size/content already set as needed by the caller) and returns the bytes.
func writeTarEntries(t *testing.T, entries []tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, hdr := range entries {
		h := hdr
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("WriteHeader(%s): %v", hdr.Name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	return buf.Bytes()
}

// TestExtractTar_ContainedSymlinkRecreated: a git-archive-produced symlink
// entry whose link target resolves inside dest (e.g. skills/archify ->
// ../.agents/skills/archify, still inside dest) must now be recreated as a
// real symlink on disk instead of being refused, since buildInto only ever
// reads registered skills out of the exported tree and never dereferences
// an unregistered symlink like this.
func TestExtractTar_ContainedSymlinkRecreated(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "skills/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "skills/archify", Typeflag: tar.TypeSymlink, Linkname: "../.agents/skills/archify", Mode: 0777},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extractTar: unexpected error for a contained symlink: %v", err)
	}

	target := filepath.Join(dest, "skills", "archify")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", target, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s: want a real symlink on disk, got mode %v", target, info.Mode())
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", target, err)
	}
	if got != "../.agents/skills/archify" {
		t.Errorf("Readlink(%s) = %q, want %q", target, got, "../.agents/skills/archify")
	}
}

// TestExtractTar_EscapingSymlinkRefused: a symlink whose link target would
// resolve outside dest must still be refused, exactly like the existing
// zip-slip guard on entry names.
func TestExtractTar_EscapingSymlinkRefused(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "skills/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "skills/escape", Typeflag: tar.TypeSymlink, Linkname: "../../../../etc/passwd", Mode: 0777},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err == nil {
		t.Fatal("extractTar: want an error for a symlink escaping dest, got nil")
	}

	if _, err := os.Lstat(filepath.Join(dest, "skills", "escape")); err == nil {
		t.Error("extractTar: escaping symlink must not be created on disk")
	}
}

// TestExtractTar_HardLinkRefused: a hard-link tar entry must still be
// refused unconditionally -- there is no legitimate use case for one in a
// git archive of tracked content.
func TestExtractTar_HardLinkRefused(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "skills/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "skills/real", Typeflag: tar.TypeReg, Mode: 0644, Size: 0},
		{Name: "skills/hardlink", Typeflag: tar.TypeLink, Linkname: "skills/real", Mode: 0644},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err == nil {
		t.Fatal("extractTar: want an error for a hard-link entry, got nil")
	}
}

// TestExtractTar_AbsoluteLinknameRefused (R2/R3): an absolute Linkname
// must be refused outright, before any resolution.
func TestExtractTar_AbsoluteLinknameRefused(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "skills/escape", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0777},
	})
	if err := extractTar(bytes.NewReader(data), t.TempDir()); err == nil {
		t.Fatal("want an error for an absolute symlink target")
	}
}

// TestExtractTar_ChainedSymlinkEscapeRefused (R1/R3): "b"->"." plus
// Linkname "b/b/b/../../secret" lexically looks contained but escapes.
func TestExtractTar_ChainedSymlinkEscapeRefused(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "b", Typeflag: tar.TypeSymlink, Linkname: ".", Mode: 0777},
		{Name: "escape", Typeflag: tar.TypeSymlink, Linkname: "b/b/b/../../secret", Mode: 0777},
	})
	if err := extractTar(bytes.NewReader(data), t.TempDir()); err == nil {
		t.Fatal("want an error for a chained symlink escape")
	}
}
