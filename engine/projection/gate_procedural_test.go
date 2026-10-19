package projection_test

// Tests for decision 5 of C23 M4 (docs/architecture/adr-c23-m4-memory-gate.md): the project's
// procedural skills, which the caller reads from the lock and hands the gate, are of source
// procedural-skills, so a plan that excludes it denies the Skill tool for one of them and a Read of
// its SKILL.md. Every other skill is untouched.

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

const (
	claudeSkillFile = "/work/repo/.claude/skills/deploy-checks/SKILL.md"
	agentsSkillFile = "/work/repo/.agents/skills/deploy-checks/SKILL.md"
)

// lockOf is a lock that records the procedural skill deploy-checks, written to both targets.
func lockOf() projection.ProceduralSkills {
	return projection.ProceduralSkills{Skills: []projection.ProceduralSkill{
		{ID: "deploy-checks", SkillFiles: []string{claudeSkillFile, agentsSkillFile}},
		{ID: "release-notes", SkillFiles: []string{"/work/repo/.claude/skills/release-notes/SKILL.md"}},
	}}
}

func skillCall(skill string) projection.ToolCall {
	return projection.ToolCall{Name: "Skill", ReadSkill: func() string { return skill }}
}

func readCall(path string) projection.ToolCall {
	return projection.ToolCall{Name: "Read", ReadFilePath: func() string { return path }}
}

// proceduralGate runs the gate for workflow w with the lock it is given.
func proceduralGate(w *workflow.Loaded, call projection.ToolCall, lock projection.ProceduralSkills) projection.GateResult {
	return projection.Gate(projection.GateInput{Binding: ownedBinding(), Workflow: w, EditTools: editTools, EngramName: lastSegment, Procedural: lock, Call: call})
}

// --- which calls can use a procedural skill ---------------------------------------

// TestTheGateLooksAtTheSkillToolAndAReadOfASkillFileOnly: the Skill tool is relevant by its name;
// a Read is relevant only when the file it names is a SKILL.md, which is decided from the path
// alone, so every other Read is answered without the binding, the workflow or the lock.
func TestTheGateLooksAtTheSkillToolAndAReadOfASkillFileOnly(t *testing.T) {
	for _, tool := range []string{"Skill", "Read"} {
		if !projection.GateRelevant(editTools, tool) {
			t.Errorf("GateRelevant(%q) = false, want true: the gate may have an opinion about it", tool)
		}
	}
	for _, tt := range []struct {
		call projection.ToolCall
		want bool
	}{
		{skillCall("deploy-checks"), true},
		{skillCall(""), true},
		{readCall(claudeSkillFile), true},
		{readCall("/anywhere/SKILL.md"), true},
		{readCall("/work/repo/.claude/skills/deploy-checks/notes.md"), false},
		{readCall("/work/repo/main.go"), false},
		{readCall("/work/repo/skill.md"), false},
		{readCall("/work/repo/SKILL.md.bak"), false},
		{readCall(""), false},
		{projection.ToolCall{Name: "Read"}, false},
		{projection.ToolCall{Name: "Grep", ReadFilePath: func() string { return claudeSkillFile }}, false},
		{projection.ToolCall{Name: "Edit", ReadFilePath: func() string { return claudeSkillFile }}, true},
		{projection.ToolCall{Name: "skill", ReadSkill: func() string { return "deploy-checks" }}, false},
		{projection.ToolCall{Name: "mcp__longterm-mem__query"}, true},
		{projection.ToolCall{Name: "Bash"}, false},
	} {
		if got := projection.GateRelevantCall(editTools, tt.call); got != tt.want {
			t.Errorf("GateRelevantCall(%s, skill %q, path %q) = %v, want %v", tt.call.Name, read(tt.call.ReadSkill), read(tt.call.ReadFilePath), got, tt.want)
		}
	}
}

func read(f func() string) string {
	if f == nil {
		return "<nil>"
	}
	return f()
}

// --- the rule ---------------------------------------------------------------------

