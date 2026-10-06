package main

import (
	"fmt"
	"strings"
)

// The cases of the registry goldens that are about the registry itself: how a file is read
// (what is accepted, and the words of every refusal), shown through 'skills list', which does
// nothing else with it. The verbs that do more are in registry_golden_verbs_test.go.
//
// A refusal is a document with exactly one thing wrong in it, except in the case that is about
// which of two faults of one entry the program names first (the one the file says first): a
// document that holds two faults of different kinds, one in each of two entries, would pin an
// order nobody chose.

func registryGoldenCases() []registryGoldenCase {
	var cases []registryGoldenCase
	cases = append(cases, registryReadingCases()...)
	cases = append(cases, registryRefusalCases()...)
	cases = append(cases, registryPolicyCases()...)
	cases = append(cases, registryVerbCases()...)
	cases = append(cases, registryProjectCases()...)
	cases = append(cases, registryPackageCases()...)
	return cases
}

// baseEntry is a registry of one valid entry: the document the refusals are made from.
const baseEntry = `version: "1"
skills:
  - id: alpha
    path: alpha
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// changed is baseEntry with the first old replaced by new: a document with one fault.
func changed(old, new string) string {
	if !strings.Contains(baseEntry, old) {
		panic(fmt.Sprintf("registry golden: %q is not in the base entry", old))
	}
	return strings.Replace(baseEntry, old, new, 1)
}

// registryDoc is a document and what the transcript calls it.
type registryDoc struct{ label, text string }

// listEach records 'skills list --registry <doc>' for each document, in its own file of the
// world, so a transcript shows what each was read as. The files are docs/01.yaml and on; a case
// that lists twice says where the second list goes (listEachIn), so that one path never stands
// for two documents in a transcript.
func (w *registryWorld) listEach(docs []registryDoc) { w.t.Helper(); w.listEachIn("docs", docs) }

// listEachIn is listEach with the files in the directory dir of the world.
func (w *registryWorld) listEachIn(dir string, docs []registryDoc) {
	w.t.Helper()
	for i, doc := range docs {
		path := w.registryAt(fmt.Sprintf("%s/%02d.yaml", dir, i+1), doc.text)
		w.label("%s", doc.label)
		w.run("skills", "list", "--registry", path)
	}
}

func registryReadingCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"list-prints-the-entries-sorted-by-id", func(w *registryWorld) {
			w.put(worldRegistry, goldenRegistryYAML)
			w.label("the registry in the working directory")
			w.run("skills", "list")
			w.label("a registry named by its path")
			w.run("skills", "list", "--registry", w.registryAt("elsewhere/other.yaml", registryOf("zed", "mid", "ant")))
			w.label("a flag with no value reads the registry in the working directory")
			w.run("skills", "list", "--registry")
			w.label("a later flag wins")
			w.run("skills", "list", "--registry", w.path("nothing.yaml"), "--registry", w.path(worldRegistry))
			w.label("words that are no flag are not read")
			w.run("skills", "list", "extra", "--registry", w.path(worldRegistry))
			w.label("a flag nobody named is refused (decision D4)")
			w.run("skills", "list", "extra", "--unknown", "--registry", w.path(worldRegistry))
		}},
		{"list-prints-nothing-for-a-registry-without-skills", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"skills with no entries", "version: \"1\"\nskills:\n"},
				{"skills with no entries and comments", "# nothing here\nversion: \"1\"\n\n# yet\nskills:\n"},
			})
		}},
		{"list-reads-the-forms-of-yaml-the-subset-allows", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"comments, blank lines and the document marker", "---\n# a registry\n\nversion: \"1\"\n\nskills:\n  # first\n  - id: alpha\n    path: alpha\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n\n    lifecycle:\n      updateStrategy: overlay-only\n"},
				{"quoted scalars, double and single", strings.NewReplacer(`id: alpha`, `id: "alpha"`, `type: custom`, `type: 'custom'`, `- claude`, `- "claude"`, `overlay-only`, `'overlay-only'`).Replace(baseEntry)},
				{"a hash inside quotes is part of the value", changed("- claude", `- "cla # ude"`)},
				{"white space after a value is not part of it", changed("id: alpha", "id: alpha   ")},
				{"the keys of an entry in any order", "version: \"1\"\nskills:\n  - lifecycle:\n      updateStrategy: overlay-only\n    install:\n      targets:\n        - claude\n      defaultScope: global\n    source:\n      type: custom\n    path: alpha\n    id: alpha\n"},
				{"the version first or last", "skills:\n  - id: alpha\n    path: alpha\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\nversion: \"1\"\n"},
				{"the version unquoted", strings.Replace(baseEntry, `version: "1"`, "version: 1", 1)},
				{"the version in single quotes", strings.Replace(baseEntry, `version: "1"`, "version: '1'", 1)},
				{"lines that end with a carriage return", strings.ReplaceAll(baseEntry, "\n", "\r\n")},
				{"a path that is not the id", changed("path: alpha", "path: group/alpha")},
				{"targets in their own order, and repeated", changed("        - claude\n", "        - pi\n        - claude\n        - pi\n")},
				{"an id that is not a slug", changed("id: alpha", "id: Alpha Beta é")},
				{"a core skill with no upstream", changed("type: custom", "type: core")},
				{"a project skill with no projects", changed("defaultScope: global", "defaultScope: project")},
				{"a project skill with projects repeated", changed("defaultScope: global", "defaultScope: project\n      allowedProjects:\n        - demo\n        - demo")},
				{"two entries with the same path", baseEntry + "  - id: beta\n    path: alpha\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"},
			})
		}},
		{"remove-shows-everything-the-reader-read-of-the-rest", func(w *registryWorld) {
			// 'list' shows four columns of an entry. 'remove' writes the rest of the registry back,
			// whole, in the one form the program writes, so what it leaves is what was read of
			// every field of every other entry, whatever form it was in: quotes, the order of
			// the keys, a field that is missing, a value that needs quotes to be written back.
			odd := `# a registry in the forms the subset allows
