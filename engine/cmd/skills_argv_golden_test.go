package main

import (
	"flag"
	"strings"
	"testing"
)

// The golden files under testdata/skills-argv-golden record how the read-only verbs of `skills`
// read their command line (Phase 9, unit H20, which moves them behind one strict flag parser):
// 'list', 'status', 'validate' and 'lint'. They record what each form of the command line does
// today, the valid ones and the mistakes, so that a change to the way the arguments are read is
// a change that shows: the flags they take, a flag given twice, a flag with no value, a flag
// they do not know, a word that is no flag, the equals form, and the words of the refusals. The
// cases that H20 changes on purpose (an unknown flag is refused, decision D4) were recorded
// from the program before the change and were rewritten in that commit, each read in its diff.
//
// Each case runs the built program in a throwaway world, as the registry goldens do, and is its
// own subtest. Rewrite them deliberately with
//
//	go test ./cmd -run TestSkillsArgvGolden -update-skills-argv-golden
//
// and read the diff before committing it.
var updateSkillsArgvGolden = flag.Bool("update-skills-argv-golden", false, "rewrite the golden files of the skills command lines")

// TestSkillsArgvGolden runs every case and compares its transcript with its golden file.
func TestSkillsArgvGolden(t *testing.T) {
	for _, tc := range skillsArgvCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newRegistryWorld(t)
			tc.run(w)
			checkGoldenIn(t, "skills-argv-golden", tc.name, w.text(), updateSkillsArgvGolden, "-update-skills-argv-golden")
		})
	}
}

// otherRegistry is a registry of one skill, so that a run that is told to read it is told apart
// from a run that reads the registry of the world.
func (w *registryWorld) otherRegistry() string {
	return w.registryAt("other/skills.registry.yaml", registryOf("only-one"))
}

func skillsArgvCases() []registryGoldenCase {
	var cases []registryGoldenCase
	cases = append(cases, readOnlyRegistryVerbCases("list")...)
	cases = append(cases, readOnlyRegistryVerbCases("status")...)
	cases = append(cases, skillsValidateArgvCases()...)
	cases = append(cases, skillsLintArgvCases()...)
	return cases
}