// TestGateDeniesAProceduralSkillThePlanExcludes: sdd's plan holds engram alone, so the Skill tool
// for a skill of the lock, and a Read of its SKILL.md under either target, are denied, naming the
// skill, the source and the profile; odd includes procedural-skills and lets them through.
func TestGateDeniesAProceduralSkillThePlanExcludes(t *testing.T) {
	sdd := loadedWorkflow("sdd", workflow.StatusRunning)
	denied(t, "Skill", proceduralGate(sdd, skillCall("deploy-checks"), lockOf()),
		`procedural skill "deploy-checks"`, `does not include source "procedural-skills"`, `profile "sdd"`, "workflow wf-1 of project proj-1", "labdrian workflow unbind")
	for _, path := range []string{claudeSkillFile, agentsSkillFile, "/work/repo/.claude/skills/./deploy-checks/SKILL.md", "/work/repo/.claude/skills/x/../deploy-checks/SKILL.md"} {
		denied(t, "Read "+path, proceduralGate(sdd, readCall(path), lockOf()),
			`SKILL.md of procedural skill "deploy-checks"`, `does not include source "procedural-skills"`, "labdrian workflow unbind")
	}
	denied(t, "another skill of the lock", proceduralGate(sdd, skillCall("release-notes"), lockOf()), `"release-notes"`)

	odd := loadedWorkflow("odd", workflow.StatusRunning)
	allowed(t, "odd/Skill", proceduralGate(odd, skillCall("deploy-checks"), lockOf()))
	allowed(t, "odd/Read", proceduralGate(odd, readCall(claudeSkillFile), lockOf()))

	none := loadedWorkflow("standalone-minimal", workflow.StatusRunning)
	denied(t, "scope none/Skill", proceduralGate(none, skillCall("deploy-checks"), lockOf()), "permits no project", "(scope none)", `"deploy-checks"`)
	denied(t, "scope none/Read", proceduralGate(none, readCall(agentsSkillFile), lockOf()), "permits no project", `"deploy-checks"`)
}

// TestGateLeavesEveryOtherSkillAlone: only the lock's procedural skills are of source
// procedural-skills (C23 M1 decision 3): a skill the lock does not record (an installed registry
// skill, a global one), another file of a skill's directory, a SKILL.md elsewhere, a relative path,
// and a name in another case all pass, with no warning.
func TestGateLeavesEveryOtherSkillAlone(t *testing.T) {
	sdd := loadedWorkflow("sdd", workflow.StatusRunning)
	for _, call := range []projection.ToolCall{
		skillCall("go-testing"), skillCall("Deploy-checks"), skillCall("deploy-checks "), skillCall(""), skillCall("plugin:deploy-checks"),
		{Name: "Skill"},
		readCall("/work/repo/.claude/skills/deploy-checks/notes.md"),
		readCall("/work/repo/.claude/skills/go-testing/SKILL.md"),
		readCall("/home/u/.claude/skills/deploy-checks/SKILL.md"),
		readCall(".claude/skills/deploy-checks/SKILL.md"),
		readCall("/work/repo/.claude/skills/deploy-checks/skill.md"),
		{Name: "Read"},
	} {
		allowed(t, call.Name+" "+read(call.ReadSkill)+read(call.ReadFilePath), proceduralGate(sdd, call, lockOf()))
	}
	// A Skill call that names no skill uses none, even beside a lock entry with no id.
	noID := projection.ProceduralSkills{Skills: []projection.ProceduralSkill{{ID: ""}}}
	allowed(t, "no skill beside an entry with no id", proceduralGate(sdd, skillCall(""), noID))
	allowed(t, "a Skill call with no reader", proceduralGate(sdd, projection.ToolCall{Name: "Skill"}, noID))
}

// TestGateChecksNoSkillWithoutALock: a project with no lock has no procedural skill, so there is
// nothing to check and nothing to say.
func TestGateChecksNoSkillWithoutALock(t *testing.T) {
	for _, profile := range []string{"sdd", "standalone-minimal"} {
		w := loadedWorkflow(profile, workflow.StatusRunning)
		allowed(t, profile+"/Skill", proceduralGate(w, skillCall("deploy-checks"), projection.ProceduralSkills{}))
		allowed(t, profile+"/Read", proceduralGate(w, readCall(claudeSkillFile), projection.ProceduralSkills{}))
	}
}

