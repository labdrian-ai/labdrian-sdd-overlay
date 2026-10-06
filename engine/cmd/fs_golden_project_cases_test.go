package main

import (
	"strings"
)

// The cases of the file system goldens about a project: what 'install', 'adopt' and the project
// verbs read of it (the lock, the installed files, the paths they resolve), what they write into
// it and what they put back when a write fails, and the words of every refusal that comes from
// the operating system or from a path that leaves the project.

// resetDemo gives the project demo back, empty, between the scenarios of a case.
func (w *registryWorld) resetDemo() {
	w.t.Helper()
	w.restore("demo")
	w.removeAll("demo")
	w.mkdir("demo")
}

func (w *registryWorld) installDemo(label string) {
	w.t.Helper()
	w.label("%s", label)
	w.runIn("demo", w.installArgs("install")...)
}

func fsProjectCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"fs-install-refuses-a-destination-that-leaves-the-project", func(w *registryWorld) {
			w.projectWorld()
			w.mkdir("outside/skills")
			w.put("outside/real.md", "a file outside the project\n")
			scenarios := []struct {
				label string
				setup func()
			}{
				{".claude is a link to a directory outside the project", func() { w.symlink("../outside", "demo/.claude") }},
				{".claude/skills is a link to a directory outside the project", func() { w.mkdir("demo/.claude"); w.symlink("../../outside/skills", "demo/.claude/skills") }},
				{"the directory of a skill is a link to a directory outside the project", func() {
					w.mkdir("demo/.claude/skills")
					w.symlink("../../../outside/skills", "demo/.claude/skills/tidy-notes")
				}},
				{"a file of an installed skill is a link to a file outside the project", func() {
					// The skill must be OURS (in the lock) for install to look at its files; a
					// directory it did not install is refused before any file is read.
					w.installDemo("the skill installed, to be linked away")
					w.remove("demo/.claude/skills/tidy-notes/SKILL.md")
					w.symlink("../../../../outside/real.md", "demo/.claude/skills/tidy-notes/SKILL.md")
				}},
				{".agents is a link to a directory inside the project", func() { w.mkdir("demo/elsewhere"); w.symlink("elsewhere", "demo/.agents") }},
				{".claude is a dangling link", func() { w.symlink("nowhere", "demo/.claude") }},
				{".claude is a link to itself", func() { w.symlink(".claude", "demo/.claude") }},
			}
			for _, sc := range scenarios {
				w.resetDemo()
				sc.setup()
				w.installDemo(sc.label)
				w.state("demo")
				w.state("outside")
			}
		}},
		{"fs-install-refuses-a-destination-that-is-not-the-kind-of-thing-it-needs", func(w *registryWorld) {
			w.projectWorld()
			scenarios := []struct {
				label string
				setup func()
			}{
				{".claude is a file", func() { w.put("demo/.claude", "a file\n") }},
				{".claude/skills is a file", func() { w.put("demo/.claude/skills", "a file\n") }},
				{"the directory of a skill is a file", func() { w.put("demo/.claude/skills/tidy-notes", "a file\n") }},
				{"SKILL.md is a directory", func() { w.mkdir("demo/.claude/skills/tidy-notes/SKILL.md") }},
				{"the project lock is a directory", func() { w.mkdir("demo/.labdrian/procedural-skills.lock.json") }},
				{".labdrian is a file", func() { w.put("demo/.labdrian", "a file\n") }},
			}
			for _, sc := range scenarios {
				w.resetDemo()
				sc.setup()
				w.installDemo(sc.label)
				w.state("demo")
			}
		}},
		{"fs-install-refuses-a-project-it-cannot-write-and-puts-back-what-it-wrote", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.projectWorld()
			scenarios := []struct {
				label string
				setup func()
			}{
				{"the project directory cannot be written to", func() { w.chmod("demo", 0o555) }},
				{".claude cannot be written to", func() { w.mkdir("demo/.claude"); w.chmod("demo/.claude", 0o555) }},
				{".agents cannot be written to, after .claude was", func() { w.mkdir("demo/.agents"); w.chmod("demo/.agents", 0o555) }},
				{"the skill directory of the second target cannot be written to", func() {
					w.mkdir("demo/.agents/skills/tidy-notes")
					w.chmod("demo/.agents/skills/tidy-notes", 0o555)
				}},
				{"the project lock cannot be read", func() {
					w.putMode("demo/.labdrian/procedural-skills.lock.json", "{}\n", 0o000)
				}},
				{"the directory of the project lock cannot be written to", func() {
					w.put("demo/.labdrian/keep", "x\n")
					w.chmod("demo/.labdrian", 0o555)
				}},
			}
			for _, sc := range scenarios {
				w.resetDemo()
				sc.setup()
				w.installDemo(sc.label)
				w.state("demo")
			}
		}},
		{"fs-install-replaces-and-removes-what-the-source-changed", func(w *registryWorld) {
			w.projectWorld()
			src := "overlay/skills/tidy-notes/"
			w.put(src+"references/old.md", "to be removed\n")
			w.put(src+"references/nested/gone.md", "to be removed with its directory\n")
			w.putMode(src+"bin/run.sh", "#!/bin/sh\necho one\n", 0o644)
			w.installDemo("a first install")
			w.state("demo")
			w.removeAll(src + "references/old.md")
			w.removeAll(src + "references/nested")
			w.putMode(src+"bin/run.sh", "#!/bin/sh\necho two\n", 0o755)
			w.put(src+"references/new.md", "added\n")
			w.installDemo("a file removed from the source, a directory with it, a file changed and made executable, a file added")
			w.state("demo")
			w.put("demo/.claude/skills/tidy-notes/references/mine.md", "the user's own file\n")
			w.removeAll(src + "references/new.md")
			w.installDemo("a file the user added beside, and a file removed from the source")
			w.state("demo")
		}},
		{"fs-adopt-reads-the-installed-files-as-they-are", func(w *registryWorld) {
			w.projectWorld()
			w.runIn("demo", w.installArgs("install")...)
			w.put("demo/.claude/skills/tidy-notes/references/extra/deep.md", "an extra file\n")
			w.symlink("SKILL.md", "demo/.claude/skills/tidy-notes/link.md")
			w.fifo("demo/.claude/skills/tidy-notes/pipe")
			w.mkdir("demo/.claude/skills/tidy-notes/empty-dir")
			w.putMode("demo/.claude/skills/tidy-notes/references/notes.md", "A reference file of the skill.\n", 0o600)
			w.label("a project with an installed skill that has grown")
			w.runIn("demo", w.installArgs("adopt")...)
			w.state("demo")
			w.label("and 'install' over the same")
			w.runIn("demo", w.installArgs("install")...)
			w.state("demo")
		}},
		{"fs-adopt-refuses-installed-files-it-cannot-read", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.projectWorld()
			w.runIn("demo", w.installArgs("install")...)
			w.chmod("demo/.claude/skills/tidy-notes/references/notes.md", 0o000)
			w.label("an installed file with no permission bits")
			w.runIn("demo", w.installArgs("adopt")...)
			w.runIn("demo", w.installArgs("install")...)
			w.chmod("demo/.claude/skills/tidy-notes/references", 0o000)
			w.label("an installed directory with none")
			w.runIn("demo", w.installArgs("adopt")...)
			w.runIn("demo", w.installArgs("install")...)
		}},
		{"fs-project-register-revise-and-retire-leave-the-files-they-wrote", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.label("registered")
			w.run(w.registerArgs()...)
			w.state("project")
			w.put("drafts/revised/SKILL.md", strings.Replace(goldenProjectDraft("tidy-worktree"), "handed over clean.", "handed over clean and reviewed.", 1))
			w.put("drafts/revised/references/extra.md", "an extra file\n")
			w.label("revised")
			w.run("skills", "project-revise", "--project-root", w.path("project"), "--candidate", goldenCandidate, "--registry", w.path("overlay/"+worldRegistry), w.path("drafts/revised/SKILL.md"))
			w.state("project")
			w.label("retired")
			w.run("skills", "project-retire", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry), "--reason", "not needed any more", "tidy-worktree")
			w.state("project")
			w.label("status")
			w.run("skills", "project-status", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry))
		}},
		{"fs-project-register-refuses-a-root-and-a-draft-it-cannot-use", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.put("a-file", "not a directory\n")
			w.symlink("project", "link-to-project")
			w.symlink("nowhere", "dangling")
			register := func(label, root, draft string) {
				w.t.Helper()
				w.label("%s", label)
				w.run("skills", "project-register", "--project-root", root, "--candidate", goldenCandidate, "--registry", w.path("overlay/"+worldRegistry), draft)
			}
			good := w.path("drafts/tidy/SKILL.md")
			register("a root that does not exist", w.path("missing"), good)
			register("a root that is a file", w.path("a-file"), good)
			register("a root that is a dangling link", w.path("dangling"), good)
			register("a root that is a link to the project", w.path("link-to-project"), good)
			w.state("project")
			register("a root that is relative", "project", good)
			register("a draft that does not exist", w.path("project"), w.path("drafts/missing/SKILL.md"))
			w.mkdir("drafts/dir/SKILL.md")
			register("a draft that is a directory", w.path("project"), w.path("drafts/dir/SKILL.md"))
			w.symlink("../tidy/SKILL.md", "drafts/link.md")
			register("a draft that is a link", w.path("project"), w.path("drafts/link.md"))
			w.state("project")
		}},
		{"fs-project-register-refuses-a-project-it-cannot-write-or-read", func(w *registryWorld) {
			w.needsAUserWhoCannotReadEverything()
			w.projectRegisterWorld()
			w.chmod("project", 0o555)
			w.label("a project directory that cannot be written to")
			w.run(w.registerArgs()...)
			w.state("project")
			w.chmod("project", 0o755)
			w.putMode("project/.labdrian/procedural-skills.lock.json", "{}\n", 0o000)
			w.label("a lock that cannot be read")
			w.run(w.registerArgs()...)
			w.run("skills", "project-status", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry))
			w.run("skills", "project-retire", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry), "--reason", "r", "tidy-worktree")
			w.state("project")
		}},
		{"fs-project-register-refuses-a-destination-that-leaves-the-project", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.mkdir("outside")
			w.symlink("../outside", "project/.claude")
			w.label(".claude is a link to a directory outside the project")
			w.run(w.registerArgs()...)
			w.state("project")
			w.state("outside")
			w.remove("project/.claude")
			w.mkdir("project/.claude/skills")
			w.symlink("../../../outside", "project/.claude/skills/tidy-worktree")
			w.label("the directory of the skill is a link to one outside")
			w.run(w.registerArgs()...)
			w.state("project")
			w.state("outside")
			w.remove("project/.claude/skills/tidy-worktree")
			w.mkdir("project/.claude/skills/tidy-worktree")
			w.symlink("../../../../outside/x.md", "project/.claude/skills/tidy-worktree/SKILL.md")
			w.label("SKILL.md of a directory already there is a link to a file outside (refused as a foreign skill: the program never looks at the link)")
			w.run(w.registerArgs()...)
			w.state("project")
			w.state("outside")
		}},
	}
}
