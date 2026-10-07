package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden files under testdata/project-identity-golden record how the verbs of `skills` that
// depend on the id of the project name it: 'install' and 'adopt', which admit the skills of a
// registry to a project by the project's id, and say that id in what they print. The id is, in
// the owner's order (Phase 9, decision Q8): what --project-id gives, then the origin remote of the
// repository the program runs in (read from its .git/config without running git), then the name of
// that directory. A repository whose origin is github.com/acme/demo is that project whatever its
// directory is called, also from a subdirectory or a linked worktree. These files were first
// recorded from the program as it was before Phase 9 unit H18 (docs/architecture/
// hexagonal-target.md) put the id behind a ProjectIdentity port, which named the project by its
// directory; the owner then put the origin in the chain (2026-10-06), and the cases that changed
// were renamed for what they pin now.
//
// Rewrite them deliberately with
//
//	go test ./cmd -run TestProjectIdentityGolden -update-project-identity-golden
//
// and read the diff before committing it. The verbs of the project tier ('project-register',
// 'project-status', 'project-retire', 'project-revise') take the project by --project-root and
// read no id; one case pins that they do not, so that the day they do (the declaration verb of
// C21) is a change that shows.
var updateProjectIdentityGolden = flag.Bool("update-project-identity-golden", false, "rewrite the golden files of the project identity")

