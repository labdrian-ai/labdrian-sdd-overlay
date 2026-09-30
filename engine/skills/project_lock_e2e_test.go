package skills

// The project lock against the BUILT engine binary: real processes started at the
// same moment against one project root, in fixtures under t.TempDir(). See
// registry_lock_e2e_test.go for the environment they run in and for
// SKILLS_E2E_ENGINE_BINARY, which measures an older build with these same tests.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
)

// registerJob is one `skills project-register` of skill id into the project at root.
func registerJob(t *testing.T, w lockE2E, root, registryPath, id string) func() e2eRun {
	t.Helper()
	draft := filepath.Join(t.TempDir(), "SKILL.md")
	writeTestFile(t, draft, string(validDraft(id)))
	args := []string{"project-register", "--project-root", root, "--candidate", "procedural/candidates/repeated-success/" + id, "--registry", registryPath, draft}
	return func() e2eRun { return w.engine(args...) }
}

// Many registrations of different skills into one project at once: every one that
// exits 0 is in the project lock and has its files, and no two runs interleave the
// read and the write of the lock file. Exit 2 (busy) is allowed and counted.
func TestProjectLockE2E_ConcurrentRegistrationsAllLandInTheProjectLock(t *testing.T) {
	bin := lockE2EBinary(t)
	const rounds, perRound = 8, 8

	var ok, busy, other, lost, missingFiles int
	var samples []string
	for round := 0; round < rounds; round++ {
		w := newLockE2E(t, bin, []string{"base"}, nil)
		root := filepath.Join(t.TempDir(), "project")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, perRound)
		jobs := make([]func() e2eRun, perRound)
		for i := range jobs {
			ids[i] = fmt.Sprintf("skill-%d", i)
			jobs[i] = registerJob(t, w, root, w.reg, ids[i])
		}
		results := runAll(jobs)

		registered := map[string]bool{}
		if data, err := os.ReadFile(projectLockFile(root)); err == nil {
			lock, perr := ParseProjectLock(data)
			if perr != nil {
				t.Fatalf("round %d: the project lock does not parse: %v\n%s", round, perr, data)
			}
			for _, e := range lock.Skills {
				registered[e.ID] = true
			}
		}
		for i, r := range results {
			switch {
			case r.code == ExitBusy:
				busy++
			case r.code == 0:
				ok++
				if !registered[ids[i]] {
					lost++
				}
				for _, dir := range []string{".claude", ".agents"} {
					if _, err := os.Stat(filepath.Join(root, dir, "skills", ids[i], "SKILL.md")); err != nil {
						missingFiles++
					}
				}
			default:
				other++
				if len(samples) < 4 {
					samples = append(samples, strings.TrimSpace(r.stderr))
				}
			}
		}
	}

	summary := fmt.Sprintf("%d rounds of %d project-register processes into one project: ok %d, busy %d; ok registrations missing from the project lock %d, missing skill files %d, other failures %d",
		rounds, perRound, ok, busy, lost, missingFiles, other)
	t.Log(summary)
	if lost+missingFiles+other != 0 {
		t.Errorf("concurrent registrations were not serialized: %s\nfailure samples: %v", summary, samples)
	}
	if ok == 0 {
		t.Errorf("no registration succeeded at all: %s", summary)
	}
}

// installs and registrations into one project, with an overlay writer and readers
// running beside them: install holds the overlay lock and then the project lock,
// and nothing that holds one waits for the other in the other order, so the whole
// run finishes (each process is bounded to a minute by the harness, and a lock
// wait to two seconds) and loses nothing.
func TestProjectLockE2E_InstallsAndRegistrationsShareOneProjectWithoutLosingAnything(t *testing.T) {
	bin := lockE2EBinary(t)
	for round := 0; round < 5; round++ {
		f := newInstallFixture(t)
		w := lockE2E{t: t, bin: bin, dir: f.dir, reg: f.reg, man: f.man, root: f.root, home: t.TempDir(), state: t.TempDir(), lockPath: RegistryLockPath(f.reg)}

		var jobs []func() e2eRun
		var kinds []string
		add := func(kind string, job func() e2eRun) {
			jobs = append(jobs, job)
			kinds = append(kinds, kind)
		}
		installArgs := append([]string{"install"}, f.installArgs()...)
		for i := 0; i < 4; i++ {
			add("install", func() e2eRun { return w.engineIn(f.project, installArgs...) })
		}
		ids := []string{"reg-0", "reg-1", "reg-2", "reg-3"}
		for _, id := range ids {
			add("register", registerJob(t, w, f.project, f.reg, id))
		}
		for i := 0; i < 2; i++ {
			add("sync", func() e2eRun { return w.engine(append([]string{"sync-manifest"}, w.flags()...)...) })
			add("validate", func() e2eRun { return w.engine(append([]string{"validate"}, w.flags()...)...) })
		}
		results := runAll(jobs)

		registered := map[string]bool{}
		installRecorded := false
		if data, err := os.ReadFile(projectLockFile(f.project)); err == nil {
			lock, perr := ParseProjectLock(data)
			if perr != nil {
				t.Fatalf("round %d: the project lock does not parse: %v", round, perr)
			}
			for _, e := range lock.Skills {
				registered[e.ID] = true
			}
			for _, in := range lock.Installs {
				installRecorded = installRecorded || in.ID == "proj"
			}
		}
		installed := 0
		for i, r := range results {
			switch {
			case r.code == ExitBusy:
			case r.code != 0:
				t.Errorf("round %d: %s: exit %d, stderr %q", round, kinds[i], r.code, r.stderr)
			case kinds[i] == "register":
				if !registered[ids[i-4]] {
					t.Errorf("round %d: %s exited 0 but is not in the project lock", round, ids[i-4])
				}
			case kinds[i] == "install":
				installed++
			}
		}
		if installed > 0 {
			for _, runtime := range []string{".claude", ".agents"} {
				if _, err := os.Stat(filepath.Join(f.project, runtime, "skills", "proj", "SKILL.md")); err != nil {
					t.Errorf("round %d: an install exited 0 but the skill is not in %s: %v", round, runtime, err)
				}
			}
			// The install record and the procedural entries share one lock file, and
			// racing processes must not lose either: an install that exited 0 while a
			// registration overwrote the lock would leave its files foreign.
			if !installRecorded {
				t.Errorf("round %d: an install exited 0 but its record is not in the project lock", round)
			}
		}
		if reg, man := registryIDs(t, f.reg), manifestIDs(t, f.man); !reflect.DeepEqual(reg, man) {
			t.Errorf("round %d: registry lists %v but the manifest lists %v", round, reg, man)
		}
	}
}

// A project root locked by another process is reported as exit 2 after the bound,
// with nothing written into the project, and the lock leaves no file behind.
func TestProjectLockE2E_ABusyProjectIsReportedAsExit2AndNothingIsWritten(t *testing.T) {
	bin := lockE2EBinary(t)
	w := newLockE2E(t, bin, []string{"base"}, nil)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	job := registerJob(t, w, root, w.reg, "skill-0")

	hold, err := filelock.AcquireDir(root, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	r := job()
	hold()

	if r.code != ExitBusy || r.stdout != "" {
		t.Fatalf("project-register while the project is locked: exit %d, stdout %q, stderr %q, want exit 2 and no stdout", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"in progress", "retry", "the project " + root} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if r.elapsed < filelock.DefaultWait-200*time.Millisecond || r.elapsed > 20*time.Second {
		t.Errorf("took %v, want about the %v bound: neither an instant refusal nor a hang", r.elapsed, filelock.DefaultWait)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a refused project-register left %d entries in the project", len(entries))
	}
}
