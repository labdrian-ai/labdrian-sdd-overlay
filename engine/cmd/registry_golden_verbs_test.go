package main

import (
	"strings"
	"testing"
)

// The cases of the registry goldens that are about the verbs that work with a registry and a
// manifest: 'validate', which cross-checks them and the skills on disk, 'add' and 'remove',
// which write both, and 'sync-manifest', which regenerates the manifest from the registry. Each
// is shown over a registry it can read, and over every state of a registry it refuses.

// registryStates are the four states of a registry that every verb has to meet, and the registry
// files that are them: one that is not there, one with a key the reader does not know, one that
// names a version it does not know, and one with a value outside its vocabulary (which only a
// check of the entry finds). Three of them are refusals. The second is not since H16 (decision Q5):
// the reader leaves out a key it does not know and says so, the verbs that read go on with the
// rest, and the verbs that write refuse; before it, it was the fourth refusal, and the golden
// files of the verbs below (named for the refusals) record that difference in place.
func registryStates() []registryDoc {
	return []registryDoc{
		{"a registry that is not there", ""},
		{"a registry with a key the reader does not know", changed("      type: custom", "      type: custom\n      mirror: x")},
		{"a registry of a version it does not know", strings.Replace(baseEntry, `"1"`, `"3"`, 1)},
		{"a registry with a value outside its vocabulary", changed("- claude", "- vim")},
	}
}

// isAbsent reports whether the document is the registry that is not there: it has no text, so no
// file is written for it, and the verb is given the path where it would have been.
func (d registryDoc) isAbsent() bool { return d.text == "" }

// registryStateFile is one state of a registry as a verb is given it: what the transcript calls it
// and the path of its file in the world, which holds nothing when the state is the absent one.
type registryStateFile struct{ label, path string }

// putRegistryStates writes each state of a registry that has a file to refused/<prefix>a.yaml,
// <prefix>b.yaml and so on, in the order of registryStates, and returns them with their paths. (The
// files are put under refused/, the name they had when all four were refusals, so that the paths in
// the golden files stay.) It is the one place that tells the absent state from the others, by
// whether it has a file and not by its place in the list.
func (w *registryWorld) putRegistryStates(prefix string) []registryStateFile {
	w.t.Helper()
	var files []registryStateFile
	for i, doc := range registryStates() {
		rel := "refused/" + prefix + string(rune('a'+i)) + ".yaml"
		if !doc.isAbsent() {
			w.put(rel, doc.text)
		}
		files = append(files, registryStateFile{label: doc.label, path: w.path(rel)})
	}
	return files
}

// eachRegistryState records the run of a verb, which takes the registry path as the argument
// --registry, over each state of a registry; the first is a path where there is no file. args is
// called with the path of the registry to build the arguments.
func (w *registryWorld) eachRegistryState(args func(registry string) []string) {
	w.t.Helper()
	for _, file := range w.putRegistryStates("") {
		w.label("%s", file.label)
		w.run(args(file.path)...)
	}
}

// The states are told apart by whether they have a file, not by their place in the list: exactly
// one of them is the registry that is not there.
func TestExactlyOneRegistryStateHasNoFile(t *testing.T) {
	absent := 0
	for _, doc := range registryStates() {
		if doc.isAbsent() {
			absent++
		}
	}
	if absent != 1 {
		t.Errorf("%d of the states of a registry have no file, want 1 (the registry that is not there)", absent)
	}
}

func registryVerbCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"validate-says-an-overlay-is-aligned", func(w *registryWorld) {
			w.overlay()
			w.label("the registry and the manifest agree, every file is a row, every global skill is approved")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.label("the paths named")
			w.run("skills", "validate", "--registry", w.path(worldRegistry), "--manifest", w.path(worldManifest), "--source-root", w.path("skills"))
			w.label("the source root is required")
			w.run("skills", "validate")
		}},
		{"validate-names-every-divergence-of-the-registry-and-the-manifest", func(w *registryWorld) {
			w.overlay()
			// The divergences that start from a manifest directory are listed in the order of a
			// map, so each of those has a run of its own: a run holds at most one of them.
			w.put(worldManifest, "engine/go.mod managed\nbeta/SKILL.md custom\next-one/SKILL.md custom\ntidy-notes/SKILL.md custom\n")
			w.label("a skill with no row (alpha), and a row with the wrong tag (beta): in the order of the registry")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.put(worldManifest, goldenManifest+"ghost/SKILL.md custom\n")
			w.label("a row with no skill")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.put(worldManifest, goldenManifest+"ext-one/SKILL.md managed\n")
			w.label("a skill with both tags")
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"validate-names-every-divergence-of-the-manifest-and-the-disk", func(w *registryWorld) {
			w.overlay()
			w.put("skills/alpha/extra.md", "not a row\n")
			w.put(worldManifest, goldenManifest+"beta/references/missing.md managed\n")
			w.label("a file with no row, and a row with no file")
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"validate-names-the-global-skills-that-are-not-approved", func(w *registryWorld) {
			w.overlay()
			w.putSkill("alpha", false)
			w.putSkillBytes("beta", skillFile("beta")+"\nan edit after the approval\n", false)
			w.label("a skill with no record, and a skill whose record is for other bytes")
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"validate-says-what-it-cannot-read", func(w *registryWorld) {
			w.overlay()
			w.label("a manifest that is not there")
			w.run("skills", "validate", "--manifest", w.path("none.manifest"), "--source-root", w.path("skills"))
			w.label("a source root that is not there")
			w.run("skills", "validate", "--source-root", w.path("no-skills"))
			w.label("a source root that is a file")
			w.run("skills", "validate", "--source-root", w.path(worldManifest))
			w.mkdir("empty-skills")
			w.label("a source root with nothing in it")
			w.run("skills", "validate", "--source-root", w.path("empty-skills"))
		}},
		{"validate-refuses-an-unusable-registry", func(w *registryWorld) {
			w.overlay()
			// The registries of the states have one entry (alpha), so the manifest has its row and
			// no other: a manifest that has rows the registry has not would list those in the order
			// of a map, which a golden file cannot hold (see the case above that has one at a time).
			w.put(worldManifest, "engine/go.mod managed\nalpha/SKILL.md custom\n")
			w.eachRegistryState(func(registry string) []string {
				return []string{"skills", "validate", "--registry", registry, "--source-root", w.path("skills")}
			})
		}},
		{"validate-fails-on-a-registry-the-reader-left-fields-out-of", func(w *registryWorld) {
			// Decision 3 of the owner: validate is the verb a CI uses to detect drift, so a registry
			// that agrees with its manifest and its disk is still not passed when the reader left
			// something of it out. The verbs that only read keep warning and going on.
			w.overlay()
			w.label("a registry that agrees with everything, read whole")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.put(worldRegistry, "extra: 1\n"+goldenRegistryYAML)
			w.label("the same, with a key at its root that the reader does not know")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.label("and list, which only reads, warns and goes on")
			w.run("skills", "list")
			w.put(worldRegistry, strings.Replace(goldenRegistryYAML, "      updateStrategy: overlay-only\n", "      updateStrategy: overlay-only\n      phase: x\n", 1)+"mirror: y\n")
			w.label("two keys the reader does not know: the first is named, and how many more")
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"status-refuses-an-unusable-registry", func(w *registryWorld) {
			w.eachRegistryState(func(registry string) []string { return []string{"skills", "status", "--registry", registry} })
		}},
		{"list-refuses-an-unusable-registry", func(w *registryWorld) {
			w.eachRegistryState(func(registry string) []string { return []string{"skills", "list", "--registry", registry} })
		}},
		{"add-registers-a-skill-in-the-registry-and-the-manifest", func(w *registryWorld) {
			w.overlay()
			w.putSkill("gamma", true)
			w.label("a custom skill")
			w.run("skills", "add", "gamma", "--source-root", w.path("skills"))
			w.putSkill("delta", true)
			w.label("an external skill, with its repository and its ref")
			w.run("skills", "add", "delta", "--repo", "https://example.test/org/delta", "--ref", "v2.0.1", "--source-root", w.path("skills"))
			w.putSkill("epsilon", true)
			w.label("an external skill with no ref")
			w.run("skills", "add", "epsilon", "--repo", "https://example.test/org/epsilon", "--source-root", w.path("skills"))
			w.label("what the three left, the registry written in the one form the program writes")
			w.show(worldRegistry)
			w.show(worldManifest)
		}},
		{"add-writes-the-registry-in-its-own-form", func(w *registryWorld) {
			// The registry is rewritten whole, in the one form the program writes: what the file
			// held in another form (comments, quotes, the order of keys, targets) is gone.
			w.put(worldRegistry, "# my registry\n---\nversion: '1'\nskills:\n  # the first\n  - lifecycle:\n      updateStrategy: 'overlay-only'\n    id: \"alpha\"\n    install:\n      targets:\n        - claude\n        - pi\n      defaultScope: project\n      allowedProjects:\n        - demo\n    path: alpha\n    source:\n      type: custom\n")
			w.put(worldManifest, "alpha/SKILL.md custom\n")
			w.putSkill("alpha", false)
			w.putSkill("gamma", true)
			w.run("skills", "add", "gamma", "--source-root", w.path("skills"))
			w.show(worldRegistry)
			w.show(worldManifest)
		}},
		{"add-refuses-what-it-cannot-add", func(w *registryWorld) {
			w.overlay()
			w.putSkill("gamma", true)
			before := w.read(worldRegistry)
			beforeManifest := w.read(worldManifest)
			w.label("no id")
			w.run("skills", "add", "--source-root", w.path("skills"))
			w.label("a ref with no repository")
			w.run("skills", "add", "gamma", "--ref", "v1", "--source-root", w.path("skills"))
			w.label("an id that is not a slug")
			w.run("skills", "add", "Gamma_1", "--source-root", w.path("skills"))
			w.label("an id already registered")
			w.run("skills", "add", "alpha", "--source-root", w.path("skills"))
			w.label("a skill with no SKILL.md")
			w.run("skills", "add", "nothing", "--source-root", w.path("skills"))
			w.putSkillBytes("lintbad", strings.Replace(skillFile("lintbad"), "  version: \"1.0\"\n", "", 1), true)
			w.label("a skill that does not pass the lint")
			w.run("skills", "add", "lintbad", "--source-root", w.path("skills"))
			w.putSkill("unapproved", false)
			w.label("a global skill with no approval")
			w.run("skills", "add", "unapproved", "--source-root", w.path("skills"))
			w.putSkill("unslugged", true)
			w.label("a repository with a character the file cannot hold")
			w.run("skills", "add", "unslugged", "--repo", "https://example.test/{x}", "--source-root", w.path("skills"))
			w.label("a manifest that is not there")
			w.run("skills", "add", "gamma", "--manifest", w.path("none.manifest"), "--source-root", w.path("skills"))
			if w.read(worldRegistry) != before || w.read(worldManifest) != beforeManifest {
				w.write("!!! a refusal changed a file\n")
			}
			w.label("and every refusal left the registry and the manifest as they were")
			w.tree("skills/unslugged")
		}},
		{"add-refuses-an-unusable-registry-and-writes-nothing", func(w *registryWorld) {
			w.overlay()
			w.putSkill("gamma", true)
			w.eachRegistryState(func(registry string) []string {
				return []string{"skills", "add", "gamma", "--registry", registry, "--manifest", w.path(worldManifest), "--source-root", w.path("skills")}
			})
			w.show(worldManifest)
			w.tree("skills/gamma")
		}},
		{"remove-unregisters-a-skill-from-the-registry-and-the-manifest", func(w *registryWorld) {
			w.overlay()
			w.label("a skill of several")
			w.run("skills", "remove", "alpha")
			w.show(worldRegistry)
			w.show(worldManifest)
			w.label("the skill's files stay")
			w.tree("skills/alpha")
		}},
		{"remove-leaves-a-registry-with-no-skills-when-it-removes-the-last", func(w *registryWorld) {
			w.put(worldRegistry, registryOf("only"))
			w.put(worldManifest, "engine/go.mod managed\nonly/SKILL.md custom\nonly/references/a.md custom\n")
			w.run("skills", "remove", "only")
			w.show(worldRegistry)
			w.show(worldManifest)
			w.label("and that registry reads as one of nothing")
			w.run("skills", "list")
			w.run("skills", "status")
		}},
		{"remove-refuses-what-it-cannot-remove", func(w *registryWorld) {
			w.overlay()
			before := w.read(worldRegistry)
			w.label("no id")
			w.run("skills", "remove")
			w.label("an id that is not registered")
			w.run("skills", "remove", "nothing")
			w.label("a manifest that is not there")
			w.run("skills", "remove", "alpha", "--manifest", w.path("none.manifest"))
			if w.read(worldRegistry) != before {
				w.write("!!! a refusal changed the registry\n")
			}
		}},
		{"add-and-remove-refuse-a-manifest-that-already-disagrees-with-the-registry", func(w *registryWorld) {
			// The cross-check that follows the change runs over the whole registry, not over the
			// entry changed: a manifest that was out of step before is refused, with its
			// divergences, and nothing is written.
			w.overlay()
			w.putSkill("gamma", true)
			w.put(worldManifest, strings.Replace(goldenManifest, "alpha/SKILL.md custom\n", "", 1))
			before, beforeManifest := w.read(worldRegistry), w.read(worldManifest)
			w.label("add, with no row for alpha")
			w.run("skills", "add", "gamma", "--source-root", w.path("skills"))
			w.label("remove, with no row for alpha")
			w.run("skills", "remove", "ext-one")
			if w.read(worldRegistry) != before || w.read(worldManifest) != beforeManifest {
				w.write("!!! a refusal changed a file\n")
			}
		}},
		{"remove-refuses-an-unusable-registry", func(w *registryWorld) {
			w.overlay()
			w.eachRegistryState(func(registry string) []string {
				return []string{"skills", "remove", "alpha", "--registry", registry, "--manifest", w.path(worldManifest)}
			})
			w.show(worldManifest)
		}},
		{"sync-manifest-regenerates-the-skill-rows-from-the-registry", func(w *registryWorld) {
			w.overlay()
			w.label("a manifest that is in sync")
			w.run("skills", "sync-manifest")
			w.show(worldManifest)
			// alpha: no row. beta: the wrong tag. ghost: a row with no entry. ext-one: both tags.
			w.put(worldManifest, "# kept\nengine/go.mod managed\nbeta/SKILL.md custom\next-one/SKILL.md custom\next-one/SKILL.md managed\nghost/SKILL.md custom\ntidy-notes/SKILL.md custom\nroot-file managed\n")
			w.label("a manifest that is not")
			w.run("skills", "sync-manifest")
			w.show(worldManifest)
			w.label("and then it is")
			w.run("skills", "sync-manifest")
		}},
		{"sync-manifest-refuses-what-it-cannot-sync", func(w *registryWorld) {
			w.overlay()
			w.label("a manifest that is not there")
			w.run("skills", "sync-manifest", "--manifest", w.path("none.manifest"))
			w.eachRegistryState(func(registry string) []string {
				return []string{"skills", "sync-manifest", "--registry", registry, "--manifest", w.path(worldManifest)}
			})
			w.show(worldManifest)
		}},
		{"skills-verbs-and-a-registry-named-by-a-relative-path", func(w *registryWorld) {
			w.overlay()
			w.put("sub/registry.yaml", registryOf("sub-skill"))
			w.label("run from the world, the registry in a directory below it")
			w.run("skills", "list", "--registry", "sub/registry.yaml")
			w.label("run from that directory, the default registry is not there")
			w.runIn("sub", "skills", "list")
			w.label("run from that directory, the registry named relative to it")
			w.runIn("sub", "skills", "status", "--registry", "registry.yaml")
		}},
	}
}
