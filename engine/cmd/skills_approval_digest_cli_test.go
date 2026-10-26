package main

// Tests of what the approval of a skill covers, through the verbs: `skills approve` binds its record
// to every file of the skill's directory, and `skills validate` and `skills add` hold the skill to
// it. What the digest is made of is tested in engine/skills, and the walk of the directory in
// engine/skills/skillsfs.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// approvalWorld is an overlay of one global skill, alpha, whose directory holds more than its
// SKILL.md: a script beside it and a file in a subdirectory. The registry, the manifest and the tree
// agree and nothing is approved yet, so what `skills validate` has to say is about approvals alone.
type approvalWorld struct{ *validateWorld }

const (
	alphaScript = "skills/alpha/validate.cjs"
	alphaHelper = "skills/alpha/lib/helper.sh"
)

func newApprovalWorld(t *testing.T) approvalWorld {
	t.Helper()
	w := &validateWorld{t: t, dir: t.TempDir()}
	w.writeRegistryOf("alpha")
	w.put("skills/alpha/SKILL.md", skillFile("alpha"))
	w.put(alphaScript, "console.log(\"v1\");\n")
	w.put(alphaHelper, "echo helper\n")
	w.args = []string{"validate", "--registry", w.path("skills.registry.yaml"), "--manifest", w.path("overlay.manifest"), "--source-root", w.path("skills")}
	aw := approvalWorld{w}
	aw.manifestOf("alpha/SKILL.md", "alpha/validate.cjs", "alpha/lib/helper.sh")
	return aw
}

// manifestOf writes a manifest that lists exactly the given paths.
func (w approvalWorld) manifestOf(paths ...string) {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(p + " custom\n")
	}
	w.put("overlay.manifest", b.String())
}

func (w approvalWorld) approve() verbRun {
	w.t.Helper()
	deps := w.deps(&recordingLocker{})
	deps.Project = skillsfs.Project{}
	deps.Now = overlayClock
	return runSkillsVerb(skillsApprove, deps, "approve", "--id", "alpha", "--approver", "reviewer", "--source-root", w.path("skills"), "--registry", w.path("skills.registry.yaml"))
}

func (w approvalWorld) validate() verbRun {
	w.t.Helper()
	return runSkillsVerb(skillsValidate, w.deps(&recordingLocker{}), w.args...)
}

// approved approves alpha as it is now and fails the test if the approval is refused.
func (w approvalWorld) approved() {
	w.t.Helper()
	if r := w.approve(); r.code() != 0 {
		w.t.Fatalf("approve alpha: exit %d, stderr %q", r.code(), r.stderr)
	}
	if r := w.validate(); r.code() != 0 {
		w.t.Fatalf("validate right after the approval: exit %d, stderr %q", r.code(), r.stderr)
	}
}

func (w approvalWorld) record() string {
	w.t.Helper()
	data, err := os.ReadFile(w.path("skills/alpha/" + skills.ApprovalRecordName))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

// recordOf is an approval record written by hand, in the shape the program writes one.
func recordOf(version int, skill, digest string) string {
	return fmt.Sprintf("{\n  \"version\": %d,\n  \"skill\": %q,\n  \"sha256\": %q,\n  \"approved_at\": %q,\n  \"approver\": \"by hand\"\n}\n", version, skill, digest, overlayApprovedAt)
}

// ---- S1: the approval covers every file of the skill -------------------------------------------

func TestAnApprovalStopsHoldingWhenAnyFileOfTheSkillChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(w approvalWorld)
	}{
		{"a script beside SKILL.md is edited", func(w approvalWorld) { w.put(alphaScript, "console.log(\"v2\");\n") }},
		{"a file in a subdirectory is edited", func(w approvalWorld) { w.put(alphaHelper, "echo changed\n") }},
		{"a file is added", func(w approvalWorld) {
			w.put("skills/alpha/extra.txt", "new\n")
			w.manifestOf("alpha/SKILL.md", "alpha/validate.cjs", "alpha/lib/helper.sh", "alpha/extra.txt")
		}},
		{"a file is removed", func(w approvalWorld) {
			if err := os.Remove(w.path(alphaHelper)); err != nil {
				w.t.Fatal(err)
			}
			w.manifestOf("alpha/SKILL.md", "alpha/validate.cjs")
		}},
		{"a file is renamed", func(w approvalWorld) {
			if err := os.Rename(w.path(alphaScript), w.path("skills/alpha/check.cjs")); err != nil {
				w.t.Fatal(err)
			}
			w.manifestOf("alpha/SKILL.md", "alpha/check.cjs", "alpha/lib/helper.sh")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newApprovalWorld(t)
			w.approved()
			tc.change(w)
			got := w.validate()
			if got.code() != 1 || !strings.Contains(got.stderr, "[APPROVAL_STALE] alpha:") || got.stdout != "" {
				t.Errorf("validate = exit %d, stdout %q, stderr %q, want exit 1 and alpha's approval stale", got.code(), got.stdout, got.stderr)
			}
			// A new approval of what the skill is now is what makes it hold again.
			w.approved()
		})
	}
}