// identityOverlayYAML is a registry of three skills, each admitted to a different spelling of the
// project: the name of the directory, the identity of the origin remote of a repository, and an id
// that is neither.
const identityOverlayYAML = `version: "1"
skills:
  - id: by-directory
    path: by-directory
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - demo
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: by-origin
    path: by-origin
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - github.com/acme/demo
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: by-choice
    path: by-choice
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - chosen
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// identityWorld is a world with the overlay of identityOverlayYAML and nothing else.
func (w *registryWorld) identityWorld() {
	w.t.Helper()
	w.put("overlay/"+worldRegistry, identityOverlayYAML)
	for _, id := range []string{"by-directory", "by-origin", "by-choice"} {
		w.putSkill(id, false)
		w.move("skills/"+id, "overlay/skills/"+id)
	}
}

// repository makes dir a git repository the way git leaves one, by hand: no git runs. origin is the
// url of its origin remote, or "" for a repository with none.
func (w *registryWorld) repository(dir, origin string) {
	w.t.Helper()
	w.put(dir+"/.git/HEAD", "ref: refs/heads/main\n")
	config := "[core]\n\trepositoryformatversion = 0\n"
	if origin != "" {
		config += "[remote \"origin\"]\n\turl = " + origin + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	}
	w.put(dir+"/.git/config", config)
}

// linkedWorktree makes dir a linked worktree of the repository at main, the way git leaves one.
func (w *registryWorld) linkedWorktree(main, dir, name string) {
	w.t.Helper()
	w.put(main+"/.git/worktrees/"+name+"/commondir", "../..\n")
	w.put(main+"/.git/worktrees/"+name+"/HEAD", "ref: refs/heads/"+name+"\n")
	w.put(dir+"/.git", "gitdir: "+w.path(main+"/.git/worktrees/"+name)+"\n")
}

func (w *registryWorld) identityArgs(verb string, extra ...string) []string {
	return append([]string{"skills", verb, "--registry", w.path("overlay/" + worldRegistry), "--source-root", w.path("overlay/skills")}, extra...)
}

func projectIdentityCases() []registryGoldenCase {
	const origin = "git@github.com:acme/demo.git"
	return []registryGoldenCase{
		{"install-names-the-project-by-its-origin-not-its-directory", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", origin)
			w.label("a repository named demo whose origin is github.com/acme/demo: the skill admitted to the origin")
			w.runIn("demo", w.identityArgs("install")...)
			w.tree("demo")
			w.show("demo/.labdrian/procedural-skills.lock.json")
		}},
		{"install-names-the-project-by-its-directory-in-a-repository-with-no-origin", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", "")
			w.runIn("demo", w.identityArgs("install")...)
			w.tree("demo")
		}},
		{"install-names-the-project-by-its-directory-in-a-directory-that-is-no-repository", func(w *registryWorld) {
			w.identityWorld()
			w.mkdir("demo")
			w.runIn("demo", w.identityArgs("install")...)
			w.tree("demo")
		}},
		{"install-in-a-renamed-checkout-is-the-project-its-origin-names", func(w *registryWorld) {
			w.identityWorld()
			w.repository("renamed-checkout", origin)
			w.label("the origin is github.com/acme/demo, the directory renamed-checkout: the project is the origin, whatever the directory is called")
			w.runIn("renamed-checkout", w.identityArgs("install")...)
			w.tree("renamed-checkout")
		}},
		{"install-below-the-root-of-a-repository-names-the-project-by-its-origin", func(w *registryWorld) {
			w.identityWorld()
			w.repository("monorepo", origin)
			w.mkdir("monorepo/demo")
			w.label("a directory below the root of a repository is the project its origin names, and installs into itself")
			w.runIn("monorepo/demo", w.identityArgs("install")...)
			w.tree("monorepo")
		}},
		{"install-in-a-linked-worktree-names-the-project-by-its-origin", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", origin)
			w.linkedWorktree("demo", "feature-x", "feature-x")
			w.label("a linked worktree of demo is the project its origin names")
			w.runIn("feature-x", w.identityArgs("install")...)
			w.treeOf("feature-x", false) // the .git file holds a path of the world, whose length varies
			w.label("and with --project-id demo it is the project named demo")
			w.runIn("feature-x", w.identityArgs("install", "--project-id", "demo")...)
			w.treeOf("feature-x", false) // the .git file holds a path of the world, whose length varies
		}},
		{"install-takes-an-explicit-project-id-over-the-directory-and-the-origin", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", origin)
			w.label("an id the registry admits a skill to, in a repository named demo")
			w.runIn("demo", w.identityArgs("install", "--project-id", "chosen")...)
			w.tree("demo")
			w.mkdir("elsewhere")
			w.label("the identity of the origin, given as the id, in a directory named elsewhere")
			w.runIn("elsewhere", w.identityArgs("install", "--project-id", "github.com/acme/demo")...)
			w.tree("elsewhere")
			w.label("an id nothing is admitted to, in the directory that is admitted")
			w.runIn("demo", w.identityArgs("install", "--project-id", "stranger")...)
		}},
		{"install-falls-back-to-the-origin-when-the-project-id-has-no-value", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", origin)
			w.label("--project-id as the last word")
			w.runIn("demo", w.identityArgs("install", "--project-id")...)
			w.label("--project-id with an empty value")
			w.runIn("demo", w.identityArgs("install", "--project-id", "")...)
			w.tree("demo")
		}},
		{"adopt-names-the-project-as-install-does", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", origin)
			w.runIn("demo", w.identityArgs("install")...)
			w.remove("demo/.labdrian/procedural-skills.lock.json")
			w.label("a repository named demo with its files in place and no lock")
			w.runIn("demo", w.identityArgs("adopt")...)
			w.show("demo/.labdrian/procedural-skills.lock.json")
			w.label("adopt with the id of a project the files do not belong to")
			w.runIn("demo", w.identityArgs("adopt", "--project-id", "chosen")...)
			w.label("a renamed checkout of the same origin is the same project: adopt asks for its skill, which is not installed there")
			w.repository("renamed-checkout", origin)
			w.runIn("renamed-checkout", w.identityArgs("adopt")...)
		}},
		{"install-and-adopt-say-the-project-id-in-what-they-tell-to-run", func(w *registryWorld) {
			w.identityWorld()
			w.repository("demo", origin)
			w.label("adopt with nothing installed names the id it would install under")
			w.runIn("demo", w.identityArgs("adopt")...)
			w.runIn("demo", w.identityArgs("adopt", "--project-id", "chosen")...)
			w.label("install over a directory it did not install names the id it would adopt under")
			w.put("demo/.claude/skills/by-directory/SKILL.md", w.read("overlay/skills/by-directory/SKILL.md"))
			w.put("demo/.agents/skills/by-directory/SKILL.md", "Not the skill.\n")
			w.runIn("demo", w.identityArgs("install")...)
			w.label("adopt of a skill that is in one runtime only notes where it is not, under the id")
			w.removeAll("demo/.agents/skills/by-directory")
			w.runIn("demo", w.identityArgs("adopt")...)
			w.show("demo/.labdrian/procedural-skills.lock.json")
			w.label("adopt of a skill installed and then changed in the source names the id to install under")
			w.put("overlay/skills/by-directory/SKILL.md", w.read("overlay/skills/by-directory/SKILL.md")+"\nA new line in the source.\n")
			w.runIn("demo", w.identityArgs("adopt")...)
			w.runIn("demo", w.identityArgs("adopt", "--project-id", "demo")...)
		}},
		{"install-refuses-a-git-file-that-is-no-pointer", func(w *registryWorld) {
			w.identityWorld()
			w.put("demo/.git", "this is not a git file\n")
			w.label("a .git that is a file and holds no gitdir line: the repository cannot be read, and the directory name is not used in its place")
			w.runIn("demo", w.identityArgs("install")...)
			w.runIn("demo", w.identityArgs("adopt")...)
			w.tree("demo")
			w.label("naming the project goes past it")
			w.runIn("demo", w.identityArgs("install", "--project-id", "demo")...)
		}},
		{"install-says-how-to-go-on-when-the-repository-cannot-be-looked-for", func(w *registryWorld) {
			w.identityWorld()
			w.mkdir("demo")
			w.symlink(".git", "demo/.git")
			w.label("a .git that cannot be inspected (a link to itself): the directory is named, and --project-id is the way on")
			w.runIn("demo", w.identityArgs("install")...)
			w.tree("demo")
			w.label("naming the project goes past it")
			w.runIn("demo", w.identityArgs("install", "--project-id", "demo")...)
		}},
		{"project-verbs-read-no-project-id", func(w *registryWorld) {
			w.put("overlay/"+worldRegistry, registryOf("unrelated"))
			w.repository("project", origin)
			w.put("drafts/tidy/SKILL.md", goldenProjectDraft("tidy-worktree"))
			w.label("a repository with an origin: the lock records the draft and no id of the project")
			w.run(w.registerArgs()...)
			w.show("project/.labdrian/procedural-skills.lock.json")
			w.run("skills", "project-status", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry))
			w.put("drafts/revised/SKILL.md", strings.Replace(goldenProjectDraft("tidy-worktree"), "handed over clean.", "handed over clean and reviewed.", 1))
			w.run("skills", "project-revise", "--project-root", w.path("project"), "--candidate", goldenCandidate, "--registry", w.path("overlay/"+worldRegistry), w.path("drafts/revised/SKILL.md"))
			w.run("skills", "project-retire", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry), "--reason", "not needed any more", "tidy-worktree")
			w.show("project/.labdrian/procedural-skills.lock.json")
		}},
	}
}

// TestProjectIdentityGolden runs every case and compares its transcript with its golden file.
func TestProjectIdentityGolden(t *testing.T) {
	for _, tc := range projectIdentityCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newRegistryWorld(t)
			tc.run(w)
			checkGoldenIn(t, "project-identity-golden", tc.name, w.text(), updateProjectIdentityGolden, "-update-project-identity-golden")
		})
	}
}

// Two cases of one name would share a golden file and each pass against the other's recording, and
// a file no case owns is a recording nothing checks.
func TestProjectIdentityGoldenCasesAreDistinctFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range projectIdentityCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "project-identity-golden"))
	if err != nil {
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/project-identity-golden/%s belongs to no case", e.Name())
		}
	}
	if len(entries) != len(seen) {
		t.Errorf("%d golden files for %d cases", len(entries), len(seen))
	}
}
