package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func hashOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func newTestPromptStore(t *testing.T) (promptStore, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", home)
	s, err := newPromptStore()
	if err != nil {
		t.Fatal(err)
	}
	return s, home
}

// promptSessionFile is where the blocks of a session are kept: the session is hashed, so its name never
// reaches the path.
func promptSessionFile(home, session string) string {
	return filepath.Join(home, "labdrian", "projection-prompt", hashOf(session), "last")
}

func TestAPromptStoreNobodyWroteToHasNoLastBlockAndMakesNothing(t *testing.T) {
	s, home := newTestPromptStore(t)
	got, err := s.last("s1")
	if err != nil || got != "" {
		t.Fatalf("last() = %q, %v, want no block and no error", got, err)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Errorf("last made %d entries under the state home, want none", len(entries))
	}
}

func TestThePromptStoreKeepsTheLastBlockOfEachSession(t *testing.T) {
	s, home := newTestPromptStore(t)
	a, b := hashOf("block a"), hashOf("block b")
	for _, h := range []string{a, b, a} {
		if err := s.record("s1", h); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.last("s1"); err != nil || got != a {
		t.Fatalf("last(s1) = %q, %v, want %q", got, err, a)
	}
	if got, err := s.last("s2"); err != nil || got != "" {
		t.Fatalf("last(s2) = %q, %v, want none: another session", got, err)
	}
	data, err := os.ReadFile(promptSessionFile(home, "s1"))
	if err != nil || string(data) != a+"\n"+b+"\n"+a+"\n" {
		t.Errorf("file = %q, %v, want one line for each change", data, err)
	}
}

func TestRecordingThePromptStoresLastBlockAgainAddsNothing(t *testing.T) {
	s, home := newTestPromptStore(t)
	h := hashOf("x")
	_ = s.record("s1", h)
	_ = s.record("s1", h)
	if data, _ := os.ReadFile(promptSessionFile(home, "s1")); string(data) != h+"\n" {
		t.Errorf("file = %q, want one line", data)
	}
}

func TestAPromptStoreResetMakesTheNextBlockNew(t *testing.T) {
	s, home := newTestPromptStore(t)
	h := hashOf("x")
	if err := s.reset("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(promptSessionFile(home, "s1")); err == nil {
		t.Errorf("a reset of a session with no block made a file")
	}
	_ = s.record("s1", h)
	if err := s.reset("s1"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.last("s1"); err != nil || got != "" {
		t.Fatalf("last() after a reset = %q, %v, want none", got, err)
	}
	_ = s.record("s1", h)
	if got, _ := s.last("s1"); got != h {
		t.Errorf("last() = %q, want %q recorded again after the reset", got, h)
	}
}

func TestAPromptStoreThatCannotBeUsedSaysSo(t *testing.T) {
	s, home := newTestPromptStore(t)
	if err := os.MkdirAll(filepath.Join(home, "labdrian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "labdrian", "projection-prompt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.last("s1"); err == nil {
		t.Errorf("last() over a file where the directory belongs = nil error")
	}
	if err := s.record("s1", hashOf("x")); err == nil {
		t.Errorf("record() over a file where the directory belongs = nil error")
	}
	if err := s.reset("s1"); err == nil {
		t.Errorf("reset() over a file where the directory belongs = nil error")
	}
}

func TestAGarbledLastLineIsNoPromptBlockAndASymlinkedFileIsRefused(t *testing.T) {
	s, home := newTestPromptStore(t)
	_ = s.record("s1", hashOf("x"))
	path := promptSessionFile(home, "s1")
	if err := os.WriteFile(path, []byte(hashOf("x")+"\nnot a hash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.last("s1"); err != nil || got != "" {
		t.Errorf("last() over a garbled last line = %q, %v, want none", got, err)
	}
	target := filepath.Join(home, "elsewhere")
	_ = os.WriteFile(target, nil, 0o600)
	_ = os.Remove(path)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.last("s1"); err == nil {
		t.Errorf("last() over a symlink = nil error")
	}
	if err := s.record("s1", hashOf("y")); err == nil {
		t.Errorf("record() over a symlink = nil error")
	}
	if data, _ := os.ReadFile(target); len(data) != 0 {
		t.Errorf("a symlink was followed and %q written", data)
	}
}

func TestThePromptStoreIsRefusedARelativeStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative/state")
	if _, err := newPromptStore(); err == nil {
		t.Errorf("newPromptStore() over a relative state home = nil error")
	}
}