---
version: '1'
skills:
  - lifecycle:
      updateStrategy: 'vendor-merge'
    install:
      targets:
        - "claude"
        - 'pi'
      defaultScope: "global"
    source:
      upstream:
        owner:   "an owner"
      type: core
    path: first
    id: "first"
  - id: second
    path: second
    source:
      type: core
    install:
      defaultScope: global
      targets:
        - codex
    lifecycle:
      updateStrategy: vendor-merge
  - id: third
    path: third
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - demo
        - demo
        - "- dash"
        - "x:"
        - "has # hash"
        - " lead"
        - "trail "
        - "?q"
        - "'single'"
        - ""
      targets:
        - opencode
    lifecycle:
      updateStrategy: overlay-only
  - id: ünï-fourth
    path: fourth
    source:
      type: external
      repo: "https://example.test/a b/c"
      ref: "v1 # candidate"
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: victim
    path: victim
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
			manifest := "first/SKILL.md managed\nsecond/SKILL.md managed\nthird/SKILL.md custom\nfourth/SKILL.md custom\nvictim/SKILL.md custom\n"
			w.put(worldRegistry, odd)
			w.put(worldManifest, manifest)
			w.label("a registry in odd forms")
			w.run("skills", "remove", "victim")
			w.show(worldRegistry)
			w.put(worldRegistry, strings.ReplaceAll(odd, "\n", "\r\n"))
			w.put(worldManifest, manifest)
			w.label("the same registry with every line ending in a carriage return")
			w.run("skills", "remove", "victim")
			w.show(worldRegistry)
		}},
		{"registry-cannot-read-a-list-item-with-a-colon-and-a-space", func(w *registryWorld) {
			// The reader looks for the colon that ends a key before it looks at quotes, so an item
			// of a list written "a: b" is the start of a mapping, not a value, and the list ends
			// there. The writer quotes such a value to keep it, which the reader then cannot read
			// back. Pinned as it is: the registry that holds one is refused with the words of
			// what that makes missing. A value that is not an item of a list reads fine.
			const withColon = "version: \"1\"\nskills:\n  - id: alpha\n    path: alpha\n    source:\n      type: custom\n    install:\n      defaultScope: project\n      allowedProjects:\n        - \"a: b\"\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
			w.put(worldRegistry, withColon)
			w.label("a project named 'a: b' in the registry")
			w.run("skills", "list")
			w.listEach([]registryDoc{{"a repository with ': ' inside quotes, which is not an item of a list", strings.Replace(baseEntry, "      type: custom", "      type: external\n      repo: \"https://example.test/a: b\"", 1)}})
			w.put(worldRegistry, baseEntry)
			w.put(worldManifest, "alpha/SKILL.md custom\n")
			w.putSkill("alpha", true)
			w.putSkill("gamma", true)
			w.label("'add' writes such a repository, quoted, and reads it back")
			w.run("skills", "add", "gamma", "--repo", "https://example.test/a: b", "--source-root", w.path("skills"))
			w.show(worldRegistry)
		}},
		{"registry-can-be-read-with-an-entry-that-has-no-scope-and-not-written", func(w *registryWorld) {
			// An entry with no defaultScope is accepted (its scope is empty). The writer writes an
			// empty scope as "", which the reader refuses (the scope is checked against its two
			// words wherever it is written), so 'add' and 'remove' of any entry refuse a registry
			// that holds one. Pinned as it is.
			noScope := strings.Replace(baseEntry, "      defaultScope: global\n", "", 1) + strings.TrimPrefix(registryOf("victim"), "version: \"1\"\nskills:\n")
			w.put(worldRegistry, noScope)
			w.put(worldManifest, "alpha/SKILL.md custom\nvictim/SKILL.md custom\n")
			w.label("read, it lists and counts")
			w.run("skills", "list")
			w.run("skills", "status")
			w.label("written back, it is refused, and nothing is changed")
			w.run("skills", "remove", "victim")
			w.show(worldRegistry)
			w.show(worldManifest)
		}},
		{"status-counts-what-it-reads", func(w *registryWorld) {
			w.put(worldRegistry, goldenRegistryYAML)
			w.label("core and custom are counted, external and project skills are in the total")
			w.run("skills", "status")
			w.label("a registry of nothing")
			w.run("skills", "status", "--registry", w.registryAt("empty.yaml", "version: \"1\"\nskills:\n"))
			w.label("another registry")
			w.run("skills", "status", "--registry", w.registryAt("other.yaml", registryOf("a", "b", "c")))
			w.label("status never reads the manifest, so a missing one is no matter")
			w.run("skills", "status", "--manifest", w.path("no-manifest"))
		}},
	}
}

func registryRefusalCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"registry-refuses-a-file-it-cannot-read-or-that-has-nothing-to-read", func(w *registryWorld) {
			w.label("no file at the default path")
			w.run("skills", "list")
			w.label("no file at the path named")
			w.run("skills", "list", "--registry", w.path("nowhere/skills.registry.yaml"))
			w.mkdir("a-directory")
			w.label("a directory")
			w.run("skills", "list", "--registry", w.path("a-directory"))
			w.listEach([]registryDoc{
				{"an empty file", ""},
				{"only a comment", "# nothing\n"},
				{"only the document marker", "---\n"},
				{"only a version", "version: \"1\"\n"},
				{"only skills", "skills:\n"},
				{"not YAML at all", "hello world\n"},
				{"a byte-order mark before the first key", "\xef\xbb\xbfversion: \"1\"\nskills:\n"},
				{"a line longer than the reader's buffer", "# " + strings.Repeat("a", 70000) + "\n" + baseEntry},
			})
		}},
		{"registry-refuses-what-the-yaml-subset-leaves-out", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"a tab where the indentation is", changed("    path: alpha", "\tpath: alpha")},
				{"a tab inside a value", changed("path: alpha", "path: al\tpha")},
				{"a flow sequence", changed("      targets:\n        - claude\n", "      targets: [claude]\n")},
				{"a flow mapping", changed("    source:\n      type: custom\n", "    source: {type: custom}\n")},
				{"an anchor", changed("id: alpha", "id: &a alpha")},
				{"an alias", changed("path: alpha", "path: *a")},
				{"a tag", changed("id: alpha", "id: !!str alpha")},
				{"a literal block scalar", changed("path: alpha", "path: |\n      alpha")},
				{"a folded block scalar with a chomping mark", changed("path: alpha", "path: >-\n      alpha")},
				{"a block scalar indicator on a line of its own", changed("      type: custom", "      type: custom\n      |-")},
				{"a second document", baseEntry + "---\n" + baseEntry},
				{"the end of a document, then content", baseEntry + "...\n" + baseEntry},
				{"a comment after a value", changed("id: alpha", "id: alpha # the first")},
				{"a comment after an item of a list", changed("- claude", "- claude # the default")},
				{"a double quote that is not closed", changed("id: alpha", `id: "alpha`)},
				{"a single quote that is not closed", changed("id: alpha", "id: 'alpha")},
				{"a line that is not a key and a value", changed("id: alpha", "id alpha")},
				{"a key with no name", changed("id: alpha", ": alpha")},
				{"an item of a list with a comment, as a bare word", changed("- claude", "- cla # ude")},
			})
		}},
		{"registry-refuses-a-document-that-is-not-shaped-like-one", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"an indented key at the root", "  version: \"1\"\nskills:\n"},
				{"an entry at the root", "version: \"1\"\n- id: alpha\n"},
				{"skills with a value", "version: \"1\"\nskills: nope\n"},
				{"skills as a flow list", "version: \"1\"\nskills: []\n"},
				{"entries that are not items of a list", "version: \"1\"\nskills:\n  id: alpha\n"},
				{"an item that is a bare word", "version: \"1\"\nskills:\n  - alpha\n"},
				{"entries indented too far", "version: \"1\"\nskills:\n    - id: alpha\n"},
				{"an entry's keys at the wrong depth", changed("    path: alpha", "   path: alpha")},
				{"a key of source too deep", changed("      type: custom", "       type: custom")},
				{"a key of install too shallow", changed("      defaultScope: global", "     defaultScope: global")},
			})
		}},
		{"registry-leaves-out-keys-it-does-not-know-and-refuses-keys-that-repeat", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"an unknown key at the root, with a value", "extra: 1\n" + baseEntry},
				{"an unknown key at the root, with none", "extra:\n" + baseEntry},
				{"an unknown key in an entry", changed("    path: alpha", "    color: red\n    path: alpha")},
				{"an unknown key in an entry, after the fields", baseEntry + "    color: red\n"},
				{"an unknown key in a source", changed("      type: custom", "      type: custom\n      mirror: x")},
				{"an unknown key in an upstream", changed("      type: custom", "      type: core\n      upstream:\n        name: x")},
				{"an unknown key in an install", changed("      defaultScope: global", "      defaultScope: global\n      mode: x")},
				{"an unknown key in a lifecycle", changed("      updateStrategy: overlay-only", "      updateStrategy: overlay-only\n      phase: x")},
				{"a field of an entry twice", changed("    path: alpha", "    id: alpha\n    path: alpha")},
				{"a field of a source twice", changed("      type: custom", "      type: custom\n      type: custom")},
				{"a field of an upstream twice", changed("      type: custom", "      type: core\n      upstream:\n        owner: a\n        owner: b")},
				{"a field of an install twice", changed("      defaultScope: global", "      defaultScope: global\n      defaultScope: global")},
				{"a field of a lifecycle twice", changed("      updateStrategy: overlay-only", "      updateStrategy: overlay-only\n      updateStrategy: overlay-only")},
				// Decision 5 of the owner: the format took the last of a key of the root said twice
				// (the entries of the first block of 'skills' were lost without a word); it is refused
				// now, naming both lines, as a key that repeats inside a mapping is.
				{"a key at the root twice: skills", baseEntry + "skills:\n  - id: other\n    path: other\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - codex\n    lifecycle:\n      updateStrategy: overlay-only\n"},
				{"a key at the root twice: version", "version: \"2\"\n" + baseEntry},
				{"a key at the root twice: version, the second one a version it does not know", baseEntry + "version: \"2\"\n"},
				{"a key at the root twice: one the reader does not know", "extra: 1\n" + baseEntry + "extra: 2\n"},
			})
		}},
		{"registry-refuses-a-version-it-does-not-know", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"no version", strings.Replace(baseEntry, "version: \"1\"\n", "", 1)},
				{"version 2", strings.Replace(baseEntry, `"1"`, `"2"`, 1)},
				{"version 1.0", strings.Replace(baseEntry, `"1"`, `"1.0"`, 1)},
				{"version v1", strings.Replace(baseEntry, `"1"`, `v1`, 1)},
				{"an empty version", strings.Replace(baseEntry, `"1"`, `""`, 1)},
				{"a version with no value", strings.Replace(baseEntry, `version: "1"`, "version:", 1)},
				{"a version of 0", strings.Replace(baseEntry, `"1"`, `0`, 1)},
			})
		}},
		{"registry-refuses-an-entry-that-is-not-valid", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"no id", strings.Replace(baseEntry, "  - id: alpha\n    path: alpha\n", "  - path: alpha\n", 1)},
				{"an empty id", changed("id: alpha", `id: ""`)},
				{"no path", strings.Replace(baseEntry, "    path: alpha\n", "", 1)},
				{"an empty path", changed("path: alpha", `path: ""`)},
				{"an absolute path", changed("path: alpha", "path: /etc/alpha")},
				{"a path with a dot first", changed("path: alpha", "path: ./alpha")},
				{"a path with a slash at its end", changed("path: alpha", "path: alpha/")},
				{"a path with two slashes", changed("path: alpha", "path: a//b")},
				{"a path that climbs", changed("path: alpha", "path: a/../b")},
				{"a path that leaves its root", changed("path: alpha", "path: ../alpha")},
				{"a path that is only dots", changed("path: alpha", "path: ..")},
				{"no type of source", changed("      type: custom\n", "      mirror: x\n")},
				{"an unknown type of source", changed("type: custom", "type: wizard")},
				{"an empty type of source", changed("type: custom", `type: ""`)},
				{"a custom skill with an upstream", changed("      type: custom", "      type: custom\n      upstream:\n        owner: someone")},
				{"an external skill with an upstream", changed("      type: custom", "      type: external\n      repo: https://example.test/r\n      upstream:\n        owner: someone")},
				{"a core skill whose upstream has no owner", changed("      type: custom", "      type: core\n      upstream:\n        owner: \"\"")},
				{"a core skill whose upstream has an owner of white space", changed("      type: custom", "      type: core\n      upstream:\n        owner:    ")},
				{"no targets", strings.Replace(baseEntry, "      targets:\n        - claude\n", "", 1)},
				{"targets with no items", changed("        - claude\n", "")},
				{"a target that does not exist", changed("- claude", "- vim")},
				{"a target in capitals", changed("- claude", "- Claude")},
				{"an empty target", changed("- claude", `- ""`)},
				{"no lifecycle", strings.Replace(baseEntry, "    lifecycle:\n      updateStrategy: overlay-only\n", "", 1)},
				{"an update strategy that does not exist", changed("overlay-only", "rolling")},
				{"no update strategy in a lifecycle", changed("      updateStrategy: overlay-only", "      other: x")},
				{"no install", strings.Replace(baseEntry, "    install:\n      defaultScope: global\n      targets:\n        - claude\n", "", 1)},
				{"projects on a global skill", changed("      targets:\n", "      allowedProjects:\n        - demo\n      targets:\n")},
				{"no source", strings.Replace(baseEntry, "    source:\n      type: custom\n", "", 1)},
			})
		}},
		{"registry-refuses-a-source-that-contradicts-its-type", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"a repository on a custom skill", changed("      type: custom", "      type: custom\n      repo: https://example.test/r")},
				{"a ref on a custom skill", changed("      type: custom", "      type: custom\n      ref: v1")},
				{"a repository on a core skill", changed("      type: custom", "      type: core\n      repo: https://example.test/r")},
				{"a ref on a core skill", changed("      type: custom", "      type: core\n      ref: v1")},
				{"an external skill with no repository", changed("type: custom", "type: external")},
				{"an external skill with a ref and no repository", changed("      type: custom", "      type: external\n      ref: v1")},
				{"a repository before the type", changed("      type: custom", "      repo: https://example.test/r\n      type: custom")},
				{"an external skill with an empty repository", changed("      type: custom", "      type: external\n      repo: \"\"")},
				{"a ref on a custom skill, after the repository fields", changed("      type: custom", "      ref: v1\n      type: custom")},
			})
			w.label("the entry is named by the id read so far: after the source, the id is not yet known")
			w.listEachIn("docs/id-order", []registryDoc{
				{"an external skill with no repository, its id after its source", "version: \"1\"\nskills:\n  - source:\n      type: external\n    id: alpha\n    path: alpha\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"},
				{"the same, its id before its source", "version: \"1\"\nskills:\n  - id: alpha\n    source:\n      type: external\n    path: alpha\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"},
			})
		}},
		{"registry-refuses-a-scope-outside-its-vocabulary", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"a scope that does not exist", changed("defaultScope: global", "defaultScope: workspace")},
				{"a scope in capitals", changed("defaultScope: global", "defaultScope: Global")},
				{"an empty scope", changed("defaultScope: global", `defaultScope: ""`)},
				{"a scope with no value", changed("defaultScope: global", "defaultScope:")},
				{"no scope: the targets alone", changed("      defaultScope: global\n", "")},
			})
		}},
		{"registry-refuses-an-id-that-repeats", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"two entries of one id", baseEntry + strings.TrimPrefix(baseEntry, "version: \"1\"\nskills:\n")},
				{"two entries of one id, the paths differing", baseEntry + strings.Replace(strings.TrimPrefix(baseEntry, "version: \"1\"\nskills:\n"), "path: alpha", "path: other", 1)},
				{"ids that differ only in case are two", baseEntry + strings.Replace(strings.TrimPrefix(baseEntry, "version: \"1\"\nskills:\n"), "id: alpha\n    path: alpha", "id: ALPHA\n    path: ALPHA", 1)},
			})
		}},
		{"registry-refuses-the-first-fault-it-meets", func(w *registryWorld) {
			// Two faults in one entry: the one the file says first. The order of the fields is
			// the order the program reads them in, so it is the order it names them in.
			w.listEach([]registryDoc{
				{"an unknown key, then an invalid scope", changed("      defaultScope: global", "      mode: x\n      defaultScope: workspace")},
				{"an invalid scope, then an unknown key", changed("      defaultScope: global", "      defaultScope: workspace\n      mode: x")},
				{"a repository on a custom skill, then an invalid scope", "version: \"1\"\nskills:\n  - id: alpha\n    path: alpha\n    source:\n      type: custom\n      repo: https://example.test/r\n    install:\n      defaultScope: workspace\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"},
				{"an invalid scope, then a repository on a custom skill", "version: \"1\"\nskills:\n  - id: alpha\n    path: alpha\n    install:\n      defaultScope: workspace\n      targets:\n        - claude\n    source:\n      type: custom\n      repo: https://example.test/r\n    lifecycle:\n      updateStrategy: overlay-only\n"},
			})
		}},
	}
}