// TestGateWarnsOnceWhenTheLockCannotBeRead: a lock that is unreadable or invalid leaves the gate
// unable to tell a procedural skill from another one, which is an allow with one warning when the
// plan excludes procedural-skills, and nothing at all when it includes them.
func TestGateWarnsOnceWhenTheLockCannotBeRead(t *testing.T) {
	broken := projection.ProceduralSkills{Unreadable: "parse project lock: unsupported version 2,\nwant 1"}
	sdd := loadedWorkflow("sdd", workflow.StatusRunning)
	for _, call := range []projection.ToolCall{skillCall("deploy-checks"), readCall(claudeSkillFile)} {
		got := proceduralGate(sdd, call, broken)
		warned(t, call.Name, got, "procedural skills lock", "could not be read", "unsupported version 2, want 1", "not checked", "workflow wf-1 of project proj-1")
	}
	odd := loadedWorkflow("odd", workflow.StatusRunning)
	allowed(t, "odd/Skill", proceduralGate(odd, skillCall("deploy-checks"), broken))
	allowed(t, "odd/Read", proceduralGate(odd, readCall(claudeSkillFile), broken))
}

// TestGateWarnsWhenItCannotComputeThePlanOfAProceduralSkill: an unknown is an allow, said once,
// and only for a skill of the lock.
func TestGateWarnsWhenItCannotComputeThePlanOfAProceduralSkill(t *testing.T) {
	w := loadedWorkflow("no-such-profile", workflow.StatusRunning)
	warned(t, "Skill", proceduralGate(w, skillCall("deploy-checks"), lockOf()), "no-such-profile", "not checked")
	allowed(t, "another skill", proceduralGate(w, skillCall("go-testing"), lockOf()))
	// With the lock unreadable too, the plan is what could not be computed first, and the warning
	// says so rather than blaming the lock.
	broken := projection.ProceduralSkills{Unreadable: "parse project lock: unsupported version 2"}
	got := proceduralGate(w, skillCall("deploy-checks"), broken)
	warned(t, "Skill with a broken lock", got, "memory plan", "could not be computed", "no-such-profile", "not checked")
	if strings.Contains(got.Warning, "lock") {
		t.Errorf("warning %q blames the lock for a plan that could not be computed", got.Warning)
	}
}

// TestTheProceduralRuleAppliesInEveryOpenStatusAndOnlyThen.
func TestTheProceduralRuleAppliesInEveryOpenStatusAndOnlyThen(t *testing.T) {
	for _, status := range openStatuses {
		denied(t, string(status), proceduralGate(loadedWorkflow("sdd", status), skillCall("deploy-checks"), lockOf()), `"deploy-checks"`)
	}
	allowed(t, "closed", proceduralGate(loadedWorkflow("sdd", workflow.StatusClosed), skillCall("deploy-checks"), lockOf()))
	allowed(t, "not bound", projection.Gate(projection.GateInput{
		Binding: projection.Loaded{Classification: projection.ClassificationAbsent}, Workflow: loadedWorkflow("sdd", workflow.StatusRunning),
		Procedural: lockOf(), Call: skillCall("deploy-checks"),
	}))
}

// TestTheGateReadsOnlyTheArgumentItNeeds: the skill of a Skill call, the path of a Read, and
// nothing of any other call.
func TestTheGateReadsOnlyTheArgumentItNeeds(t *testing.T) {
	sdd := loadedWorkflow("sdd", workflow.StatusRunning)
	for _, name := range []string{"Skill", "Read", "Edit", "Bash", "mcp__longterm-mem__get"} {
		skills, paths := 0, 0
		proceduralGate(sdd, projection.ToolCall{Name: name,
			ReadSkill:    func() string { skills++; return "deploy-checks" },
			ReadFilePath: func() string { paths++; return claudeSkillFile },
		}, lockOf())
		wantSkills, wantPaths := 0, 0
		switch name {
		case "Skill":
			wantSkills = 1
		case "Read":
			wantPaths = 1
		}
		if skills > wantSkills || (wantSkills > 0 && skills == 0) || paths > 2*wantPaths || (wantPaths > 0 && paths == 0) {
			t.Errorf("%s: the skill was read %d times and the path %d times", name, skills, paths)
		}
	}
	// The input is not changed.
	lock := lockOf()
	before := strings.Join(lock.Skills[0].SkillFiles, ",")
	proceduralGate(sdd, readCall(claudeSkillFile), lock)
	if strings.Join(lock.Skills[0].SkillFiles, ",") != before {
		t.Error("the gate changed the lock it was given")
	}
}
