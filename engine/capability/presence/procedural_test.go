package presence_test

// The memory:procedural-skills signal of C23 M8 (docs/architecture/adr-c23-m8-session-availability.md):
// it used to be a fixed "unavailable", so the record of a workflow's creation said no procedural
// skill could be read even in a project that has a lock. It now stats the lock of the project, and
// like every signal here never opens it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

func TestTheProceduralSignalIsAvailableWhenTheProjectHasALock(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "procedural-skills.lock.json")
	if err := os.WriteFile(lock, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := probeOne(t, presence.Prober{ProceduralLock: lock}, "memory:procedural-skills")
	if got.Status != workflow.ObservationAvailable || !strings.Contains(got.Detail, "not opened, so its validity is unverified") {
		t.Errorf("observation = %+v, want available, stating that the lock was not opened", got)
	}
	if strings.Contains(got.Detail, lock) {
		t.Errorf("detail %q names the path", got.Detail)
	}
}

func TestTheProceduralSignalIsUnavailableWithoutALockOrWithoutAProject(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "procedural-skills.lock.json")
	got := probeOne(t, presence.Prober{ProceduralLock: missing}, "memory:procedural-skills")
	if got.Status != workflow.ObservationUnavailable || !strings.HasPrefix(got.Detail, "the procedural skills lock was not found") {
		t.Errorf("no lock: observation = %+v, want unavailable, saying the lock was not found", got)
	}
	dir := filepath.Dir(missing)
	got = probeOne(t, presence.Prober{ProceduralLock: dir}, "memory:procedural-skills")
	if got.Status != workflow.ObservationUnavailable {
		t.Errorf("a directory in the lock's place: observation = %+v, want unavailable", got)
	}
	for _, path := range []string{"", "relative/procedural-skills.lock.json"} {
		got = probeOne(t, presence.Prober{ProceduralLock: path}, "memory:procedural-skills")
		if got.Status != workflow.ObservationUnavailable || !strings.Contains(got.Detail, "the project directory is unknown") {
			t.Errorf("lock path %q: observation = %+v, want unavailable because the project is unknown", path, got)
		}
	}
}
