package projection

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These tests reach two defensive paths the public API cannot: Bind classifies
// the file before it writes, so a failing publish or an oversized read can only
// be produced by calling the helpers directly. They write only under
// t.TempDir().

// TestWriteBindingRemovesItsTemporaryFileWhenThePublishFails makes the final
// rename fail (a file cannot be renamed over a non-empty directory) and checks
// that the temporary file written just before it does not stay behind.
func TestWriteBindingRemovesItsTemporaryFileWhenThePublishFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := writeBinding(target, []byte("{}\n"))
	if err == nil || !strings.Contains(err.Error(), "publish") {
		t.Fatalf("writeBinding() = %v, want a publish error", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"target.json"}) {
		t.Fatalf("directory holds %v after the failed publish, want only the original target", names)
	}
}

// TestReadBindingFileStopsOneByteAfterTheCap pins that a large file is not read
// in full: one byte past MaxBindingBytes is all classifyBinding needs to see
// that the file is too large.
func TestReadBindingFileStopsOneByteAfterTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := readBindingFile(path)
	if err != nil {
		t.Fatalf("readBindingFile() = %v, want nil", err)
	}
	if len(data) != MaxBindingBytes+1 {
		t.Fatalf("readBindingFile() read %d bytes of a 1 MiB file, want %d", len(data), MaxBindingBytes+1)
	}
}