// readOnlyRegistryVerbCases are the command lines of a verb that only reads the registry and says
// something of it: list and status take the same flags.
func readOnlyRegistryVerbCases(verb string) []registryGoldenCase {
	return []registryGoldenCase{
		{verb + "-argv-reads-the-registry-it-is-named", func(w *registryWorld) {
			w.overlay()
			other := w.otherRegistry()
			w.label("no flag: the registry of the working directory")
			w.run("skills", verb)
			w.label("--registry names another one")
			w.run("skills", verb, "--registry", other)
		}},
		{verb + "-argv-uses-the-last-of-a-repeated-registry-flag", func(w *registryWorld) {
			w.overlay()
			other := w.otherRegistry()
			w.label("the last --registry wins")
			w.run("skills", verb, "--registry", w.path(worldRegistry), "--registry", other)
			w.label("and so when the other comes first")
			w.run("skills", verb, "--registry", other, "--registry", w.path(worldRegistry))
		}},
		{verb + "-argv-takes-the-flags-of-the-wrapper-and-reads-no-more-of-them", func(w *registryWorld) {
			w.overlay()
			w.label("--manifest and --source-root come after every verb: they are not read, and are no mistake")
			w.run("skills", verb, "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
			w.label("and with a registry, the wrapper's order")
			w.run("skills", verb, "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
		}},
		{verb + "-argv-says-what-it-does-with-a-flag-it-does-not-know", func(w *registryWorld) {
			w.overlay()
			w.label("an unknown flag")
			w.run("skills", verb, "--frobnicate")
			w.label("an unknown flag with a value, and a registry")
			w.run("skills", verb, "--frobnicate", "yes", "--registry", w.path(worldRegistry))
			w.label("a short flag")
			w.run("skills", verb, "-v")
			w.label("the registry flag spelled with an equals sign")
			w.run("skills", verb, "--registry="+w.otherRegistry())
		}},
		{verb + "-argv-ignores-the-words-that-are-no-flag", func(w *registryWorld) {
			w.overlay()
			w.label("a word after the verb")
			w.run("skills", verb, "extra")
			w.label("two words")
			w.run("skills", verb, "extra", "more")
			w.label("a word after the registry")
			w.run("skills", verb, "--registry", w.path(worldRegistry), "extra")
		}},
		{verb + "-argv-with-a-flag-that-has-no-value", func(w *registryWorld) {
			w.overlay()
			w.label("--registry is the last word: no value, so the registry of the working directory")
			w.run("skills", verb, "--registry")
			w.label("--registry is followed by a flag: the flag is its value")
			w.run("skills", verb, "--registry", "--manifest", "x")
			w.label("--registry is given the empty word")
			w.run("skills", verb, "--registry", "")
		}},
		{verb + "-argv-finds-the-verb-after-the-flags", func(w *registryWorld) {
			w.overlay()
			other := w.otherRegistry()
			w.label("the flag first, then a word that is its value and is taken for the verb")
			w.run("skills", "--registry", other, verb)
			w.label("a flag that takes no part, then the verb")
			w.run("skills", "--frobnicate", verb)
			w.label("the verb, and a registry named as the verb is")
			w.run("skills", "--registry", verb)
		}},
		{verb + "-argv-with-a-missing-registry", func(w *registryWorld) {
			w.mkdir(".")
			w.label("no registry in the working directory")
			w.run("skills", verb)
			w.label("a registry that is not there")
			w.run("skills", verb, "--registry", w.path("absent.yaml"))
		}},
	}
}

func skillsValidateArgvCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"validate-argv-needs-the-source-root", func(w *registryWorld) {
			w.overlay()
			w.label("no flags")
			w.run("skills", "validate")
			w.label("the other two flags and no source root")
			w.run("skills", "validate", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest))
			w.label("a source root with no value, last")
			w.run("skills", "validate", "--source-root")
			w.label("a source root that is the empty word")
			w.run("skills", "validate", "--source-root", "")
		}},
		{"validate-argv-uses-the-last-of-a-repeated-flag", func(w *registryWorld) {
			w.overlay()
			other := w.otherRegistry()
			w.label("--registry twice")
			w.run("skills", "validate", "--registry", other, "--registry", w.path(worldRegistry), "--source-root", w.path("skills"))
			w.label("--manifest twice, the last is missing")
			w.run("skills", "validate", "--manifest", w.path(worldManifest), "--manifest", w.path("absent.manifest"), "--source-root", w.path("skills"))
			w.label("--source-root twice, the last is the one that is read")
			w.run("skills", "validate", "--source-root", w.path("nowhere"), "--source-root", w.path("skills"))
		}},
		{"validate-argv-says-what-it-does-with-a-flag-it-does-not-know", func(w *registryWorld) {
			w.overlay()
			src := w.path("skills")
			w.label("an unknown flag")
			w.run("skills", "validate", "--frobnicate", "--source-root", src)
			w.label("an unknown flag with a value")
			w.run("skills", "validate", "--source-root", src, "--frobnicate", "yes")
			w.label("a short flag")
			w.run("skills", "validate", "-x", "--source-root", src)
			w.label("the source root spelled with an equals sign")
			w.run("skills", "validate", "--source-root="+src)
		}},
		{"validate-argv-ignores-the-words-that-are-no-flag", func(w *registryWorld) {
			w.overlay()
			w.label("a word after the verb")
			w.run("skills", "validate", "extra", "--source-root", w.path("skills"))
			w.label("a word as the last argument")
			w.run("skills", "validate", "--source-root", w.path("skills"), "extra")
		}},
		{"validate-argv-with-a-manifest-that-is-not-there", func(w *registryWorld) {
			w.overlay()
			w.label("--manifest names a file that is not there")
			w.run("skills", "validate", "--manifest", w.path("absent.manifest"), "--source-root", w.path("skills"))
			w.label("--manifest is the last word: no value, so the manifest of the working directory")
			w.run("skills", "validate", "--source-root", w.path("skills"), "--manifest")
		}},
		{"validate-argv-finds-the-verb-after-the-flags", func(w *registryWorld) {
			w.overlay()
			w.label("the flags first, then the verb")
			w.run("skills", "--source-root", w.path("skills"), "validate")
			w.label("a flag that takes no part, then the verb")
			w.run("skills", "--frobnicate", "validate", "--source-root", w.path("skills"))
		}},
	}
}

