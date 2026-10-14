package identityledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// The words of a ledger that cannot be written are the ones the ledger has always said them in, and they end up in
// front of whoever runs a command in a repository whose ledger is in the way (the warning of case 55 of the identity
// goldens is one). They were:
//
//	identityledger: marshal: <the encoder's words>
//	identityledger: create directory for <the ledger's file>: <the file system's words>
//	identityledger: <the words of durable.WriteFile>
//
// The sequence is durable.WriteJSON's now, and its encoding failure names the file; the ledger does not. The
// failures are put where they happen by handing the write a jsonWriter that fails as the real one does, so no
// file system is asked to break; one case at the end runs the real one over a temporary directory, for the two
// failures a directory can provoke.

// failing is a jsonWriter that records what it was asked to write and answers err.
type failing struct {
	err        error
	path       string
	value      any
	createPerm os.FileMode
}

func (f *failing) write(path string, v any, createPerm os.FileMode) error {
	f.path, f.value, f.createPerm = path, v, createPerm
	return f.err
}

func TestWrite_SaysAnEncodingFailureInTheWordsItHasAlwaysUsed(t *testing.T) {
	commonDir := t.TempDir()
	encoder := errors.New("json: unsupported type: chan int")
	writer := &failing{err: &durable.EncodeError{Path: Path(commonDir), Err: encoder}}

	err := writeWith(writer.write, commonDir, ledger{})

	if want := "identityledger: marshal: json: unsupported type: chan int"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q: no file in it", err, want)
	}
	if !errors.Is(err, encoder) {
		t.Errorf("the encoder's error is not found through %v", err)
	}
}

func TestWrite_SaysAFailureToMakeTheDirectoryInTheWordsItHasAlwaysUsed(t *testing.T) {
	commonDir := t.TempDir()
	path := Path(commonDir)
	system := errors.New("mkdir " + filepath.Dir(path) + ": file exists")
	writer := &failing{err: fmt.Errorf("create directory for %s: %w", path, system)}

	err := writeWith(writer.write, commonDir, ledger{})

	if want := "identityledger: create directory for " + path + ": mkdir " + filepath.Dir(path) + ": file exists"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if !errors.Is(err, system) {
		t.Errorf("the file system's error is not found through %v", err)
	}
}

func TestWrite_SaysAFailureToReplaceTheFileInTheWordsOfTheReplacement(t *testing.T) {
	commonDir := t.TempDir()
	system := errors.New("device or resource busy")
	writer := &failing{err: fmt.Errorf("durable: rename temp file into %s: %w", Path(commonDir), system)}

	err := writeWith(writer.write, commonDir, ledger{})

	if want := "identityledger: durable: rename temp file into " + Path(commonDir) + ": device or resource busy"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if !errors.Is(err, system) {
		t.Errorf("the file system's error is not found through %v", err)
	}
}

// The ledger is written where it lives, owner-only, as the value it is, and a write that succeeds says nothing.
func TestWrite_WritesTheLedgerWhereItLivesOwnerOnly(t *testing.T) {
	commonDir := t.TempDir()
	want := ledger{Schema: Schema, Entries: []Name{{Name: "widgets", Rule: "remote", Adoptable: true}}}
	writer := &failing{}

	if err := writeWith(writer.write, commonDir, want); err != nil {
		t.Fatalf("writeWith: %v", err)
	}

	if writer.path != Path(commonDir) || writer.createPerm != 0o600 {
		t.Errorf("written at %q with mode %o, want %q with mode 600", writer.path, writer.createPerm, Path(commonDir))
	}
	if got, ok := writer.value.(ledger); !ok || got.Schema != want.Schema || len(got.Entries) != 1 {
		t.Errorf("written value = %#v, want the ledger", writer.value)
	}
}

// The real writer fails the two ways a directory can make it, and says them in the same words.
func TestWrite_TheRealWriterSaysTheFailuresADirectoryCanProvokeInTheSameWords(t *testing.T) {
	t.Run("the directory of the ledger is a file", func(t *testing.T) {
		commonDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(commonDir, dirName), []byte("a file where the directory should be\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := write(commonDir, ledger{})

		if want := "identityledger: create directory for " + Path(commonDir) + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("error = %v, want it to begin %q", err, want)
		}
	})
	t.Run("the ledger is a directory", func(t *testing.T) {
		commonDir := t.TempDir()
		if err := os.MkdirAll(Path(commonDir), 0o700); err != nil {
			t.Fatal(err)
		}

		err := write(commonDir, ledger{})

		if want := "identityledger: durable: "; err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("error = %v, want it to begin %q", err, want)
		}
	})
}
