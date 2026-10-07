package main

// Which registry `install` and `adopt` lock: the overlay lock is keyed by the registry a command
// line names, and the edge cases of reading that name (a flag with no value, a flag given twice, a
// value flag whose value looks like --registry) decide whether the lock protects the registry the
// verb reads.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// overlayLockTaken is the overlay lock the verb asked the locker for, or "" when it asked for none.
func overlayLockTaken(t *testing.T, verb string, args ...string) string {
	t.Helper()
	locker := &holdingLocker{}
	project := t.TempDir()
	deps := skills.Deps{
		Locker: locker,
		Cwd:    func() (string, error) { return project, nil },
	}
	run := skillsInstall
	if verb == "adopt" {
		run = skillsAdopt
	}
	runSkillsVerb(run, deps, append([]string{verb}, args...)...)
	for _, event := range locker.log() {
		if rest, ok := strings.CutPrefix(event, "lock shared "); ok {
			return rest
		}
	}
	return ""
}

func TestInstallAndAdoptLockTheRegistryTheCommandLineNames(t *testing.T) {
	dir := t.TempDir()
	reg := func(name string) string { return filepath.Join(dir, name) }
	lockOf := func(name string) string { return skills.RegistryLockPath(reg(name)) }
	defaultLock := skills.RegistryLockPath("skills.registry.yaml")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"the registry that is named", []string{"--registry", reg("a.yaml")}, lockOf("a.yaml")},
		{"the registry that is named, after other flags", []string{"--source-root", "s", "--project-id", "p", "--registry", reg("a.yaml")}, lockOf("a.yaml")},
		{"no registry: the one of the working directory", []string{"--source-root", "s"}, defaultLock},
		{"a registry given twice: the last", []string{"--registry", reg("a.yaml"), "--registry", reg("b.yaml")}, lockOf("b.yaml")},
		{"--registry as the last word names none", []string{"--source-root", "s", "--registry"}, defaultLock},
		{"a registry that was named and then --registry with no value: the one named", []string{"--registry", reg("a.yaml"), "--registry"}, lockOf("a.yaml")},
		{"the value of --source-root is its value, even when it is --registry", []string{"--source-root", "--registry", reg("a.yaml")}, defaultLock},
		{"the value of --manifest is its value, even when it is --registry", []string{"--manifest", "--registry", reg("a.yaml")}, defaultLock},
		{"a registry named before the wrapper's flags", []string{"--registry", reg("a.yaml"), "--manifest", "m", "--source-root", "s"}, lockOf("a.yaml")},
	} {
		for _, verb := range []string{"install", "adopt"} {
			t.Run(verb+" "+tc.name, func(t *testing.T) {
				if got := overlayLockTaken(t, verb, tc.args...); got != tc.want {
					t.Errorf("%s %q locked %q, want %q", verb, tc.args, got, tc.want)
				}
			})
		}
	}
	// The lock is shared: install and adopt only read the overlay.
	locker := &holdingLocker{}
	runSkillsVerb(skillsInstall, skills.Deps{Locker: locker, Cwd: func() (string, error) { return dir, nil }}, "install", "--registry", reg("a.yaml"))
	if got := locker.log(); len(got) == 0 || got[0] != "lock shared "+lockOf("a.yaml") {
		t.Errorf("lock events = %v, want the shared lock of the registry first", got)
	}
}

// The lock is keyed by the registry the verb then reads, whatever the command line is: both are
// read by one parser. The dispatcher locked the registry of a second hand-written scan, which did
// not know --project-id took a value, so it took `--project-id --registry r` for a flag and the
// registry r, and then read the default registry (the first flag was given the second as its
// value): the registry that was read was not the one that was locked.
func TestInstallAndAdoptLockTheRegistryTheyRead(t *testing.T) {
	dir := t.TempDir()
	r := filepath.Join(dir, "r.yaml")
	for _, verb := range []string{"install", "adopt"} {
		got := overlayLockTaken(t, verb, "--project-id", "--registry", r)
		if want := skills.RegistryLockPath("skills.registry.yaml"); got != want {
			t.Errorf("%s locked %q, want %q: the registry it reads, since --registry was the value of --project-id", verb, got, want)
		}
	}
}
