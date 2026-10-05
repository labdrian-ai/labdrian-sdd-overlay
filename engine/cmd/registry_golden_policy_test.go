package main

import (
	"fmt"
	"strings"
)

// The cases of the registry goldens that are about the reader policy of H16 (decision Q5): the
// version selects the decoder and is judged first, a key the reader does not know is left out and
// said, and a field the install, the approval and the projection depend on is refused in a shape
// the reader does not read, where a field nothing depends on is left out. They are shown through
// 'skills list', which does nothing else with a registry; what the other verbs do with a registry
// that has a key the reader does not know is in the golden files of those verbs (the second
// state of registryStates).
//
// The documents are made from the registry of every kind of entry (goldenRegistryYAML), whose first
// entry is the core skill that holds a source with an upstream, an install and a lifecycle.

// changedGolden is goldenRegistryYAML with the first old replaced by new: a document with one
// thing different.
func changedGolden(old, new string) string {
	if !strings.Contains(goldenRegistryYAML, old) {
		panic(fmt.Sprintf("registry golden: %q is not in the registry of every kind of entry", old))
	}
	return strings.Replace(goldenRegistryYAML, old, new, 1)
}

func registryPolicyCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"registry-names-its-version-before-it-reads-its-entries", func(w *registryWorld) {
			notValid := strings.Replace(baseEntry, "path: alpha", `path: ""`, 1)
			w.listEach([]registryDoc{
				{"a version it does not know, and then an entry that is not valid", strings.Replace(notValid, `"1"`, `"2"`, 1)},
				{"an entry that is not valid, and a version it knows", notValid},
				{"no version, and then an entry indented wrongly", strings.Replace(changed("    path: alpha", "   path: alpha"), "version: \"1\"\n", "", 1)},
				{"a version of a later format, with entries this reader could not read",
					"version: \"2\"\nskills:\n  - id: alpha\n    color: red\n    shape: 3\n"},
				{"a version at the end of the file", strings.Replace(baseEntry, "version: \"1\"\n", "", 1) + "version: \"1\"\n"},
				{"a version said twice: the last is the version (it is 1)", "version: \"2\"\n" + baseEntry},
				{"a version said twice: the last is the version (it is 2)", baseEntry + "version: \"2\"\n"},
				{"a version with a block under it", strings.Replace(baseEntry, "version: \"1\"\n", "version:\n  nested: 1\n", 1)},
			})
		}},
		{"registry-says-what-it-leaves-out", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"one key it does not know", "extra: 1\n" + baseEntry},
				{"three keys it does not know: the first is named, and how many more", "first: 1\n" +
					changed("    path: alpha", "    path: alpha\n    second: 2") + "third: 3\n"},
				{"the same key it does not know twice at the root", "extra: 1\n" + baseEntry + "extra: 2\n"},
				{"a key it does not know with a block under it", "extra:\n  nested: 1\n  other:\n    - a\n    - b\n" + baseEntry},
				{"two keys it does not know in a source and one at the end of the entry",
					changed("      type: custom", "      type: custom\n      mirror: x\n      mirror2: y") + "    color: red\n"},
			})
		}},
		{"registry-refuses-a-field-the-install-depends-on-in-a-shape-it-does-not-read", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"version with a block under it", changedGolden("version: \"1\"\n", "version:\n  nested: 1\n")},
				{"skills with a value", "version: \"1\"\nskills: nope\n"},
				{"id with a block under it", changedGolden("  - id: beta", "  - id: beta\n      nested: 1")},
				{"path with a block under it", changedGolden("    path: beta", "    path: beta\n      nested: 1")},
				{"source with a value", changedGolden("    source:", "    source: core")},
				{"source.type with a block under it", changedGolden("      type: core", "      type: core\n        nested: 1")},
				{"install with a value", changedGolden("    install:", "    install: global")},
				{"install.defaultScope with a block under it", changedGolden("      defaultScope: global", "      defaultScope: global\n        nested: 1")},
				{"install.allowedProjects with a value", changedGolden("      allowedProjects:", "      allowedProjects: demo")},
				{"install.targets with a value", changedGolden("      targets:", "      targets: claude")},
			})
		}},
		{"registry-leaves-out-a-field-nothing-depends-on-in-a-shape-it-does-not-read", func(w *registryWorld) {
			w.listEach([]registryDoc{
				{"source.upstream with a value", changedGolden("      upstream:", "      upstream: someone")},
				{"source.upstream.owner with a block under it (the domain refuses a core skill with no owner)",
					changedGolden("        owner: gentleman-programming", "        owner:\n          nested: 1")},
				{"source.ref with a block under it", changedGolden("      ref: v1.2.0", "      ref:\n        nested: 1")},
				{"source.repo with a block under it (an external source needs its repository)",
					changedGolden("      repo: https://example.test/org/ext-one", "      repo:\n        nested: 1")},
				{"lifecycle with a value (the value is left out, the block under it is read)",
					changedGolden("    lifecycle:", "    lifecycle: vendor-merge")},
				{"lifecycle.updateStrategy with a block under it (the domain refuses an entry with none)",
					changedGolden("      updateStrategy: vendor-merge", "      updateStrategy:\n        nested: 1")},
			})
		}},
	}
}
