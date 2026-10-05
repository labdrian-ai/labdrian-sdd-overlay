package main

import (
	"os/exec"
	"regexp"
	"strings"
)

// The cases of the registry goldens that are about the package of the Pi runtime, which 'pipkg
// build' writes from the registry (one skill of the package for each entry whose targets include
// pi) and 'pipkg check' compares against what the overlay would build now. The same code serves
// the Pi runtime verbs ('runtime install --target pi', 'runtime sync-check --target pi'), whose
// refusal of an unusable registry is pinned here too.
//
// 'pipkg check' reads the registry of the working tree, or, when the overlay is a git repository
// that has built before, the registry of the deploy ref (main), exported to a directory: the two
// ways the program reaches a registry, a file and a directory, are both pinned.

// packageRegistryYAML is a registry of three skills, two of them for Pi.
const packageRegistryYAML = `version: "1"
skills:
  - id: alpha
    path: alpha
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
        - pi
    lifecycle:
      updateStrategy: overlay-only
  - id: beta
    path: beta
    source:
      type: core
      upstream:
        owner: gentleman-programming
    install:
      defaultScope: global
      targets:
        - pi
    lifecycle:
      updateStrategy: vendor-merge
  - id: gamma
    path: gamma
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// packageWorld is a world that holds an overlay of three skills and the agent the package ships,
// and the place the package is built.
func (w *registryWorld) packageWorld() {
	w.t.Helper()
	w.put("overlay/"+worldRegistry, packageRegistryYAML)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		w.put("overlay/skills/"+id+"/SKILL.md", skillFile(id))
	}
	w.put("overlay/skills/alpha/references/notes.md", "A reference file.\n")
	w.put("overlay/agents/GADU.md", "---\nname: GADU\n---\nThe agent.\n")
	w.put("overlay/skills/_shared/minimalism-contract.md", "A contract.\n")
	w.put("overlay/skills/_shared/anti-generic-design.md", "Another contract.\n")
	w.filter = maskCommits
}

// pipkgArgs is the command line of 'pipkg <verb>' over the overlay of packageWorld.
func (w *registryWorld) pipkgArgs(verb string) []string {
	return w.pipkgArgsFor(verb, w.path("overlay/"+worldRegistry))
}

func (w *registryWorld) pipkgArgsFor(verb, registry string) []string {
	return []string{"pipkg", verb, "--overlay-root", w.path("overlay"), "--registry", registry, "--dest-dir", w.path("out/labdrian-pi")}
}

// commitOverlay makes the overlay a git repository whose main holds what is in it, with the
// dates and names of the commit fixed, so what git reports of it is the same on every run.
func (w *registryWorld) commitOverlay() {
	w.t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", "overlay"},
	} {
		cmd := exec.Command("git", append([]string{"-C", w.path("overlay")}, args...)...)
		cmd.Env = append(goldenEnvironment(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test",
			"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			w.t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// commitDigests are the digests of commits a transcript may carry: a commit's own (40
// characters) and the short form the disclosure prints (12). A world that commits sets them
// from fixed dates, but the text does not depend on that being stable on every git.
var (
	commitSHA40 = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	commitSHA12 = regexp.MustCompile(`\b[0-9a-f]{12}\b`)
)

func maskCommits(text string) string {
	return commitSHA12.ReplaceAllString(commitSHA40.ReplaceAllString(text, "<SHA>"), "<SHA12>")
}

// treeNames records the names of the files under rel, in order, for a tree that holds a file
// whose size is not what a case is about (an asset the program embeds).
func (w *registryWorld) treeNames(rel string) { w.t.Helper(); w.treeOf(rel, false) }

func registryPackageCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"pipkg-build-writes-the-package-of-the-pi-skills", func(w *registryWorld) {
			w.packageWorld()
			w.label("a build: the skills for Pi, and the agent")
			w.run(w.pipkgArgs("build")...)
			w.treeNames("out")
			w.show("out/labdrian-pi/package.json")
			w.label("a build over a build replaces it")
			w.run(w.pipkgArgs("build")...)
			w.treeNames("out")
		}},
		{"pipkg-check-says-whether-the-package-is-the-current-one", func(w *registryWorld) {
			w.packageWorld()
			w.label("nothing built yet")
			w.run(w.pipkgArgs("check")...)
			w.run(w.pipkgArgs("build")...)
			w.label("right after a build")
			w.run(w.pipkgArgs("check")...)
			w.put("overlay/skills/alpha/SKILL.md", skillFile("alpha")+"\nan edit\n")
			w.label("a skill edited since")
			w.run(w.pipkgArgs("check")...)
			w.put("overlay/skills/alpha/SKILL.md", skillFile("alpha"))
			w.put("overlay/"+worldRegistry, strings.Replace(packageRegistryYAML, "        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n", "        - claude\n        - pi\n    lifecycle:\n      updateStrategy: overlay-only\n", 2))
			w.label("a skill that is for Pi since")
			w.run(w.pipkgArgs("check")...)
		}},
		{"pipkg-refuses-what-it-cannot-build", func(w *registryWorld) {
			w.packageWorld()
			w.label("no verb, an unknown verb, an unknown option, a flag with no value, a flag missing")
			w.run("pipkg")
			w.run("pipkg", "pack")
			w.run("pipkg", "build", "--shape", "x")
			w.run("pipkg", "build", "--registry")
			w.run("pipkg", "build", "--registry", w.path("overlay/"+worldRegistry))
			w.label("a destination that is the overlay")
			w.run("pipkg", "build", "--overlay-root", w.path("overlay"), "--registry", w.path("overlay/"+worldRegistry), "--dest-dir", w.path("overlay"))
			w.remove("overlay/skills/beta/SKILL.md")
			w.label("a skill of the registry that has no SKILL.md")
			w.run(w.pipkgArgs("build")...)
			w.put("overlay/skills/beta/SKILL.md", strings.Replace(skillFile("beta"), "name: beta", "name: other", 1))
			w.label("a skill that is named otherwise than its directory")
			w.run(w.pipkgArgs("build")...)
			w.treeNames("out")
		}},
		{"pipkg-refuses-an-unusable-registry", func(w *registryWorld) {
			w.packageWorld()
			w.run(w.pipkgArgs("build")...)
			for _, verb := range []string{"build", "check"} {
				for i, doc := range registryStates() {
					path := w.path("refused/" + verb + string(rune('a'+i)) + ".yaml")
					if i > 0 {
						w.put("refused/"+verb+string(rune('a'+i))+".yaml", doc.text)
					}
					w.label("%s: %s", verb, doc.label)
					w.run(w.pipkgArgsFor(verb, path)...)
				}
			}
			w.label("and the package that was built is as it was")
			w.treeNames("out")
		}},
		{"pipkg-check-reads-the-registry-of-the-deploy-ref-when-it-compares-with-it", func(w *registryWorld) {
			w.packageWorld()
			w.commitOverlay()
			w.label("a committed overlay, built, and checked")
			w.run(w.pipkgArgs("build")...)
			w.run(w.pipkgArgs("check")...)
			w.put("overlay/"+worldRegistry, "version: \"1\"\nskills:\n  - id: broken\n")
			w.label("the registry of the working tree broken since: the comparison is with the committed one")
			w.run(w.pipkgArgs("check")...)
			w.label("a build from that working tree is not")
			w.run(w.pipkgArgs("build")...)
		}},
		{"pipkg-check-refuses-a-registry-the-deploy-ref-has-that-is-unusable", func(w *registryWorld) {
			w.packageWorld()
			w.put("overlay/"+worldRegistry, strings.Replace(packageRegistryYAML, "- pi\n", "- vim\n", 1))
			w.commitOverlay()
			w.label("the committed registry has a target that does not exist; the package was built from another")
			w.put("out/labdrian-pi/package.json", "{}\n")
			w.run(w.pipkgArgs("check")...)
		}},
		{"runtime-pi-refuses-an-unusable-registry", func(w *registryWorld) {
			w.packageWorld()
			w.mkdir("state")
			w.setenv("OVERLAY_DIR", w.path("overlay"))
			w.setenv("STATE_DIR", w.path("state"))
			w.label("a package built where the runtime looks for it")
			w.run("pipkg", "build", "--overlay-root", w.path("overlay"), "--registry", w.path("overlay/"+worldRegistry), "--dest-dir", w.path("state/pi/labdrian-pi"))
			for i, doc := range registryStates() {
				if i > 0 {
					w.put("overlay/"+worldRegistry, doc.text)
				} else {
					w.remove("overlay/" + worldRegistry)
				}
				w.label("install over: %s", doc.label)
				w.run("runtime", "install", "--target", "pi")
				w.label("status over: %s", doc.label)
				w.run("runtime", "status", "--target", "pi")
			}
		}},
	}
}
