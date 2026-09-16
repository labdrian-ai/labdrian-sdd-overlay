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
		{Name: ".agents/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: ".agents/skills/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: ".agents/skills/archify/", Typeflag: tar.TypeDir, Mode: 0755},
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

// TestExtractTar_AbsoluteLinknameRefused (R2/R3-abs-symlink-escape): a
// symlink whose Linkname is an absolute path must be refused outright, even
// though a purely lexical filepath.Join(parentDir, linkTarget) would make it
// look like it resolves inside dest -- filepath.Join treats an absolute
// second argument as just another path segment, so that lexical check alone
// is not a containment guarantee.
func TestExtractTar_AbsoluteLinknameRefused(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "skills/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "skills/escape", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0777},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err == nil {
		t.Fatal("extractTar: want an error for an absolute symlink target, got nil")
	}
	if _, err := os.Lstat(filepath.Join(dest, "skills", "escape")); err == nil {
		t.Error("extractTar: symlink with an absolute target must not be created on disk")
	}
}

// TestExtractTar_DanglingThroughMissingIntermediateAllowed: a legitimate
// dangling symlink whose target traverses a directory that does not exist
// in this export at all (not just as its final component -- e.g.
// skills/archify -> ../.agents/skills/archify, where .agents is never part
// of exportGitTree's path set) must still be recreated, exactly like the
// simpler single-missing-component case already covered by
// TestExtractTar_ContainedSymlinkRecreated's setup (which pre-creates
// .agents/). This reproduces the real regression: without pre-creating
// .agents/, only the FINAL path component was missing in that test, never
// an intermediate one.
func TestExtractTar_DanglingThroughMissingIntermediateAllowed(t *testing.T) {
	data := writeTarEntries(t, []tar.Header{
		{Name: "skills/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "skills/archify", Typeflag: tar.TypeSymlink, Linkname: "../.agents/skills/archify", Mode: 0777},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extractTar: unexpected error for a symlink dangling through a missing intermediate directory: %v", err)
	}
	target := filepath.Join(dest, "skills", "archify")
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("Lstat(%s): %v", target, err)
	}
}

// TestExtractTar_ChainedSymlinkEscapeRefused (R1/R3-symlink-chain-escape): an
// earlier symlink entry that looks contained lexically (it points at ".",
// its own directory) can still be used to walk back out of dest if a later
// entry's containment check only reasons about strings and never re-resolves
// through symlinks an earlier entry already created on disk. extractTar must
// resolve the parent directory through any real symlinks before trusting it.
func TestExtractTar_ChainedSymlinkEscapeRefused(t *testing.T) {
	// "loop" is a symlink to ".", i.e. to its own parent directory -- lexically
	// contained, but it makes anything written "under" loop/ physically alias
	// dest itself. A naive lexical check on a later entry named
	// loop/loop/loop/../../escape cancels the loop/../.. pairs on paper and
	// still looks contained, while physically resolving right back out to
	// dest's parent.
	data := writeTarEntries(t, []tar.Header{
		{Name: "loop", Typeflag: tar.TypeSymlink, Linkname: ".", Mode: 0777},
		{Name: "loop/loop/loop/escape", Typeflag: tar.TypeSymlink, Linkname: "../../../../outside", Mode: 0777},
	})

	dest := t.TempDir()
	err := extractTar(bytes.NewReader(data), dest)
	if err == nil {
		t.Fatal("extractTar: want an error for a chained symlink escape, got nil")
	}
	if _, statErr := os.Lstat(filepath.Join(dest, "loop", "loop", "loop", "escape")); statErr == nil {
		t.Error("extractTar: chained-escape symlink must not be created on disk")
	}
}
