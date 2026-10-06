package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden files under testdata/skills-writers-argv-golden record how the verbs of `skills` that
// write read their command line (Phase 9, unit H20, which moves them behind use cases and the one
// strict flag parser): the writers of an overlay ('add', 'remove', 'sync-manifest', 'approve'),
// 'install' and 'adopt', and the four verbs of a project. They record what each form of the
// command line does, the valid ones and the mistakes, so that a change to the way the arguments
// are read is a change that shows: the flags each takes, a flag given twice, a flag with no value,
// a flag it does not know, a word that is no flag, the equals form, "--", and the words of every
// refusal. The cases a verb's move changes on purpose (an unknown flag is refused, decision D4)
// were recorded from the program before the move and are rewritten in the commit that moves the
// verb, each read in its diff.
//
// Each case runs the built program in a throwaway world, as the registry goldens do. A form that
// writes is followed by the restoration of the world, so that the next form starts from the same
// files. Rewrite the files deliberately with
//
//	go test ./cmd -run TestSkillsWritersArgvGolden -update-skills-writers-argv-golden
//
// and read the diff before committing it.
var updateSkillsWritersArgvGolden = flag.Bool("update-skills-writers-argv-golden", false, "rewrite the golden files of the command lines of the writers of skills")

// TestSkillsWritersArgvGolden runs every case and compares its transcript with its golden file.
func TestSkillsWritersArgvGolden(t *testing.T) {
	for _, tc := range skillsWritersArgvCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newRegistryWorld(t)
			tc.run(w)
			checkGoldenIn(t, "skills-writers-argv-golden", tc.name, w.text(), updateSkillsWritersArgvGolden, "-update-skills-writers-argv-golden")
		})
	}
}

// Two cases of one name would share a golden file, and a file no case owns is a recording nothing
// checks.
func TestSkillsWritersArgvGoldenCasesAreDistinctFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range skillsWritersArgvCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "skills-writers-argv-golden"))
	if err != nil {
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/skills-writers-argv-golden/%s belongs to no case", e.Name())
		}
	}
	if len(entries) != len(seen) {
		t.Errorf("%d golden files for %d cases", len(entries), len(seen))
	}
}

// try records one invocation under a line that says what it shows.
func (w *registryWorld) try(what string, args ...string) {
	w.t.Helper()
	w.label("%s", what)
	w.run(args...)
}

// tryIn records one invocation started in the directory cwd of the world.
func (w *registryWorld) tryIn(cwd, what string, args ...string) {
	w.t.Helper()
	w.label("%s", what)
	w.runIn(cwd, args...)
}

// writersWorld is the overlay of the writers' cases: a registry and a manifest that agree on one
// skill, alpha, and the source of alpha and of foo, a second skill that 'add' can register.
func (w *registryWorld) writersWorld() {
	w.t.Helper()
	w.addWorld()
	w.putSkill("bar", false)
}

// restoreOverlay puts the registry and the manifest back as writersWorld made them.
func (w *registryWorld) restoreOverlay(manifest string) {
	w.t.Helper()
	w.put(worldRegistry, registryOf("alpha"))
	w.put(worldManifest, manifest)
}

