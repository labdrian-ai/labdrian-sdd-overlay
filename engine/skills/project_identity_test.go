package skills

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeIdentity is a ProjectIdentity that answers what the test says and records what it was asked.
type fakeIdentity struct {
	id    ProjectID
	ok    bool
	err   error
	asked []ProjectQuery
}

func (f *fakeIdentity) Identify(q ProjectQuery) (ProjectID, bool, error) {
	f.asked = append(f.asked, q)
	return f.id, f.ok, f.err
}

// identityRun runs install or adopt in a project directory named "demo" with the identity the
// test gives, over a registry that admits one skill to the project "named-by-the-port" only.
func identityRun(t *testing.T, verb string, identity ProjectIdentity, args ...string) (code int, stdout, stderr string, project string) {
	t.Helper()
	overlay := t.TempDir()
	project = filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	makeSourceSkill(t, overlay, "the-skill", map[string]string{"SKILL.md": "S"})
	regYAML := strings.ReplaceAll(strings.ReplaceAll(minimalProjectRegistry, "SKILL", "the-skill"), "target-repo", "named-by-the-port")
	deps := Deps{ReadFile: os.ReadFile, Registries: testRegistries(func(string) ([]byte, error) { return []byte(regYAML), nil }), Tree: testTree(), Project: testProjectFS(), Identity: identity}
	var out, errBuf bytes.Buffer
	code = -1
	all := append([]string{"--registry", "reg.yaml", "--source-root", overlay}, args...)
	if verb == "adopt" {
		renderAdopt(installEnvOf(deps, installCwdFn(project)), all, &out, &errBuf, func(c int) { code = c })
	} else {
		RenderInstallCore(all, deps, installCwdFn(project), &out, &errBuf, func(c int) { code = c })
	}
	return code, out.String(), errBuf.String(), project
}

// The domain asks the port once, with the directory it works in and the id it was given, and plans
// for the id the port answers: it derives nothing itself.
func TestInstallAsksTheIdentityPortForTheProjectAndPlansForItsAnswer(t *testing.T) {
	port := &fakeIdentity{id: "named-by-the-port", ok: true}
	code, stdout, stderr, project := identityRun(t, "install", port)

	if code != 0 || stdout != "installed: the-skill\n" {
		t.Fatalf("install = exit %d, stdout %q, stderr %q, want the skill admitted to the id the port named", code, stdout, stderr)
	}
	if want := []ProjectQuery{{Dir: project}}; len(port.asked) != 1 || port.asked[0] != want[0] {
		t.Errorf("the port was asked %+v, want once, %+v", port.asked, want)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "the-skill", "SKILL.md")); err != nil {
		t.Errorf("the skill was not installed: %v", err)
	}
}

func TestInstallPassesTheExplicitProjectIdToThePortAndLeavesTheAnswerToIt(t *testing.T) {
	port := &fakeIdentity{id: "named-by-the-port", ok: true}
	_, _, _, project := identityRun(t, "install", port, "--project-id", "given")

	if want := (ProjectQuery{Dir: project, Explicit: "given"}); len(port.asked) != 1 || port.asked[0] != want {
		t.Errorf("the port was asked %+v, want once, %+v", port.asked, want)
	}
}

func TestAdoptAsksTheSamePortTheSameWay(t *testing.T) {
	port := &fakeIdentity{id: "named-by-the-port", ok: true}
	_, _, stderr, project := identityRun(t, "adopt", port, "--project-id", "given")

	if want := (ProjectQuery{Dir: project, Explicit: "given"}); len(port.asked) != 1 || port.asked[0] != want {
		t.Errorf("the port was asked %+v, want once, %+v (stderr %q)", port.asked, want, stderr)
	}
	if !strings.Contains(stderr, "run `labdrian skills install --project-id named-by-the-port`") {
		t.Errorf("stderr %q does not name the id the port answered in what it tells to run", stderr)
	}
}

func TestInstallRefusesWhenNoSourceNamesTheProject(t *testing.T) {
	port := &fakeIdentity{}
	code, stdout, stderr, project := identityRun(t, "install", port)

	want := "error: skills install: no source of project identity could name the project in " + project + "; give --project-id\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Errorf("install = exit %d, stdout %q, stderr %q, want exit 1 and %q", code, stdout, stderr, want)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); err == nil {
		t.Error("install wrote into a project it could not name")
	}
}

func TestInstallRefusesWithTheReasonWhenTheIdentityCannotBeTold(t *testing.T) {
	port := &fakeIdentity{err: errors.New("config is unreadable")}
	code, stdout, stderr, project := identityRun(t, "install", port)

	want := "error: skills install: resolving project identity: config is unreadable\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Errorf("install = exit %d, stdout %q, stderr %q, want exit 1 and %q", code, stdout, stderr, want)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); err == nil {
		t.Error("install wrote into a project it could not name")
	}
}

// A composition root that wired no identity has forgotten something, and a verb says so instead of
// naming the project by a rule of its own.
func TestInstallRefusesWhenNoProjectIdentityIsWired(t *testing.T) {
	code, stdout, stderr, project := identityRun(t, "adopt", nil)

	want := "error: skills adopt: no project identity is wired\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Errorf("adopt = exit %d, stdout %q, stderr %q, want exit 1 and %q", code, stdout, stderr, want)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); err == nil {
		t.Error("adopt wrote into a project it could not name")
	}
}
