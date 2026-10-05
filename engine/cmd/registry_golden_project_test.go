package main

import (
	"os"
	"path/filepath"
	"strings"
)

// The cases of the registry goldens that are about a project: 'install' and 'adopt', which read
// the registry to know which skills are admitted for the project they run in, and the verbs of
// the project tier ('project-register', 'project-status', 'project-retire'), which read it to
// know which ids the overlay already has. 'project-revise' accepts a registry and does not read
// it, and that is pinned too: a registry it must not refuse.

// projectOverlayYAML is a registry of two skills admitted to a project: one for the projects
// demo and other, one for demo alone; and one that is global, which a project never takes.
const projectOverlayYAML = `version: "1"
skills:
  - id: tidy-notes
    path: tidy-notes
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - demo
        - other
      targets:
        - claude
        - pi
    lifecycle:
      updateStrategy: overlay-only
  - id: only-demo
    path: only-demo
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - demo
      targets:
        - codex
    lifecycle:
      updateStrategy: overlay-only
  - id: everywhere
    path: everywhere
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// projectWorld is a world that holds an overlay (a registry of skills admitted to projects, and the
// source of each) and a project, demo, where the program runs.
func (w *registryWorld) projectWorld() {
	w.t.Helper()
	w.put("overlay/"+worldRegistry, projectOverlayYAML)
	w.putSkill("tidy-notes", false)
	w.putSkill("only-demo", false)
	w.putSkill("everywhere", true)
	// putSkill writes under skills/: the overlay's source root is overlay/skills.
	for _, id := range []string{"tidy-notes", "only-demo", "everywhere"} {
		w.move("skills/"+id, "overlay/skills/"+id)
	}
	w.put("overlay/skills/tidy-notes/references/notes.md", "A reference file of the skill.\n")
	w.mkdir("demo")
}

// installArgs is the command line of 'install' or 'adopt' over the overlay of projectWorld.
func (w *registryWorld) installArgs(verb string, extra ...string) []string {
	return append([]string{"skills", verb, "--registry", w.path("overlay/" + worldRegistry), "--source-root", w.path("overlay/skills")}, extra...)
}

func registryProjectCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"install-copies-the-skills-the-registry-admits-to-the-project", func(w *registryWorld) {
			w.projectWorld()
			w.label("the project is the directory it runs in, named demo")
			w.runIn("demo", w.installArgs("install")...)
			w.tree("demo")
			w.show("demo/.labdrian/procedural-skills.lock.json")
			w.label("the same again changes nothing")
			w.runIn("demo", w.installArgs("install")...)
		}},
		{"install-names-the-project-by-its-id-or-by-the-directory", func(w *registryWorld) {
			w.projectWorld()
			w.label("a project id that the registry names for one skill only")
			w.runIn("demo", w.installArgs("install", "--project-id", "other")...)
			w.tree("demo")
			w.mkdir("nowhere")
			w.label("a directory that is not a project the registry admits a skill to")
			w.runIn("nowhere", w.installArgs("install")...)
			w.label("the same directory under the project id that is")
			w.runIn("nowhere", w.installArgs("install", "--project-id", "demo")...)
			w.tree("nowhere")
		}},
		{"install-refuses-what-it-cannot-install", func(w *registryWorld) {
			w.projectWorld()
			w.label("no source root")
			w.runIn("demo", "skills", "install", "--registry", w.path("overlay/"+worldRegistry))
			w.label("a source root that has not the skills")
			w.runIn("demo", "skills", "install", "--registry", w.path("overlay/"+worldRegistry), "--source-root", w.path("empty"))
			w.tree("demo")
		}},
		{"install-and-adopt-refuse-an-unusable-registry", func(w *registryWorld) {
			w.projectWorld()
			for _, verb := range []string{"install", "adopt"} {
				for i, doc := range registryStates() {
					path := w.path("refused/" + verb + string(rune('a'+i)) + ".yaml")
					if i > 0 {
						w.put("refused/"+verb+string(rune('a'+i))+".yaml", doc.text)
					}
					w.label("%s: %s", verb, doc.label)
					w.runIn("demo", "skills", verb, "--registry", path, "--source-root", w.path("overlay/skills"))
				}
			}
			w.tree("demo")
		}},
		{"adopt-records-what-is-already-in-the-project", func(w *registryWorld) {
			w.projectWorld()
			w.runIn("demo", w.installArgs("install")...)
			w.remove("demo/.labdrian/procedural-skills.lock.json")
			w.label("a project whose lock is gone, with the files in place")
			w.runIn("demo", w.installArgs("adopt")...)
			w.show("demo/.labdrian/procedural-skills.lock.json")
			w.label("the same again")
			w.runIn("demo", w.installArgs("adopt")...)
			w.put("demo/.claude/skills/only-demo/SKILL.md", "Edited by someone.\n")
			w.label("a file that is not what the overlay has")
			w.runIn("demo", w.installArgs("adopt")...)
			w.label("and 'install' says the same of it")
			w.runIn("demo", w.installArgs("install")...)
		}},
		{"adopt-says-when-no-skill-is-admitted", func(w *registryWorld) {
			w.projectWorld()
			w.mkdir("nowhere")
			w.runIn("nowhere", w.installArgs("adopt")...)
			w.tree("nowhere")
		}},
		{"project-register-writes-a-draft-into-the-project", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.label("a plan")
			w.run(w.registerArgs("--dry-run")...)
			w.tree("project")
			w.label("a registration")
			w.run(w.registerArgs()...)
			w.tree("project")
			w.show("project/.labdrian/procedural-skills.lock.json")
			w.label("the same again")
			w.run(w.registerArgs()...)
		}},
		{"project-register-refuses-an-id-the-registry-has", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.put("overlay/"+worldRegistry, registryOf("tidy-worktree"))
			w.run(w.registerArgs()...)
			w.tree("project")
		}},
		{"project-register-refuses-an-unusable-registry", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.eachRegistryState(func(registry string) []string { return w.registerArgs("--registry", registry) })
			w.tree("project")
		}},
		{"project-status-lists-the-project-and-what-the-registry-says-of-it", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.run(w.registerArgs()...)
			// A second skill whose id is not the last word of its candidate: the registry can
			// have it under either name.
			w.put("drafts/two/SKILL.md", goldenProjectDraft("tidy-two"))
			w.run("skills", "project-register", "--project-root", w.path("project"), "--candidate", "procedural/candidates/repeated-success/renamed-two", "--registry", w.path("overlay/"+worldRegistry), w.path("drafts/two/SKILL.md"))
			status := func() {
				w.run("skills", "project-status", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry))
			}
			w.label("a registry that has nothing of them")
			status()
			w.label("a registry that has the id of the first: it is superseded")
			w.put("overlay/"+worldRegistry, registryOf("tidy-worktree"))
			status()
			w.label("a registry that has the id of the second: it is superseded")
			w.put("overlay/"+worldRegistry, registryOf("tidy-two"))
			status()
			w.label("a registry that has the last word of the candidate of the second: it is superseded")
			w.put("overlay/"+worldRegistry, registryOf("renamed-two"))
			status()
		}},
		{"project-status-refuses-an-unusable-registry", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.run(w.registerArgs()...)
			w.eachRegistryState(func(registry string) []string {
				return []string{"skills", "project-status", "--project-root", w.path("project"), "--registry", registry}
			})
		}},
		{"project-retire-retires-a-registered-skill", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.run(w.registerArgs()...)
			w.label("a plan")
			w.run("skills", "project-retire", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry), "--reason", "not needed any more", "--dry-run", "tidy-worktree")
			w.label("a retirement")
			w.run("skills", "project-retire", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry), "--reason", "not needed any more", "tidy-worktree")
			w.tree("project")
			w.show("project/.labdrian/procedural-skills.lock.json")
		}},
		{"project-retire-refuses-an-unusable-registry", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.run(w.registerArgs()...)
			w.eachRegistryState(func(registry string) []string {
				return []string{"skills", "project-retire", "--project-root", w.path("project"), "--registry", registry, "--reason", "r", "tidy-worktree"}
			})
		}},
		{"project-revise-accepts-a-registry-it-does-not-read", func(w *registryWorld) {
			w.projectRegisterWorld()
			w.run(w.registerArgs()...)
			w.put("drafts/revised/SKILL.md", strings.Replace(goldenProjectDraft("tidy-worktree"), "handed over clean.", "handed over clean and reviewed.", 1))
			for i, doc := range registryStates() {
				path := w.path("refused/r" + string(rune('a'+i)) + ".yaml")
				if i > 0 {
					w.put("refused/r"+string(rune('a'+i))+".yaml", doc.text)
				}
				w.label("revising with: %s", doc.label)
				w.run("skills", "project-revise", "--project-root", w.path("project"), "--candidate", goldenCandidate, "--registry", path, "--dry-run", w.path("drafts/revised/SKILL.md"))
			}
		}},
	}
}

// goldenCandidate is the key of the procedural candidate the draft is registered for.
const goldenCandidate = "procedural/candidates/repeated-success/tidy-worktree"

// projectDraft is a draft skill that passes what project-register asks of one.
func goldenProjectDraft(name string) string {
	return "---\n" +
		"name: " + name + "\n" +
		"description: Tidy a git worktree before handing it to a reviewer.\n" +
		"license: Apache-2.0\n" +
		"metadata:\n" +
		"  author: someone\n" +
		"  version: 1.0.0\n" +
		"---\n" +
		"\n" +
		"## Activation Contract\n" +
		"\n" +
		"Use when a worktree must be handed over clean.\n"
}

// projectRegisterWorld is a world with an overlay (a registry that has nothing of the draft), a
// project to register into, and the draft outside the project.
func (w *registryWorld) projectRegisterWorld() {
	w.t.Helper()
	w.put("overlay/"+worldRegistry, registryOf("unrelated"))
	w.mkdir("project")
	w.put("drafts/tidy/SKILL.md", goldenProjectDraft("tidy-worktree"))
}

// registerArgs is the command line of 'project-register' for the draft of projectRegisterWorld,
// with extra flags before the draft; a later --registry in extra wins.
func (w *registryWorld) registerArgs(extra ...string) []string {
	args := []string{"skills", "project-register", "--project-root", w.path("project"), "--candidate", goldenCandidate, "--registry", w.path("overlay/" + worldRegistry)}
	return append(append(args, extra...), w.path("drafts/tidy/SKILL.md"))
}

// move renames a path of the world.
func (w *registryWorld) move(from, to string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.path(to)), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.Rename(w.path(from), w.path(to)); err != nil {
		w.t.Fatal(err)
	}
}

// remove deletes a file of the world.
func (w *registryWorld) remove(rel string) {
	w.t.Helper()
	if err := os.Remove(w.path(rel)); err != nil {
		w.t.Fatal(err)
	}
}