func TestTheRecordOfAnApprovalIsNotPartOfItsOwnDigest(t *testing.T) {
	w := newApprovalWorld(t)
	first := w.approve()
	if first.code() != 0 || !strings.HasPrefix(first.stdout, "approved: alpha\nsha256: ") {
		t.Fatalf("first approve = exit %d, %q, %q", first.code(), first.stdout, first.stderr)
	}
	again := w.approve()
	if again.code() != 0 || !strings.HasPrefix(again.stdout, "unchanged: alpha\n") {
		t.Errorf("a second approve of the same files = exit %d, %q, %q, want the approval left unchanged", again.code(), again.stdout, again.stderr)
	}
	digestLine := func(stdout string) string { return strings.Split(stdout, "\n")[1] }
	if digestLine(first.stdout) != digestLine(again.stdout) {
		t.Errorf("the digest was %q before the record existed and %q once it did", digestLine(first.stdout), digestLine(again.stdout))
	}
}

// A link, or anything else that is not a regular file, is something an approval cannot cover: the
// bytes behind a link are not the skill's. It is never skipped in silence.
func TestAnApprovalCannotCoverALinkInTheSkillDirectory(t *testing.T) {
	for _, tc := range []struct{ name, target, link string }{
		{"a link to a file", "SKILL.md", "shortcut.md"},
		{"a link to a directory", "lib", "libs"},
		{"a link to nothing", "nowhere", "dangling"},
	} {
		t.Run("approve refuses "+tc.name, func(t *testing.T) {
			w := newApprovalWorld(t)
			if err := os.Symlink(tc.target, w.path("skills/alpha/"+tc.link)); err != nil {
				t.Fatal(err)
			}
			got := w.approve()
			if got.code() != 1 || got.stdout != "" || !strings.Contains(got.stderr, tc.link) || !strings.Contains(got.stderr, "symbolic link") {
				t.Errorf("approve = exit %d, stdout %q, stderr %q, want a refusal that names %q as a symbolic link", got.code(), got.stdout, got.stderr, tc.link)
			}
			if _, err := os.Stat(w.path("skills/alpha/" + skills.ApprovalRecordName)); err == nil {
				t.Error("a record was written for a skill the approval cannot cover")
			}
		})
		t.Run("validate reports "+tc.name, func(t *testing.T) {
			w := newApprovalWorld(t)
			w.approved()
			if err := os.Symlink(tc.target, w.path("skills/alpha/"+tc.link)); err != nil {
				t.Fatal(err)
			}
			got := w.validate()
			if got.code() != 1 || !strings.Contains(got.stderr, "[APPROVAL_UNVERIFIABLE] alpha:") || !strings.Contains(got.stderr, tc.link) {
				t.Errorf("validate = exit %d, stdout %q, stderr %q, want exit 1 and the approval unverifiable, naming %q", got.code(), got.stdout, got.stderr, tc.link)
			}
		})
	}
}

// The digest is the SHA-256 of the list that `sha256sum` writes for the files of the skill, sorted
// by path as bytes. The expected value was made outside the program, on the same three files:
//
//	(cd skills/alpha && fd -t f -H -E .approval.json | LC_ALL=C sort | xargs -d '\n' sha256sum | sha256sum)
//
// The two names that sort differently by walk order and by bytes (a/b.txt is walked before a.txt, and
// sorts after it) are in it on purpose.
func TestTheDigestOfAnApprovalIsWhatSha256sumGivesForTheListOfItsFiles(t *testing.T) {
	const want = "7d6ffe45818224fc8db83a0e5e213e7ffeee7a9dfd6edae9fa1e359811ec37ca"
	w := newApprovalWorld(t)
	for _, gone := range []string{alphaScript, alphaHelper} {
		if err := os.Remove(w.path(gone)); err != nil {
			t.Fatal(err)
		}
	}
	w.put("skills/alpha/a.txt", "a file at the top\n")
	w.put("skills/alpha/a/b.txt", "a file in a directory\n")

	got := w.approve()
	if got.code() != 0 || !strings.Contains(got.stdout, "\nsha256: "+want+"\n") {
		t.Fatalf("approve = exit %d, %q, %q, want the sha256 line to be %s", got.code(), got.stdout, got.stderr, want)
	}
	if record := w.record(); !strings.Contains(record, `"sha256": "`+want+`"`) {
		t.Errorf("the record = %s, want it to bind %s", record, want)
	}
}

