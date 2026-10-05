package main

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The cases of the file system goldens about what a verb reads from disk: the skills tree that
// 'validate' walks, the manifest it opens, the lock file it looks for, and the source tree of a
// skill that 'install' copies.

// validateIn records 'skills validate' over the world's registry and manifest and the skills
// tree at root, which is written as it is given.
func (w *registryWorld) validateIn(label, root string) {
	w.t.Helper()
	w.label("%s", label)
	w.run("skills", "validate", "--source-root", root)
}

func fsReadCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"fs-validate-walks-the-skills-tree-and-names-what-it-finds", func(w *registryWorld) {
			w.overlay()
			// Every kind of thing a skills tree can hold. The walk reports the regular files that
			// have no row, sorted, and leaves out the names that begin with a dot, links and
			// anything that is not a regular file.
			w.put("skills/alpha/references/deep/notes.md", "deep\n")
			w.put("skills/alpha/.scratch", "an editor's file\n")
			w.put("skills/.git/config", "[core]\n")
			w.put("skills/.hidden/readme.md", "hidden\n")
			w.put("skills/zeta/last.md", "last\n")
			// A directory and a file whose names sort the other way round than their paths do.
			w.put("skills/zeta/a/b.md", "in the directory\n")
			w.put("skills/zeta/a.c", "beside it\n")
			w.put("skills/ünï/cødé.md", "unicode\n")
			w.put("skills/with space/a file.md", "space\n")
			w.mkdir("skills/empty-dir")
			w.symlink("SKILL.md", "skills/alpha/link-to-a-file.md")
			w.symlink("../beta", "skills/alpha/link-to-a-directory")
			w.symlink("nowhere", "skills/alpha/dangling")
			w.fifo("skills/alpha/pipe")
			w.validateIn("a tree with a file of every kind", w.path("skills"))
		}},
		{"fs-validate-counts-the-files-of-an-aligned-tree", func(w *registryWorld) {
			w.overlay()
			w.put("skills/alpha/.scratch", "an editor's file\n")
			w.symlink("SKILL.md", "skills/alpha/link-to-a-file.md")
			w.mkdir("skills/empty-dir")
			w.validateIn("a tree whose only files are the rows of the manifest", w.path("skills"))
			w.validateIn("the root with a trailing separator", w.path("skills")+"/")
			w.validateIn("the root written with a dot segment", w.path("skills/../skills"))
		}},
		{"fs-validate-refuses-a-skills-root-it-cannot-scan", func(w *registryWorld) {
			w.overlay()
			w.put("a-file", "not a directory\n")
			w.mkdir("empty")
			w.symlink("skills", "link-to-skills")
			w.symlink("nowhere", "dangling")
			w.validateIn("a root that does not exist", w.path("missing"))
			w.validateIn("a root that is a file", w.path("a-file"))
			w.validateIn("a root that is an empty directory", w.path("empty"))
			w.validateIn("a root that is a link to the tree", w.path("link-to-skills"))
			w.validateIn("a root that is a dangling link", w.path("dangling"))
			w.validateIn("a root below a file", w.path("a-file/skills"))
			w.validateIn("a root with no name", "")
			w.validateIn("a relative root", "skills")
		}},
		{"fs-validate-refuses-a-tree-with-a-directory-it-cannot-read", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.overlay()
			w.chmod("skills/alpha", 0o000)
			w.validateIn("a skill directory with no permission bits", w.path("skills"))
			w.chmod("skills", 0o000)
			w.validateIn("the root itself with none", w.path("skills"))
		}},
		{"fs-validate-opens-the-manifest-it-is-given", func(w *registryWorld) {
			w.overlay()
			validateManifest := func(label string, manifest string) {
				w.t.Helper()
				w.label("%s", label)
				w.run("skills", "validate", "--manifest", manifest, "--source-root", w.path("skills"))
			}
			validateManifest("a manifest that does not exist", w.path("missing.manifest"))
			w.mkdir("a-directory.manifest")
			validateManifest("a manifest that is a directory", w.path("a-directory.manifest"))
			w.put("empty.manifest", "")
			validateManifest("a manifest with nothing in it", w.path("empty.manifest"))
			w.put("crlf.manifest", strings.ReplaceAll(goldenManifest, "\n", "\r\n"))
			validateManifest("a manifest with CRLF line ends", w.path("crlf.manifest"))
			w.put("no-newline.manifest", strings.TrimSuffix(goldenManifest, "\n"))
			validateManifest("a manifest whose last row has no line end", w.path("no-newline.manifest"))
			w.put("long.manifest", goldenManifest+"# "+strings.Repeat("a", 70000)+"\n")
			validateManifest("a manifest with a line longer than the reader's buffer", w.path("long.manifest"))
			w.symlink(worldManifest, "link.manifest")
			validateManifest("a manifest that is a link to the real one", w.path("link.manifest"))
			w.symlink("nowhere", "dangling.manifest")
			validateManifest("a manifest that is a dangling link", w.path("dangling.manifest"))
			validateManifest("a manifest with no name", "")
		}},
		{"fs-validate-refuses-a-manifest-it-cannot-read", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.overlay()
			w.chmod(worldManifest, 0o000)
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"fs-validate-and-the-lock-it-looks-for", func(w *registryWorld) {
			w.overlay()
			w.label("no lock file beside the registry: validate holds nothing, and creates nothing")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.state(".")
			w.label("the registry's lock file is there (a writer ran once)")
			w.put(".skills.registry.yaml.lock", "")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.label("the lock file is a directory")
			w.remove(".skills.registry.yaml.lock")
			w.mkdir(".skills.registry.yaml.lock")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.state(".")
		}},
		{"fs-validate-in-an-overlay-it-cannot-write-to", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.overlay()
			w.chmod(".", 0o555)
			w.label("a shared lock creates nothing, so a read-only overlay is read")
			w.run("skills", "validate", "--source-root", w.path("skills"))
			w.run("skills", "list")
			w.run("skills", "status")
		}},
		{"fs-install-copies-the-source-tree-of-a-skill-as-it-is", func(w *registryWorld) {
			w.projectWorld()
			src := "overlay/skills/tidy-notes/"
			w.putMode(src+"bin/run.sh", "#!/bin/sh\necho run\n", 0o755)
			w.putMode(src+"references/readonly.md", "read only\n", 0o444)
			w.putMode(src+"references/private.md", "private\n", 0o600)
			w.put(src+"references/deep/er/still.md", "deeper\n")
			w.put(src+skills.ApprovalRecordName, "the record of the skill is governance state, not content\n")
			w.put(src+".dotfile", "a dot file is content\n")
			w.put(src+".hidden-dir/inner.md", "a dot directory too\n")
			w.put(src+"ünï/cødé.md", "unicode\n")
			w.mkdir(src + "empty-dir")
			w.mkdir(src + "references/empty-too")
			w.put(src+".tmp-skills-123456", "half a write\n")
			w.put(src+"references/.tmp-skills-987", "half a write, deeper\n")
			w.put(src+"references/.tmp-skills-", "the prefix alone is not a temporary file\n")
			w.put(src+".tmp-skills-dir/kept.md", "a directory with the prefix and a suffix is not a file\n")
			w.put(src+".tmp-skills-/inside.md", "nor is a directory with the prefix alone\n")
			w.put(src+"references/"+skills.ApprovalRecordName, "a record that is not at the root of the skill\n")
			w.symlink("SKILL.md", src+"link-to-a-file.md")
			w.symlink("references", src+"link-to-a-directory")
			w.symlink("nowhere", src+"dangling")
			w.fifo(src + "pipe")
			w.label("install reads the tree: links, pipes, the record at the root and the temporary files of a writer are not copied")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
			w.label("and a second time")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
		}},
		{"fs-install-copies-the-source-through-a-link", func(w *registryWorld) {
			w.projectWorld()
			w.move("overlay/skills/tidy-notes", "overlay/real-tidy-notes")
			w.symlink("../real-tidy-notes", "overlay/skills/tidy-notes")
			w.label("a skill directory that is a link to the tree")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
		}},
		{"fs-install-refuses-a-source-tree-it-cannot-read", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.projectWorld()
			src := "overlay/skills/tidy-notes/"
			w.chmod(src+"references", 0o000)
			w.label("a directory of the source with no permission bits")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
			w.chmod(src+"references", 0o755)
			w.chmod(src+"references/notes.md", 0o000)
			w.label("a file of the source with no permission bits")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
			w.chmod(src+"references/notes.md", 0o644)
			w.chmod(src+"SKILL.md", 0o000)
			w.label("the SKILL.md of the source with none")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
		}},
		{"fs-install-refuses-a-source-that-is-not-a-directory", func(w *registryWorld) {
			w.projectWorld()
			w.removeAll("overlay/skills/only-demo")
			w.mkdir("overlay/skills/only-demo")
			w.label("a skill directory with nothing in it")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
			w.removeAll("overlay/skills/only-demo")
			w.put("overlay/skills/only-demo", "a file where the directory should be\n")
			w.label("a file where the skill directory should be")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
		}},
	}
}