func skillsLintArgvCases() []registryGoldenCase {
	const (
		good = "good/SKILL.md"
		bad  = "bad/SKILL.md"
	)
	lintWorld := func(w *registryWorld) {
		w.put(good, skillFile("good"))
		w.put(bad, strings.TrimPrefix(skillFile("bad"), "---\n"))
		w.put("-dash/SKILL.md", skillFile("dash"))
	}
	return []registryGoldenCase{
		{"lint-argv-prints-the-rules", func(w *registryWorld) {
			lintWorld(w)
			w.label("--rules alone")
			w.run("skills", "lint", "--rules")
			w.label("--rules with a path: the rules, the path is not read")
			w.run("skills", "lint", "--rules", w.path(good))
			w.label("--rules with a path that is not there")
			w.run("skills", "lint", "--rules", w.path("absent.md"))
			w.label("--rules given twice")
			w.run("skills", "lint", "--rules", "--rules")
		}},
		{"lint-argv-reads-the-path-it-is-given", func(w *registryWorld) {
			lintWorld(w)
			w.label("a skill that passes")
			w.run("skills", "lint", w.path(good))
			w.label("a skill that does not")
			w.run("skills", "lint", w.path(bad))
			w.label("the verb after the flag")
			w.run("skills", "--rules", "lint")
			w.label("the path before the verb is the verb")
			w.run("skills", w.path(good), "lint")
		}},
		{"lint-argv-needs-a-path-or-the-rules", func(w *registryWorld) {
			lintWorld(w)
			w.label("no path")
			w.run("skills", "lint")
			w.label("only the flags of the wrapper")
			w.run("skills", "lint", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
		}},
		{"lint-argv-takes-the-flags-of-the-wrapper-and-reads-no-more-of-them", func(w *registryWorld) {
			lintWorld(w)
			w.label("the wrapper appends --registry, --manifest and --source-root after the path")
			w.run("skills", "lint", w.path(good), "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
			w.label("and before it: a value is not taken for the path")
			w.run("skills", "lint", "--registry", w.path(worldRegistry), w.path(good))
			w.label("a flag of the wrapper with no value, last")
			w.run("skills", "lint", w.path(good), "--source-root")
			w.label("a flag of the wrapper that takes the path as its value: no path is left")
			w.run("skills", "lint", "--source-root", w.path(good))
		}},
		{"lint-argv-refuses-a-flag-it-does-not-know", func(w *registryWorld) {
			lintWorld(w)
			w.label("an unknown flag before the path")
			w.run("skills", "lint", "--frobnicate", w.path(good))
			w.label("an unknown flag after the path")
			w.run("skills", "lint", w.path(good), "--frobnicate")
			w.label("a short flag")
			w.run("skills", "lint", "-v", w.path(good))
			w.label("the equals form of a flag of the wrapper")
			w.run("skills", "lint", "--registry=x", w.path(good))
			w.label("a bad skill and an unknown flag: the flag is refused first")
			w.run("skills", "lint", w.path(bad), "--frobnicate")
			w.label("--rules and an unknown flag")
			w.run("skills", "lint", "--rules", "--frobnicate")
		}},
		{"lint-argv-refuses-a-second-path", func(w *registryWorld) {
			lintWorld(w)
			w.label("two paths: only one is accepted, neither is read")
			w.run("skills", "lint", w.path(good), w.path(bad))
			w.label("a second path after the flags of the wrapper")
			w.run("skills", "lint", w.path(good), "--source-root", w.path("skills"), w.path(bad))
			w.label("--rules and two paths: the second is refused before the rules are printed")
			w.run("skills", "lint", "--rules", w.path(good), w.path(bad))
		}},
		{"lint-argv-ends-the-options-at-a-double-dash", func(w *registryWorld) {
			lintWorld(w)
			w.label("a path that begins with a dash, after --")
			w.run("skills", "lint", "--", "-dash/SKILL.md")
			w.label("an unknown flag after --: it is the path")
			w.run("skills", "lint", "--", "--frobnicate")
			w.label("-- and two paths")
			w.run("skills", "lint", "--", w.path(good), w.path(bad))
			w.label("-- and nothing after it")
			w.run("skills", "lint", "--")
			w.label("--rules after --: it is a path")
			w.run("skills", "lint", "--", "--rules")
			w.label("-- twice: the second is a path")
			w.run("skills", "lint", "--", "--", w.path(good))
		}},
	}
}