// add holds a skill to the same approval: a global skill is registered only while every file it has
// is the one a human approved.
func TestAddRefusesASkillWhoseCompanionFileChangedAfterItsApproval(t *testing.T) {
	w := newOverlayWorld(t)
	deps := w.deps(noopOverlayLocker{}, os.ReadFile)
	writeTestFile(t, filepath.Join(w.root, "newbie", "run.sh"), "echo approved\n")
	approve := func() verbRun {
		return w.run(skillsApprove, deps, append([]string{"approve", "--id", "newbie", "--approver", "reviewer"}, w.flags()...)...)
	}
	if r := approve(); r.code() != 0 {
		t.Fatalf("approve newbie: exit %d, stderr %q", r.code(), r.stderr)
	}
	writeTestFile(t, filepath.Join(w.root, "newbie", "run.sh"), "echo changed after the approval\n")
	before := w.snapshot()

	r := w.run(skillsAdd, deps, append([]string{"add", "newbie"}, w.flags()...)...)
	if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, "[APPROVAL_STALE]") {
		t.Errorf("add = exit %d, %q, %q, want a refusal that says the approval is stale", r.code(), r.stdout, r.stderr)
	}
	if !reflect.DeepEqual(before, w.snapshot()) {
		t.Error("a refused add changed files")
	}

	if r := approve(); r.code() != 0 {
		t.Fatalf("approve newbie again: exit %d, stderr %q", r.code(), r.stderr)
	}
	if r := w.run(skillsAdd, deps, append([]string{"add", "newbie"}, w.flags()...)...); r.code() != 0 || r.stdout != "added: newbie\n" {
		t.Errorf("add after the new approval = exit %d, %q, %q", r.code(), r.stdout, r.stderr)
	}
}

// ---- S2: a record of version 1 covered only SKILL.md --------------------------------------------

func TestAVersionOneRecordIsStaleAndSaysItCoversOnlySKILLmd(t *testing.T) {
	w := newApprovalWorld(t)
	// A record the way the program wrote one before: the digest of SKILL.md alone, which is exactly
	// what it still matches.
	w.put("skills/alpha/"+skills.ApprovalRecordName, recordOf(1, "alpha", sha256hex.Sum([]byte(skillFile("alpha")))))

	got := w.validate()
	if got.code() != 1 || got.stdout != "" {
		t.Fatalf("validate = exit %d, stdout %q, stderr %q, want exit 1", got.code(), got.stdout, got.stderr)
	}
	for _, want := range []string{"[APPROVAL_STALE] alpha:", "covers only SKILL.md", "labdrian skills approve --id alpha --approver <name>"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr %q does not say %q", got.stderr, want)
		}
	}
}

func TestApproveReplacesAVersionOneRecordWithOneOfVersionTwo(t *testing.T) {
	w := newApprovalWorld(t)
	w.put("skills/alpha/"+skills.ApprovalRecordName, recordOf(1, "alpha", sha256hex.Sum([]byte(skillFile("alpha")))))

	got := w.approve()
	if got.code() != 0 || !strings.HasPrefix(got.stdout, "approved: alpha\n") {
		t.Fatalf("approve = exit %d, %q, %q, want a new record written over the old one", got.code(), got.stdout, got.stderr)
	}
	if record := w.record(); !strings.Contains(record, `"version": 2`) || strings.Contains(record, `"version": 1`) {
		t.Errorf("the record = %s, want version 2", record)
	}
	if r := w.validate(); r.code() != 0 {
		t.Errorf("validate after the new approval = exit %d, stderr %q", r.code(), r.stderr)
	}
}

func TestAddRefusesASkillWhoseApprovalIsARecordOfVersionOne(t *testing.T) {
	w := newOverlayWorld(t)
	writeTestFile(t, skills.ApprovalRecordPath(w.root, "newbie"), recordOf(1, "newbie", sha256hex.Sum([]byte(overlaySkillMD("newbie")))))
	r := w.run(skillsAdd, w.deps(noopOverlayLocker{}, os.ReadFile), append([]string{"add", "newbie"}, w.flags()...)...)
	if r.code() != 1 || !strings.Contains(r.stderr, "[APPROVAL_STALE]") || !strings.Contains(r.stderr, "covers only SKILL.md") {
		t.Errorf("add = exit %d, %q, %q, want a refusal that says the version 1 record covers only SKILL.md", r.code(), r.stdout, r.stderr)
	}
}

// Version 1 is the one version that is told apart: a record of any other version is still one this
// program cannot read.
func TestARecordOfAnyOtherVersionIsMalformed(t *testing.T) {
	for _, version := range []int{0, 3} {
		t.Run(fmt.Sprintf("version %d", version), func(t *testing.T) {
			w := newApprovalWorld(t)
			w.put("skills/alpha/"+skills.ApprovalRecordName, recordOf(version, "alpha", sha256hex.Sum([]byte(skillFile("alpha")))))
			got := w.validate()
			if got.code() != 1 || !strings.Contains(got.stderr, "[APPROVAL_MALFORMED] alpha:") || !strings.Contains(got.stderr, fmt.Sprintf("unsupported version %d", version)) {
				t.Errorf("validate = exit %d, stderr %q, want the record malformed for its version", got.code(), got.stderr)
			}
		})
	}
}

// ---- S3: no skill is exempt from the approval ---------------------------------------------------

func TestValidateCountsTheApprovedGlobalSkillsAndNothingElse(t *testing.T) {
	w := newApprovalWorld(t)
	w.approved()
	got := w.validate()
	const want = "global skill approvals verified (1 skills: 1 approved)\n"
	if got.code() != 0 || !strings.Contains(got.stdout, want) || strings.Contains(got.stdout, "grandfathered") {
		t.Errorf("validate = exit %d, stdout %q, want a line that says %q and does not count a baseline", got.code(), got.stdout, want)
	}
}