func skillsWritersArgvCases() []registryGoldenCase {
	const alphaManifest = "alpha/SKILL.md custom\n"
	const driftedManifest = "ghost/SKILL.md custom\n"
	const repo = "https://example.test/foo.git"
	return []registryGoldenCase{
		// ---- add --------------------------------------------------------------------------
		{"add-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.writersWorld()
			w.try("no flags: the registry, the manifest and the source root of the working directory", "skills", "add", "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("every flag, before the id", "skills", "add", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"), "foo")
			w.restoreOverlay(alphaManifest)
			w.try("the id first, the flags after it", "skills", "add", "foo", "--source-root", w.path("skills"), "--registry", w.path(worldRegistry))
			w.restoreOverlay(alphaManifest)
			w.try("--repo and --ref make the entry external", "skills", "add", "--repo", repo, "--ref", "v1", "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("--repo without --ref", "skills", "add", "--repo", repo, "foo")
			w.show(worldRegistry)
		}},
		{"add-argv-reads-the-last-of-a-repeated-flag", func(w *registryWorld) {
			w.writersWorld()
			w.try("--source-root twice: the second is the one read", "skills", "add", "--source-root", w.path("absent"), "--source-root", w.path("skills"), "foo")
			w.restoreOverlay(alphaManifest)
			w.try("--registry twice: the second is the one written", "skills", "add", "--registry", w.path("absent.yaml"), "--registry", w.path(worldRegistry), "foo")
			w.restoreOverlay(alphaManifest)
			w.try("--repo twice", "skills", "add", "--repo", "https://example.test/first.git", "--repo", repo, "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("the first --source-root is the one read: it holds no skill", "skills", "add", "--source-root", w.path("skills"), "--source-root", w.path("absent"), "foo")
		}},
		{"add-argv-says-what-it-does-with-a-flag-that-has-no-value", func(w *registryWorld) {
			w.writersWorld()
			w.try("--repo as the last word: it has no value and the entry is custom", "skills", "add", "foo", "--repo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("--source-root as the last word: the default is read", "skills", "add", "foo", "--source-root")
			w.restoreOverlay(alphaManifest)
			w.try("--repo before the id: the id is its value", "skills", "add", "--repo", "foo")
			w.try("--ref with no --repo", "skills", "add", "--ref", "v1", "foo")
			w.try("--registry before the id: the id is its value", "skills", "add", "--registry", "foo")
			w.try("--ref as the last word, with a --repo", "skills", "add", "--repo", repo, "foo", "--ref")
			w.show(worldRegistry)
		}},
		{"add-argv-says-what-it-does-with-a-flag-it-does-not-know", func(w *registryWorld) {
			w.writersWorld()
			w.try("an unknown flag before the id", "skills", "add", "--frobnicate", "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("an unknown flag after the id", "skills", "add", "foo", "--frobnicate")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("a short flag before the id", "skills", "add", "-v", "foo")
			w.try("a short flag after the id", "skills", "add", "foo", "-v")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("the equals form of a flag it reads", "skills", "add", "--registry="+w.path(worldRegistry), "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("a flag of install", "skills", "add", "--project-id", "p", "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("an unknown flag and no id", "skills", "add", "--frobnicate")
		}},
		{"add-argv-reads-the-first-word-as-the-id", func(w *registryWorld) {
			w.writersWorld()
			w.try("a second word", "skills", "add", "foo", "bar")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("the other word first", "skills", "add", "bar", "foo")
			w.try("an empty word before the id", "skills", "add", "", "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("-- before the id", "skills", "add", "--", "foo")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("an id that is no slug", "skills", "add", "Foo")
			w.try("an id that is registered", "skills", "add", "alpha")
		}},
		{"add-argv-needs-an-id", func(w *registryWorld) {
			w.writersWorld()
			w.try("nothing", "skills", "add")
			w.try("the flags of the wrapper and no id", "skills", "add", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
			w.try("an empty id", "skills", "add", "")
			w.show(worldRegistry)
		}},

		// ---- remove -----------------------------------------------------------------------
		{"remove-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.writersWorld()
			w.try("no flags", "skills", "remove", "alpha")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("every flag of the wrapper, before the id", "skills", "remove", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"), "alpha")
			w.restoreOverlay(alphaManifest)
			w.try("the id first, the flags after it", "skills", "remove", "alpha", "--manifest", w.path(worldManifest), "--registry", w.path(worldRegistry))
			w.restoreOverlay(alphaManifest)
			w.try("the flags of add, which remove does not read", "skills", "remove", "--repo", repo, "--ref", "v1", "alpha")
			w.show(worldRegistry)
		}},
		{"remove-argv-reads-the-last-of-a-repeated-flag-and-leaves-a-valueless-one-unset", func(w *registryWorld) {
			w.writersWorld()
			w.try("--registry twice", "skills", "remove", "--registry", w.path("absent.yaml"), "--registry", w.path(worldRegistry), "alpha")
			w.restoreOverlay(alphaManifest)
			w.try("--manifest twice", "skills", "remove", "--manifest", w.path("absent.manifest"), "--manifest", w.path(worldManifest), "alpha")
			w.restoreOverlay(alphaManifest)
			w.try("--registry as the last word: the default is read", "skills", "remove", "alpha", "--registry")
			w.restoreOverlay(alphaManifest)
			w.try("--registry before the id: the id is its value", "skills", "remove", "--registry", "alpha")
		}},
		{"remove-argv-says-what-it-does-with-a-flag-it-does-not-know", func(w *registryWorld) {
			w.writersWorld()
			w.try("an unknown flag before the id", "skills", "remove", "--frobnicate", "alpha")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("an unknown flag after the id", "skills", "remove", "alpha", "--frobnicate")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("a short flag before the id", "skills", "remove", "-v", "alpha")
			w.try("the equals form of a flag it reads", "skills", "remove", "--registry="+w.path(worldRegistry), "alpha")
			w.show(worldRegistry)
		}},
		{"remove-argv-reads-the-first-word-as-the-id", func(w *registryWorld) {
			w.writersWorld()
			w.try("nothing", "skills", "remove")
			w.try("a second word", "skills", "remove", "alpha", "beta")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("the other word first", "skills", "remove", "beta", "alpha")
			w.try("-- before the id", "skills", "remove", "--", "alpha")
			w.show(worldRegistry)
			w.restoreOverlay(alphaManifest)
			w.try("an empty id", "skills", "remove", "")
		}},

		// ---- sync-manifest ----------------------------------------------------------------
		{"sync-manifest-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.writersWorld()
			w.restoreOverlay(driftedManifest)
			w.try("no flags", "skills", "sync-manifest")
			w.show(worldManifest)
			w.restoreOverlay(driftedManifest)
			w.try("every flag of the wrapper", "skills", "sync-manifest", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
			w.restoreOverlay(driftedManifest)
			w.try("--repo and --ref, which it does not read", "skills", "sync-manifest", "--repo", repo, "--ref", "v1")
			w.show(worldManifest)
		}},
		{"sync-manifest-argv-reads-the-last-of-a-repeated-flag-and-leaves-a-valueless-one-unset", func(w *registryWorld) {
			w.writersWorld()
			w.restoreOverlay(driftedManifest)
			w.try("--manifest twice", "skills", "sync-manifest", "--manifest", w.path("absent.manifest"), "--manifest", w.path(worldManifest))
			w.show(worldManifest)
			w.restoreOverlay(driftedManifest)
			w.try("--manifest as the last word: the default is read", "skills", "sync-manifest", "--manifest")
			w.show(worldManifest)
			w.restoreOverlay(driftedManifest)
			w.try("--registry twice", "skills", "sync-manifest", "--registry", w.path("absent.yaml"), "--registry", w.path(worldRegistry))
		}},
		{"sync-manifest-argv-says-what-it-does-with-a-flag-it-does-not-know", func(w *registryWorld) {
			w.writersWorld()
			w.restoreOverlay(driftedManifest)
			w.try("an unknown flag", "skills", "sync-manifest", "--frobnicate")
			w.show(worldManifest)
			w.restoreOverlay(driftedManifest)
			w.try("a short flag", "skills", "sync-manifest", "-v")
			w.show(worldManifest)
			w.restoreOverlay(driftedManifest)
			w.try("the equals form of a flag it reads", "skills", "sync-manifest", "--manifest="+w.path(worldManifest))
			w.show(worldManifest)
		}},
		{"sync-manifest-argv-reads-no-word", func(w *registryWorld) {
			w.writersWorld()
			w.restoreOverlay(driftedManifest)
			w.try("a word that is no flag", "skills", "sync-manifest", "alpha")
			w.show(worldManifest)
			w.restoreOverlay(driftedManifest)
			w.try("-- and a word", "skills", "sync-manifest", "--", "alpha")
			w.show(worldManifest)
		}},

		// ---- approve ----------------------------------------------------------------------
		{"approve-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.writersWorld()
			ok := []string{"skills", "approve", "--id", "bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills")}
			w.try("the flags it reads", ok...)
			w.remove("skills/bar/.approval.json")
			w.try("the flags in another order", "skills", "approve", "--source-root", w.path("skills"), "--approver", "fixture-reviewer", "--id", "bar")
			w.remove("skills/bar/.approval.json")
			w.try("the flags of the wrapper, which it takes and ignores", append(ok, "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest))...)
			w.remove("skills/bar/.approval.json")
			w.try("an approver label that is a sentence", "skills", "approve", "--id", "bar", "--approver", "A Reviewer", "--source-root", w.path("skills"))
		}},
		{"approve-argv-reads-the-last-of-a-repeated-flag", func(w *registryWorld) {
			w.writersWorld()
			w.try("--id twice: the second is the one approved", "skills", "approve", "--id", "absent", "--id", "bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills"))
			w.remove("skills/bar/.approval.json")
			w.try("--approver twice", "skills", "approve", "--id", "bar", "--approver", "first", "--approver", "second", "--source-root", w.path("skills"))
			w.remove("skills/bar/.approval.json")
			w.try("--source-root twice", "skills", "approve", "--id", "bar", "--approver", "fixture-reviewer", "--source-root", w.path("absent"), "--source-root", w.path("skills"))
		}},
		{"approve-argv-refuses-a-flag-with-no-value-or-a-value-that-is-a-flag", func(w *registryWorld) {
			w.writersWorld()
			w.try("--id as the last word", "skills", "approve", "--approver", "fixture-reviewer", "--source-root", w.path("skills"), "--id")
			w.try("--approver as the last word", "skills", "approve", "--id", "bar", "--source-root", w.path("skills"), "--approver")
			w.try("--source-root as the last word", "skills", "approve", "--id", "bar", "--approver", "fixture-reviewer", "--source-root")
			w.try("--id followed by a flag", "skills", "approve", "--id", "--approver", "fixture-reviewer", "--source-root", w.path("skills"))
			w.try("an approver label that begins with a dash", "skills", "approve", "--id", "bar", "--approver", "-reviewer", "--source-root", w.path("skills"))
			w.try("--registry as the last word", "skills", "approve", "--id", "bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills"), "--registry")
			w.try("--manifest followed by a flag", "skills", "approve", "--id", "bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills"), "--manifest", "--registry")
		}},
		{"approve-argv-refuses-a-flag-it-does-not-know-and-a-word", func(w *registryWorld) {
			w.writersWorld()
			ok := []string{"skills", "approve", "--id", "bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills")}
			w.try("an unknown flag", append(ok, "--frobnicate")...)
			w.try("a short flag", append(ok, "-v")...)
			w.try("the equals form of a flag it reads", "skills", "approve", "--id=bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills"))
			w.try("a word that is no flag", append(ok, "bar")...)
			w.try("-- is an unknown flag", append(ok, "--")...)
			w.try("a flag of add", append(ok, "--repo", repo)...)
		}},
		{"approve-argv-needs-its-three-flags", func(w *registryWorld) {
			w.writersWorld()
			w.try("nothing", "skills", "approve")
			w.try("no approver", "skills", "approve", "--id", "bar", "--source-root", w.path("skills"))
			w.try("an empty approver", "skills", "approve", "--id", "bar", "--approver", "", "--source-root", w.path("skills"))
			w.try("no id", "skills", "approve", "--approver", "fixture-reviewer", "--source-root", w.path("skills"))
			w.try("no source root", "skills", "approve", "--id", "bar", "--approver", "fixture-reviewer")
			w.try("an id that is no slug", "skills", "approve", "--id", "Bar", "--approver", "fixture-reviewer", "--source-root", w.path("skills"))
			w.try("an approver of two lines", "skills", "approve", "--id", "bar", "--approver", "one\ntwo", "--source-root", w.path("skills"))
		}},

		// ---- install and adopt ------------------------------------------------------------
		{"install-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.projectWorld()
			args := w.installArgs("install")
			w.tryIn("demo", "the registry and the source root", args...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "the flags in another order", "skills", "install", "--source-root", w.path("overlay/skills"), "--registry", w.path("overlay/"+worldRegistry))
			w.resetDemo()
			w.tryIn("demo", "--project-id", append(args, "--project-id", "other")...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "the flags of the wrapper, which are taken and not read", append(args, "--manifest", w.path("overlay/overlay.manifest"))...)
			w.resetDemo()
			w.tryIn("demo", "no registry given: the one of the working directory", "skills", "install", "--source-root", w.path("overlay/skills"))
		}},
		{"install-argv-reads-the-last-of-a-repeated-flag-and-leaves-a-valueless-one-unset", func(w *registryWorld) {
			w.projectWorld()
			args := w.installArgs("install")
			w.tryIn("demo", "--registry twice", append([]string{"skills", "install", "--registry", w.path("absent.yaml")}, args[2:]...)...)
			w.resetDemo()
			w.tryIn("demo", "--project-id twice", append(args, "--project-id", "stranger", "--project-id", "other")...)
			w.resetDemo()
			w.tryIn("demo", "--source-root twice", append(args, "--source-root", w.path("absent"))...)
			w.tryIn("demo", "--project-id as the last word: the project is named by the directory", append(args, "--project-id")...)
			w.resetDemo()
			w.tryIn("demo", "--project-id before another flag: that flag is its value", "skills", "install", "--project-id", "--registry", w.path("overlay/"+worldRegistry), "--source-root", w.path("overlay/skills"))
			w.tryIn("demo", "--source-root as the last word", "skills", "install", "--registry", w.path("overlay/"+worldRegistry), "--source-root")
		}},
		{"install-argv-says-what-it-does-with-a-flag-it-does-not-know", func(w *registryWorld) {
			w.projectWorld()
			args := w.installArgs("install")
			w.tryIn("demo", "an unknown flag", append(args, "--frobnicate")...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "an unknown flag before the others", append([]string{"skills", "install", "--frobnicate"}, args[2:]...)...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "a short flag", append(args, "-v")...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "the equals form of a flag it reads", append(args, "--project-id=other")...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "a flag of add", append(args, "--repo", repo)...)
			w.tree("demo")
		}},
		{"install-argv-reads-no-word", func(w *registryWorld) {
			w.projectWorld()
			args := w.installArgs("install")
			w.tryIn("demo", "a word that is no flag", append(args, "extra")...)
			w.tree("demo")
			w.resetDemo()
			w.tryIn("demo", "-- and a word", append(args, "--", "extra")...)
			w.tree("demo")
		}},
		{"adopt-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.projectWorld()
			args := w.installArgs("adopt")
			w.tryIn("demo", "nothing installed to adopt", args...)
			w.tryIn("demo", "--project-id", append(args, "--project-id", "other")...)
			w.tryIn("demo", "the flags in another order", "skills", "adopt", "--source-root", w.path("overlay/skills"), "--registry", w.path("overlay/"+worldRegistry))
			w.tryIn("demo", "the flags of the wrapper, which are taken and not read", append(args, "--manifest", w.path("overlay/overlay.manifest"))...)
			w.tryIn("demo", "--project-id twice", append(args, "--project-id", "stranger", "--project-id", "other")...)
			w.tryIn("demo", "--project-id as the last word", append(args, "--project-id")...)
			w.tryIn("demo", "--registry as the last word: the default is read", "skills", "adopt", "--source-root", w.path("overlay/skills"), "--registry")
		}},
		{"adopt-argv-says-what-it-does-with-a-flag-it-does-not-know-and-a-word", func(w *registryWorld) {
			w.projectWorld()
			args := w.installArgs("adopt")
			w.tryIn("demo", "an unknown flag", append(args, "--frobnicate")...)
			w.tryIn("demo", "a short flag", append(args, "-v")...)
			w.tryIn("demo", "the equals form of a flag it reads", append(args, "--project-id=other")...)
			w.tryIn("demo", "a word that is no flag", append(args, "extra")...)
			w.tryIn("demo", "-- and a word", append(args, "--", "extra")...)
			w.tree("demo")
		}},

		// ---- the four verbs of a project --------------------------------------------------
		{"project-register-argv-takes-the-flags-it-reads", func(w *registryWorld) {
			w.projectRegisterWorld()
			root, reg, draft := w.path("project"), w.path("overlay/"+worldRegistry), w.path("drafts/tidy/SKILL.md")
			w.try("--dry-run, the flags in another order", "skills", "project-register", "--dry-run", "--registry", reg, "--candidate", goldenCandidate, "--project-root", root, draft)
			w.try("the flags of the wrapper, which are taken and not read", "skills", "project-register", "--dry-run", "--project-root", root, "--candidate", goldenCandidate, "--registry", reg, "--manifest", w.path("overlay.manifest"), "--source-root", w.path("skills"), draft)
			w.try("the draft first", "skills", "project-register", draft, "--dry-run", "--project-root", root, "--candidate", goldenCandidate, "--registry", reg)
			w.try("--project-root twice: the second is the one read", "skills", "project-register", "--dry-run", "--project-root", w.path("absent"), "--project-root", root, "--candidate", goldenCandidate, "--registry", reg, draft)
			w.try("--candidate twice", "skills", "project-register", "--dry-run", "--project-root", root, "--candidate", "first", "--candidate", goldenCandidate, "--registry", reg, draft)
			w.try("--dry-run twice", "skills", "project-register", "--dry-run", "--dry-run", "--project-root", root, "--candidate", goldenCandidate, "--registry", reg, draft)
			w.try("-- ends the options", "skills", "project-register", "--dry-run", "--project-root", root, "--candidate", goldenCandidate, "--registry", reg, "--", draft)
			w.tree("project")
		}},
		{"project-register-argv-refuses-what-it-cannot-read", func(w *registryWorld) {
			w.projectRegisterWorld()
			root, reg, draft := w.path("project"), w.path("overlay/"+worldRegistry), w.path("drafts/tidy/SKILL.md")
			base := []string{"skills", "project-register", "--project-root", root, "--candidate", goldenCandidate, "--registry", reg}
			w.try("an unknown flag", append(append([]string{}, base...), "--frobnicate", draft)...)
			w.try("a short flag", append(append([]string{}, base...), "-v", draft)...)
			w.try("a misspelled --dry-run", append(append([]string{}, base...), "--dryrun", draft)...)
			w.try("the equals form", append(append([]string{}, base...), "--dry-run=true", draft)...)
			w.try("a flag of retire", append(append([]string{}, base...), "--reason", "x", draft)...)
			w.try("--project-root as the last word", "skills", "project-register", "--candidate", goldenCandidate, "--registry", reg, draft, "--project-root")
			w.try("--candidate followed by a flag", "skills", "project-register", "--project-root", root, "--candidate", "--registry", reg, draft)
			w.try("--registry as the last word", append(append([]string{}, base[:len(base)-2]...), draft, "--registry")...)
			w.try("a second draft", append(append([]string{}, base...), draft, draft)...)
			w.try("no draft", base...)
			w.try("no project root", "skills", "project-register", "--candidate", goldenCandidate, "--registry", reg, draft)
			w.try("no candidate", "skills", "project-register", "--project-root", root, "--registry", reg, draft)
			w.try("-- and a flag as the draft", append(append([]string{}, base...), "--", "--dry-run")...)
			w.tree("project")
		}},
		{"project-revise-argv-takes-the-flags-it-reads-and-refuses-the-rest", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.try("the draft is registered, so that the verbs find a skill to read", w.registerArgs()...)
			root, reg, draft := w.path("project"), w.path("overlay/"+worldRegistry), w.path("drafts/tidy/SKILL.md")
			base := []string{"skills", "project-revise", "--project-root", root, "--candidate", goldenCandidate, "--registry", reg}
			w.try("--dry-run", append(append([]string{"skills", "project-revise", "--dry-run"}, base[2:]...), draft)...)
			w.try("the flags of the wrapper", append(append([]string{}, base...), "--manifest", w.path("overlay.manifest"), "--source-root", w.path("skills"), draft)...)
			w.try("an unknown flag", append(append([]string{}, base...), "--frobnicate", draft)...)
			w.try("a short flag", append(append([]string{}, base...), "-v", draft)...)
			w.try("a flag of retire", append(append([]string{}, base...), "--reason", "x", draft)...)
			w.try("--candidate as the last word", "skills", "project-revise", "--project-root", root, "--registry", reg, draft, "--candidate")
			w.try("--project-root followed by a flag", "skills", "project-revise", "--project-root", "--candidate", goldenCandidate, "--registry", reg, draft)
			w.try("a second draft", append(append([]string{}, base...), draft, draft)...)
			w.try("no draft", base...)
			w.try("no project root", "skills", "project-revise", "--candidate", goldenCandidate, "--registry", reg, draft)
			w.try("-- and a draft", append(append([]string{}, base...), "--", draft)...)
			w.tree("project")
		}},
		{"project-status-argv-takes-the-flags-it-reads-and-refuses-the-rest", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.try("the draft is registered, so that the verbs find a skill to read", w.registerArgs()...)
			root, reg := w.path("project"), w.path("overlay/"+worldRegistry)
			w.try("the flags it reads", "skills", "project-status", "--project-root", root, "--registry", reg)
			w.try("the flags in another order", "skills", "project-status", "--registry", reg, "--project-root", root)
			w.try("the flags of the wrapper", "skills", "project-status", "--project-root", root, "--registry", reg, "--manifest", w.path("overlay.manifest"), "--source-root", w.path("skills"))
			w.try("--project-root twice", "skills", "project-status", "--project-root", w.path("absent"), "--project-root", root, "--registry", reg)
			w.try("no registry given: the default is read", "skills", "project-status", "--project-root", root)
			w.try("an unknown flag", "skills", "project-status", "--project-root", root, "--registry", reg, "--frobnicate")
			w.try("a short flag", "skills", "project-status", "-v", "--project-root", root, "--registry", reg)
			w.try("a flag of register", "skills", "project-status", "--project-root", root, "--registry", reg, "--dry-run")
			w.try("a word that is no flag", "skills", "project-status", "--project-root", root, "--registry", reg, "tidy-worktree")
			w.try("--project-root as the last word", "skills", "project-status", "--registry", reg, "--project-root")
			w.try("--registry followed by a flag", "skills", "project-status", "--project-root", root, "--registry", "--project-root")
			w.try("no project root", "skills", "project-status", "--registry", reg)
			w.try("-- and a word", "skills", "project-status", "--project-root", root, "--registry", reg, "--", "tidy-worktree")
		}},
		{"project-retire-argv-takes-the-flags-it-reads-and-refuses-the-rest", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.try("the draft is registered, so that the verbs find a skill to read", w.registerArgs()...)
			root, reg := w.path("project"), w.path("overlay/"+worldRegistry)
			base := []string{"skills", "project-retire", "--project-root", root, "--registry", reg}
			w.try("--dry-run of a skill that is not there", append(append([]string{}, base...), "--dry-run", "--reason", "not needed", "tidy-worktree")...)
			w.try("--absorbed-into", append(append([]string{}, base...), "--dry-run", "--reason", "not needed", "--absorbed-into", "other", "tidy-worktree")...)
			w.try("--reason twice", append(append([]string{}, base...), "--dry-run", "--reason", "first", "--reason", "second", "tidy-worktree")...)
			w.try("the flags of the wrapper", append(append([]string{}, base...), "--dry-run", "--reason", "x", "--manifest", w.path("overlay.manifest"), "--source-root", w.path("skills"), "tidy-worktree")...)
			w.try("an unknown flag", append(append([]string{}, base...), "--reason", "x", "--frobnicate", "tidy-worktree")...)
			w.try("a short flag", append(append([]string{}, base...), "--reason", "x", "-v", "tidy-worktree")...)
			w.try("a flag of register", append(append([]string{}, base...), "--reason", "x", "--candidate", "k", "tidy-worktree")...)
			w.try("--reason as the last word", append(append([]string{}, base...), "tidy-worktree", "--reason")...)
			w.try("--reason followed by a flag", append(append([]string{}, base...), "--reason", "--dry-run", "tidy-worktree")...)
			w.try("a reason that begins with a dash", append(append([]string{}, base...), "--reason", "-x", "tidy-worktree")...)
			w.try("--absorbed-into followed by a flag", append(append([]string{}, base...), "--reason", "x", "--absorbed-into", "--dry-run", "tidy-worktree")...)
			w.try("a second id", append(append([]string{}, base...), "--reason", "x", "tidy-worktree", "other")...)
			w.try("no id", append(append([]string{}, base...), "--reason", "x")...)
			w.try("no reason", append(append([]string{}, base...), "--dry-run", "tidy-worktree")...)
			w.try("-- and an id", append(append([]string{}, base...), "--reason", "x", "--", "tidy-worktree")...)
			w.tree("project")
		}},
	}
}
