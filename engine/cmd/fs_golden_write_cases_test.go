package main

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The cases of the file system goldens about what a verb writes in an overlay: the registry and
// the manifest of 'add', 'remove' and 'sync-manifest', and the approval record of 'approve'. Each
// shows the files left (modes and digests, and that no temporary file stays behind) and the words
// of the refusals that come from the operating system.

// approveArgs is the command line of 'approve' for the skill id of the world's skills tree.
func (w *registryWorld) approveArgs(id string, extra ...string) []string {
	return append([]string{"skills", "approve", "--id", id, "--approver", "fixture-reviewer", "--source-root", w.path("skills")}, extra...)
}

// addWorld is an overlay with a registry and a manifest that agree on one skill, and the source of
// that skill and of a second one, foo, that 'add' can register.
func (w *registryWorld) addWorld() {
	w.t.Helper()
	w.put(worldRegistry, registryOf("alpha"))
	w.put(worldManifest, "alpha/SKILL.md custom\n")
	w.putSkill("alpha", true)
	w.putSkill("foo", true)
}

func (w *registryWorld) addFoo(args ...string) {
	w.t.Helper()
	w.run(append([]string{"skills", "add", "--source-root", w.path("skills")}, append(args, "foo")...)...)
}

func fsWriteCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"fs-add-and-remove-leave-the-files-they-wrote-and-nothing-else", func(w *registryWorld) {
			w.addWorld()
			w.label("before")
			w.state(".")
			w.label("add")
			w.addFoo()
			w.state(".")
			w.label("add what is already there")
			w.addFoo()
			w.state(".")
			w.label("remove")
			w.run("skills", "remove", "foo")
			w.state(".")
			w.label("remove what is not there")
			w.run("skills", "remove", "foo")
			w.state(".")
		}},
		{"fs-add-names-the-files-it-is-given", func(w *registryWorld) {
			w.addWorld()
			w.put("registries/other.yaml", registryOf("alpha"))
			w.put("manifests/other.manifest", "alpha/SKILL.md custom\n")
			w.label("a registry and a manifest in other directories")
			w.addFoo("--registry", w.path("registries/other.yaml"), "--manifest", w.path("manifests/other.manifest"))
			w.state("registries")
			w.state("manifests")
			w.label("relative paths, from the directory the program runs in")
			w.runIn("registries", "skills", "remove", "--registry", "other.yaml", "--manifest", "../manifests/other.manifest", "foo")
			w.state("registries")
			w.state("manifests")
			w.label("a manifest that does not exist")
			w.addFoo("--manifest", w.path("missing.manifest"))
			w.label("a manifest that is a directory")
			w.mkdir("a-directory.manifest")
			w.addFoo("--manifest", w.path("a-directory.manifest"))
			w.state(".")
		}},
		{"fs-add-replaces-a-link-to-the-registry-with-the-file-it-wrote", func(w *registryWorld) {
			w.addWorld()
			w.move(worldRegistry, "real/"+worldRegistry)
			w.move(worldManifest, "real/"+worldManifest)
			w.symlink("real/"+worldRegistry, worldRegistry)
			w.symlink("real/"+worldManifest, worldManifest)
			w.label("a registry and a manifest that are links to files elsewhere")
			w.addFoo()
			w.state(".")
			w.label("and the next write")
			w.run("skills", "remove", "foo")
			w.state(".")
		}},
		{"fs-add-in-a-directory-it-cannot-write", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.addWorld()
			// The lock file is made by the first writer; once it is there a writer needs no
			// permission to take it, so the failure is the write of the manifest.
			w.put(".skills.registry.yaml.lock", "")
			w.chmod(".", 0o555)
			w.label("the overlay directory cannot be written to")
			w.addFoo()
			w.run("skills", "remove", "alpha")
			w.state(".")
		}},
		{"fs-sync-manifest-in-a-directory-it-cannot-write", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.addWorld()
			w.put(worldManifest, "alpha/SKILL.md custom\nghost/SKILL.md custom\n")
			w.put(".skills.registry.yaml.lock", "")
			w.chmod(".", 0o555)
			w.label("a manifest that needs a row dropped, in a directory that cannot be written to")
			w.run("skills", "sync-manifest")
			w.state(".")
		}},
		{"fs-add-when-the-registry-cannot-be-written", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.addWorld()
			w.put("registries/.skills.registry.yaml.lock", "")
			w.put("registries/"+worldRegistry, registryOf("alpha"))
			w.chmod("registries", 0o555)
			w.label("the manifest is written, the registry's directory cannot be")
			w.addFoo("--registry", w.path("registries/"+worldRegistry))
			w.state(".")
		}},
		{"fs-sync-manifest-leaves-the-manifest-it-wrote", func(w *registryWorld) {
			w.overlay()
			w.put(worldManifest, "engine/go.mod managed\nalpha/SKILL.md custom\n")
			w.label("a manifest with rows to add and to retag")
			w.run("skills", "sync-manifest")
			w.state(".")
			w.label("the same again: nothing is written")
			w.run("skills", "sync-manifest")
			w.state(".")
			w.label("a manifest that is a link")
			w.put("real.manifest", goldenManifest+"gone/SKILL.md custom\n")
			w.remove(worldManifest)
			w.symlink("real.manifest", worldManifest)
			w.run("skills", "sync-manifest")
			w.state(".")
		}},
		{"fs-approve-writes-the-record-next-to-the-skill", func(w *registryWorld) {
			w.overlay()
			w.removeAll("skills/alpha/" + skills.ApprovalRecordName)
			w.label("a skill with no record")
			w.run(w.approveArgs("alpha")...)
			// The record holds the time it was written: its mode and size are shown, not its digest.
			w.state("skills/alpha", true)
			w.label("the same bytes again: the record is left as it is")
			w.run(w.approveArgs("alpha")...)
			w.state("skills/alpha", true)
			w.label("changed bytes")
			w.put("skills/alpha/SKILL.md", skillFile("alpha")+"\nchanged\n")
			w.run(w.approveArgs("alpha")...)
			w.state("skills/alpha", true)
			w.label("a record that cannot be parsed is replaced")
			w.put("skills/alpha/"+skills.ApprovalRecordName, "not a record\n")
			w.run(w.approveArgs("alpha")...)
			w.state("skills/alpha", true)
		}},
		{"fs-approve-refuses-what-it-cannot-read-or-write", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.overlay()
			w.removeAll("skills/alpha/" + skills.ApprovalRecordName)
			w.label("the skill's directory cannot be written to")
			w.chmod("skills/alpha", 0o555)
			w.run(w.approveArgs("alpha")...)
			w.state("skills/alpha", true)
			w.chmod("skills/alpha", 0o755)
			w.label("a record that cannot be read")
			w.putMode("skills/alpha/"+skills.ApprovalRecordName, "{}\n", 0o000)
			w.run(w.approveArgs("alpha")...)
			w.label("a SKILL.md that cannot be read")
			w.chmod("skills/beta/SKILL.md", 0o000)
			w.run(w.approveArgs("beta")...)
			w.label("a skill that is not there")
			w.run(w.approveArgs("nothing")...)
			w.label("a skill whose SKILL.md is a directory")
			w.mkdir("skills/dir-skill/SKILL.md")
			w.run(w.approveArgs("dir-skill")...)
			w.label("a source root that is not there")
			w.run("skills", "approve", "--id", "alpha", "--approver", "x", "--source-root", w.path("missing"))
		}},
		{"fs-lint-reads-the-file-it-is-given", func(w *registryWorld) {
			w.put("good/SKILL.md", skillFile("good"))
			w.put("bad/SKILL.md", strings.TrimPrefix(skillFile("bad"), "---\n"))
			w.mkdir("a-directory")
			w.symlink("good/SKILL.md", "link.md")
			w.label("a skill that passes")
			w.run("skills", "lint", w.path("good/SKILL.md"))
			w.label("a skill that does not")
			w.run("skills", "lint", w.path("bad/SKILL.md"))
			w.label("a file that is not there")
			w.run("skills", "lint", w.path("missing.md"))
			w.label("a directory")
			w.run("skills", "lint", w.path("a-directory"))
			w.label("a link")
			w.run("skills", "lint", w.path("link.md"))
			w.label("a path with no name")
			w.run("skills", "lint", "")
		}},
	}
}
