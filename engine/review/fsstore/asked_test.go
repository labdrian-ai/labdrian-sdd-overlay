package fsstore_test

// What the stop hook keeps of the questions it has put (own-rdd T6a, S6): one empty marker for each session and
// question, under the state home of the user and outside every working tree, so that the hook blocks a stop once.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

const questionKey = "111111111111-222222222222"

func newAsked(t *testing.T) (fsstore.Asked, string) {
	t.Helper()
	home := t.TempDir()
	asked, err := fsstore.NewAsked(home)
	if err != nil {
		t.Fatalf("setup: NewAsked() error = %v", err)
	}
	return asked, home
}

func TestAskedRemembersAQuestionForTheSessionThatPutItAndNoOther(t *testing.T) {
	asked, home := newAsked(t)

	before, beforeErr := asked.Has("session-1", questionKey)
	addErr := asked.Add("session-1", questionKey)
	after, afterErr := asked.Has("session-1", questionKey)
	otherKey, otherKeyErr := asked.Has("session-1", "333333333333-444444444444")
	otherSession, otherSessionErr := asked.Has("session-2", questionKey)

	if beforeErr != nil || before || addErr != nil || afterErr != nil || !after {
		t.Errorf("Has = %v, %v; Add = %v; Has = %v, %v; want false, then stored, then true, with no error", before, beforeErr, addErr, after, afterErr)
	}
	if otherKeyErr != nil || otherKey || otherSessionErr != nil || otherSession {
		t.Errorf("another question = %v, %v and another session = %v, %v; want false for both", otherKey, otherKeyErr, otherSession, otherSessionErr)
	}
	sum := sha256.Sum256([]byte("session-1"))
	dir := filepath.Join(home, "labdrian", "review-stop", hex.EncodeToString(sum[:]))
	marker, err := os.Stat(filepath.Join(dir, questionKey))
	if err != nil || marker.Mode().Perm() != 0o600 {
		t.Fatalf("marker = %v, %v; want a private file under the hash of the session", marker, err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("directory of the session = %v, %v; want it private (0700)", info, err)
	}
	if raw, err := os.ReadDir(filepath.Join(home, "labdrian", "review-stop")); err != nil || len(raw) != 1 || strings.Contains(raw[0].Name(), "session") {
		t.Errorf("directories of the store = %v, %v; want one, named by the hash of the session and not by the session", raw, err)
	}
}

// Asking never makes anything: a store nobody wrote to is not made by being read.
func TestAskedHasMakesNothingAndRecordingTwiceChangesNothing(t *testing.T) {
	asked, home := newAsked(t)

	if has, err := asked.Has("session-1", questionKey); err != nil || has {
		t.Fatalf("Has() = %v, %v; want false and no error", has, err)
	}
	if _, err := os.Lstat(filepath.Join(home, "labdrian")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Has() made %s/labdrian (%v), want nothing made", home, err)
	}
	if err := asked.Add("session-1", questionKey); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := asked.Add("session-1", questionKey); err != nil {
		t.Errorf("the second Add() error = %v, want nil", err)
	}
}

// A key is one plain name: nothing a caller composes leads out of the directory of the session.
func TestAskedRefusesAKeyThatIsNotAPlainName(t *testing.T) {
	for _, key := range []string{"", ".", "..", "../x", "a/b", ".hidden", "UPPER", strings.Repeat("a", 65)} {
		t.Run(key, func(t *testing.T) {
			asked, home := newAsked(t)

			_, hasErr := asked.Has("session-1", key)
			addErr := asked.Add("session-1", key)

			if hasErr == nil || addErr == nil {
				t.Errorf("Has() error = %v and Add() error = %v, want both to refuse the key %q", hasErr, addErr, key)
			}
			if _, err := os.Lstat(filepath.Join(home, "labdrian")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a store was made (%v) for the key %q, want nothing made", err, key)
			}
		})
	}
}

// A link where a directory of the store belongs is refused, as it is for every store under the state home.
func TestAskedRefusesALinkWhereADirectoryOfTheStoreBelongs(t *testing.T) {
	asked, home := newAsked(t)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(home, "labdrian")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, hasErr := asked.Has("session-1", questionKey)
	addErr := asked.Add("session-1", questionKey)

	if !errors.Is(hasErr, statestore.ErrSymlink) || !errors.Is(addErr, statestore.ErrSymlink) {
		t.Errorf("Has() error = %v and Add() error = %v, want both to be ErrSymlink", hasErr, addErr)
	}
	if entries, err := os.ReadDir(elsewhere); err != nil || len(entries) != 0 {
		t.Errorf("%s holds %v (%v) after the refusals, want nothing written through the link", elsewhere, entries, err)
	}
}

func TestNewAskedRefusesAStateHomeThatIsNotAbsolute(t *testing.T) {
	if _, err := fsstore.NewAsked("relative/state"); err == nil {
		t.Error("NewAsked() of a relative state home error = nil, want a refusal")
	}
}
