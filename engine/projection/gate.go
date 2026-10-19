package projection

// The PreToolUse gate: the pure decision of whether a tool call may go ahead in a
// session whose repository is bound to a workflow. Like Project, it is a function
// of its arguments and nothing else: it reads no file, starts no process, keeps
// no state, and never writes, so the caller (the hook command in engine/cmd) does
// the reading and says the result to the runtime (engine/hookwire). Nothing here
// knows the format that is spoken in: the gate is told the name of the tool and,
// for a memory query, what it names as its project.
//
// The gate has two rules, both about a workflow that is open (created, running,
// or paused) and owned:
//
//   - The paused edit gate. While the workflow is paused, the file-edit tools
//     are denied: the ones the caller names (GateInput.EditTools; the engine
//     names Write, Edit, MultiEdit, and NotebookEdit). Bash is never gated
//     (a shell command cannot be classified reliably), and neither is anything
//     that only reads.
//   - The memory gate (C23 M4, docs/architecture/adr-c23-m4-memory-gate.md). A
//     memory read is checked against the workflow's memory plan: the longterm-mem
//     query and get, and the Engram mem_search, mem_context, mem_get_observation
//     and mem_timeline. A plan with no project (scope none) permits no read at
//     all, and a plan that does not include the read's source denies it. Then a
//     longterm-mem query must name the plan's project, which is the shared
//     identity the workflow recorded at create (C23 M2), and, under scope goal,
//     the plan's goal too (C23 M5, docs/architecture/adr-c23-m5-goal-filter.md:
//     its goal argument is checked after its project, and under any other scope
//     never looked at), and an Engram mem_search
//     or mem_context must name the project Engram knows that identity by (the
//     caller's GateInput.EngramName) and must not ask for every project. A read
//     by id (get, mem_get_observation, mem_timeline) names no project and passes
//     once its source is in the plan. A workflow that recorded no identity
//     (created before M2) does not say which project the stores know it by, so
//     its reads pass, with a warning. Every write (promote, every Engram write)
//     and every other tool are never touched: the plan grants no write, and it
//     does not forbid one.
//
//     The project's procedural skills (C23 M4 decision 5) are of source
//     procedural-skills too: the Skill tool for a skill the procedural skills lock
//     records, and a Read whose absolute path, cleaned, is one of that skill's
//     SKILL.md files, are checked like a read by id. The caller reads the lock
//     (GateInput.Procedural) and only for such a call: a Read of any other file is
//     never looked at. A skill the lock does not record (a registry install, a
//     global skill) is never touched; no lock means no procedural skill, and a
//     lock that cannot be read is an allow, with one warning when the plan would
//     have denied a procedural skill.
//
// The gate denies only when it has a valid workflow state and a clear violation.
// Every unknown (no binding, a binding or workflow it cannot follow, a closed
// workflow, an unrecognized status, a plan it cannot compute, input it cannot
// read) is an allow, with a one-line warning only where the gate had something
// to check and could not. It never unbinds: removing the binding of a closed
// workflow is the prompt hook's job.

import (
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// GateInput is everything Gate decides from: the classification of the
// repository's binding as the binding store reported it, and, only when the
// binding is owned, the workflow it names as the workflow store reported it (nil
// when it could not be loaded), the tools the paused edit gate denies, and the
// tool call.
type GateInput struct {
	Binding  Loaded
	Workflow *workflow.Loaded
	// EditTools are the names of the tools that edit a file, which the paused edit
	// gate denies, matched exactly. The caller says which: the gate has no list of
	// its own, and with none it gates no edit.
	EditTools []string
	// EngramName is how Engram names the project of a shared identity (identity.EngramName in the
	// engine), which the Engram project check compares a read's project with. The caller says how:
	// the gate has no rule of its own, and with none it cannot check an Engram project, which is an
	// allow with a warning.
	EngramName func(identity string) string
	// Procedural is what the project's procedural skills lock says, as the caller read it: the gate
	// reads no file. The caller reads it only for a call that can use a procedural skill (the Skill
	// tool, or a Read of a SKILL.md); its zero value is a project with no lock.
	Procedural ProceduralSkills
	Call       ToolCall
}

// ProceduralSkills is the part of the project's procedural skills lock
// (.labdrian/procedural-skills.lock.json) the gate decides from: the procedural
// skills it records, which are the only skills of source procedural-skills (C23
// M1 decision 3). The skills a registry install wrote, and global skills, are not
// in it.
type ProceduralSkills struct {
	Skills []ProceduralSkill
	// Unreadable says why the lock could not be read or is not a valid lock. It is
	// empty when the lock was read, and when there is none.
	Unreadable string
}

// ProceduralSkill is one procedural skill of the lock: its id, which the Skill
// tool names, and the absolute paths of its SKILL.md, one per target it was
// written to.
type ProceduralSkill struct {
	ID         string
	SkillFiles []string
}

// ToolCall is the tool call the gate is asked about, as the gate reads it.
type ToolCall struct {
	// Name is the name of the tool, or empty when the call has none.
	Name string
	// ReadQuery reads what the call names as the project of a memory read. Reading the
	// arguments of a call is the caller's work and costs a decode of its whole input, so Gate
	// calls it only for a read whose project it checks (a longterm-mem query, an Engram
	// mem_search or mem_context), at most once, and never for any other call. It is nil for a
	// caller with no way to read them, which is read as arguments that are not named.
	ReadQuery func() QueryArguments
	// ReadSkill reads the skill a Skill call names, and ReadFilePath the file a Read call names:
	// "" when the argument is missing or not text. Gate calls each only for the tool it belongs
	// to. Either is nil for a caller with no way to read it, which is read as "".
	ReadSkill    func() string
	ReadFilePath func() string
}

// query reads the arguments of the call as those of a memory query.
func (c ToolCall) query() QueryArguments {
	if c.ReadQuery == nil {
		return QueryArguments{}
	}
	return c.ReadQuery()
}

// QueryArguments is the arguments of a call read as those of a memory query.
type QueryArguments struct {
	// Named is true when the arguments could be read as named arguments, among which is the
	// project. It is false when they could not (they are not a set of named arguments).
	Named bool
	// Project is the project the query names: empty when it names none, which is the case
	// when the argument is missing, null, not text, or empty.
	Project string
	// AllProjects is true when the "all_projects" argument is the JSON value true: the read asks
	// for every project. Any other value, or none, is false.
	AllProjects bool
	// Goal is the goal the query names (C23 M5): empty when it names none, which is the case when
	// the argument is missing, null, not text, or empty.
	Goal string
}

// GateResult is what Gate decides. Deny is true for a denial, and Reason then
// says why and how to proceed. Warning is one line for the user, set only when
// the gate allows a call it would have liked to check and could not.
type GateResult struct {
	Deny    bool
	Reason  string
	Warning string
}

// denyFallbackReason is the reason of a denial that somehow has none, so a denial
// never reaches the model and the user without a word.
const denyFallbackReason = "labdrian: this tool call was denied by the workflow this repository is bound to."

// Explanation is what a denial says: its reason, or, for a denial that somehow has
// none, a fixed sentence. Gate always gives a reason; this is for the caller that
// would otherwise print a denial with nothing to read.
func (r GateResult) Explanation() string {
	if r.Reason == "" {
		return denyFallbackReason
	}
	return r.Reason
}

// GateRelevant reports whether Gate can ever have an opinion about a tool: a
// file-edit tool of editTools (paused edit gate) or a memory read (memory gate:
// the longterm-mem query and get, and the Engram mem_search, mem_context,
// mem_get_observation and mem_timeline). It is decided from the name alone, so a
// caller can answer for every other tool without reading the binding or the
// workflow. Gate itself denies or warns only for tools this reports true for.
func GateRelevant(editTools []string, toolName string) bool {
	_, read := memoryReadOf(toolName)
	return slices.Contains(editTools, toolName) || read || toolName == skillTool || toolName == readTool
}

// GateRelevantCall is GateRelevant for a call, narrowed for a Read: a Read is
// relevant only when the file it names is a SKILL.md, which is decided from the
// path alone (C23 M4 decision 5), so a caller answers every other Read without
// reading the binding, the workflow, or the procedural skills lock.
func GateRelevantCall(editTools []string, call ToolCall) bool {
	return relevantWith(editTools, call.Name, skillUseOf(call))
}

// relevantWith is whether the gate can have an opinion about a call named name
// that uses a skill as use says: a file-edit tool of editTools, a memory read, or
// a use of a skill. A Skill or Read call that uses none (a Read of another file)
// is relevant only as a file-edit tool.
func relevantWith(editTools []string, name string, use skillUse) bool {
	_, read := memoryReadOf(name)
	return slices.Contains(editTools, name) || read || use.isSkill || use.isRead
}

// The tools that can use a skill: the Skill tool names one by its id, and a Read
// can read its SKILL.md.
const (
	skillTool     = "Skill"
	readTool      = "Read"
	skillFileName = "SKILL.md"
)

// skillUse is how a call can use a procedural skill: by its id (the Skill tool)
// or by reading a file (a Read of a SKILL.md). It is the zero value for a call
// that cannot use one.
type skillUse struct {
	skill, file string
	isSkill     bool
	isRead      bool
}

// skillUseOf reads what a call uses a skill by, reading its argument once: the
// skill a Skill call names, or the file a Read of a SKILL.md names. It is the zero
// value for every other call, a Read of another file included.
func skillUseOf(call ToolCall) skillUse {
	switch call.Name {
	case skillTool:
		return skillUse{isSkill: true, skill: readArg(call.ReadSkill)}
	case readTool:
		file := readArg(call.ReadFilePath)
		if file != "" && filepath.Base(filepath.Clean(file)) == skillFileName {
			return skillUse{isRead: true, file: file}
		}
	}
	return skillUse{}
}

// CanUseASkill reports whether a call can use a procedural skill: the Skill tool,
// or a Read of a SKILL.md. It is the call a caller reads the procedural skills
// lock for, and no other.
func CanUseASkill(call ToolCall) bool {
	use := skillUseOf(call)
	return use.isSkill || use.isRead
}

func readArg(read func() string) string {
	if read == nil {
		return ""
	}
	return read()
}

// The MCP names of the memory reads, each matched as strictly as the others and
// nothing looser: "mcp__", an optional plugin prefix made of one or more
// [A-Za-z0-9-] segments each followed by a single underscore, the server name,
// and the tool. So "mcp__longterm-mem__query", "mcp__plugin_x_longterm-mem__query"
// and "mcp__plugin_engram_engram__mem_search" match, and
// "mcp__longterm-memx__query", "mcp__a__longterm-mem__query",
// "mcp__engramx__mem_search", "longterm-mem__query", and every write
// ("mcp__longterm-mem__promote", "mcp__engram__mem_save") do not.
var (
	longtermQueryTool = regexp.MustCompile(`^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$`)
	longtermGetTool   = regexp.MustCompile(`^mcp__([A-Za-z0-9-]+_)*longterm-mem__get$`)
	// engramProjectReadTool reads name a project; engramIDReadTool reads look an
	// observation up by its id.
	engramProjectReadTool = regexp.MustCompile(`^mcp__([A-Za-z0-9-]+_)*engram__(mem_search|mem_context)$`)
	engramIDReadTool      = regexp.MustCompile(`^mcp__([A-Za-z0-9-]+_)*engram__(mem_get_observation|mem_timeline)$`)
)

// memoryRead is one kind of memory read the gate checks: the source of the plan it
// reads, how a sentence names it, and the project check it carries.
type memoryRead struct {
	source memoryscope.Source
	// what names the read in a reason: "longterm-mem query", "Engram read".
	what string
	// unbindTo ends the advice to unbind: "... 'labdrian workflow unbind' to query."
	unbindTo string
	check    projectCheck
}

// projectCheck is what a memory read is checked for once its source is in the plan.
type projectCheck int

const (
	// checkSourceOnly is a read by id: it names no project, so its source is all
	// there is to check (C23 M4, decision 4).
	checkSourceOnly projectCheck = iota
	// checkLongtermProject compares the project argument with the plan's project.
	checkLongtermProject
	// checkEngramProject compares the project argument with the Engram name of the
	// workflow's identity, and refuses every project (C23 M4, decision 3).
	checkEngramProject
)

// memoryReads are the memory reads, by the pattern of their tool name.
var memoryReads = []struct {
	tool *regexp.Regexp
	read memoryRead
}{
	{longtermQueryTool, memoryRead{source: memoryscope.SourceLongtermMem, what: "longterm-mem query", unbindTo: "query", check: checkLongtermProject}},
	{longtermGetTool, memoryRead{source: memoryscope.SourceLongtermMem, what: "longterm-mem get", unbindTo: "read it", check: checkSourceOnly}},
	{engramProjectReadTool, memoryRead{source: memoryscope.SourceEngram, what: "Engram read", unbindTo: "read it", check: checkEngramProject}},
	{engramIDReadTool, memoryRead{source: memoryscope.SourceEngram, what: "Engram read", unbindTo: "read it", check: checkSourceOnly}},
}

// memoryReadOf is the memory read a tool is, by its name.
func memoryReadOf(toolName string) (memoryRead, bool) {
	for _, r := range memoryReads {
		if r.tool.MatchString(toolName) {
			return r.read, true
		}
	}
	return memoryRead{}, false
}

// Gate decides whether the tool call in in may go ahead. See the file comment
// for the rules. It is total: it never fails and never panics, and anything it
// does not understand is an allow.
func Gate(in GateInput) GateResult {
	use := skillUseOf(in.Call)
	if !relevantWith(in.EditTools, in.Call.Name, use) {
		return GateResult{}
	}
	if in.Binding.Classification != ClassificationOwned {
		return GateResult{}
	}
	w := in.Workflow
	if w == nil || w.Classification != workflow.ClassificationOwned {
		return GateResult{}
	}
	switch w.State.Status {
	case workflow.StatusPaused:
		if slices.Contains(in.EditTools, in.Call.Name) {
			return GateResult{Deny: true, Reason: pausedReason(in.Binding.Binding, in.Call.Name)}
		}
	case workflow.StatusCreated, workflow.StatusRunning:
	default:
		// Closed, or a status this package does not know: not a state the gate
		// enforces anything in.
		return GateResult{}
	}
	if read, ok := memoryReadOf(in.Call.Name); ok {
		return memoryGate(in, w.State, read)
	}
	if use.isSkill || use.isRead {
		return proceduralGate(in, w.State, use)
	}
	return GateResult{}
}

// proceduralGate checks a use of a skill (C23 M4 decision 5): a skill the lock
// records is of source procedural-skills and goes through the memory gate like a
// read by id; any other skill is not the gate's business. A lock that could not be
// read leaves the gate unable to tell, which it says once when the plan would
// deny a procedural skill, and not at all when the plan includes them.
func proceduralGate(in GateInput, s workflow.State, use skillUse) GateResult {
	if in.Procedural.Unreadable != "" {
		b := in.Binding.Binding
		plan, err := resolvePlan(s, b.ProjectID)
		if err != nil {
			// The plan is the first thing the gate could not read, so the warning names it.
			return GateResult{Warning: "labdrian: the memory plan of " + workflowRef(b) + " could not be computed" + detailIn(err.Error()) +
				", so this " + use.what() + " was not checked against it."}
		}
		if plan.Scope != memoryscope.ScopeNone && slices.Contains(plan.Sources, memoryscope.SourceProceduralSkills) {
			return GateResult{}
		}
		return GateResult{Warning: "labdrian: the procedural skills lock of this repository could not be read" + detailIn(in.Procedural.Unreadable) +
			", so whether this " + use.what() + " uses a procedural skill was not checked against the memory plan of " + workflowRef(b) + "."}
	}
	id, ok := in.Procedural.match(use)
	if !ok {
		return GateResult{}
	}
	what := "use of procedural skill " + quoteValue(id)
	if use.isRead {
		what = "read of the SKILL.md of procedural skill " + quoteValue(id)
	}
	return memoryGate(in, s, memoryRead{source: memoryscope.SourceProceduralSkills, what: what, unbindTo: "use it", check: checkSourceOnly})
}

// what names a use of a skill whose skill is not known to be procedural.
func (u skillUse) what() string {
	if u.isRead {
		return "read of a SKILL.md"
	}
	return "use of skill " + quoteValue(u.skill)
}

// match is the id of the procedural skill a use uses: the skill a Skill call
// names, exactly; or the skill one of whose SKILL.md files a Read names, compared
// as cleaned paths. The lock's files are absolute, so a relative path never
// matches one: the gate does not guess what it is relative to.
func (p ProceduralSkills) match(use skillUse) (string, bool) {
	for _, skill := range p.Skills {
		if use.isSkill && use.skill != "" && use.skill == skill.ID {
			return skill.ID, true
		}
		if use.isRead {
			for _, file := range skill.SkillFiles {
				if filepath.Clean(use.file) == filepath.Clean(file) {
					return skill.ID, true
				}
			}
		}
	}
	return "", false
}

func pausedReason(b Binding, tool string) string {
	flags := "--project " + sanitizeLine(b.ProjectID) + " --workflow " + sanitizeLine(b.WorkflowID)
	return "labdrian: " + workflowRef(b) + " is paused, so " + tool + " is denied: a paused workflow is not to be advanced. " +
		"Resume it with 'labdrian workflow resume " + flags + "', or stop following it with 'labdrian workflow unbind'."
}

// memoryGate checks one memory read against the memory plan of the workflow,
// computed exactly as the projected context computes it: its source first, then
// the project it names, for a read that names one.
func memoryGate(in GateInput, s workflow.State, read memoryRead) GateResult {
	b := in.Binding.Binding
	plan, err := resolvePlan(s, b.ProjectID)
	if err != nil {
		return GateResult{Warning: "labdrian: the memory plan of " + workflowRef(b) + " could not be computed" + detailIn(err.Error()) +
			", so this " + read.what + " was not checked against it."}
	}
	if plan.Scope == memoryscope.ScopeNone {
		return GateResult{Deny: true, Reason: "labdrian: " + workflowRef(b) + " (profile " + strconv.Quote(sanitizeLine(s.Profile)) +
			") permits no project in its memory plan (scope none), so every " + read.what + " is denied. " +
			"Stop following this workflow with 'labdrian workflow unbind' to " + read.unbindTo + "."}
	}
	if !slices.Contains(plan.Sources, read.source) {
		return GateResult{Deny: true, Reason: "labdrian: the memory plan of " + workflowRef(b) + " (profile " + strconv.Quote(sanitizeLine(s.Profile)) +
			") does not include source " + strconv.Quote(string(read.source)) + ", so this " + read.what + " is denied. " +
			"Stop following this workflow with 'labdrian workflow unbind' to " + read.unbindTo + "."}
	}
	if read.check == checkSourceOnly {
		return GateResult{}
	}
	if s.ProjectIdentity == "" {
		// A workflow created before C23 M2 recorded no shared identity, so the project
		// the store names it by is unknown here: an unknown is an allow.
		return GateResult{Warning: "labdrian: " + workflowRef(b) + " records no project identity (it was created before workflows recorded one), " +
			"so the project of this " + read.what + " was not checked against its memory plan."}
	}
	if plan.Filters.ProjectID == "" {
		// A plan with a scope that names a project always has one; this cannot be
		// reached through Resolve, and the gate does not deny on what it cannot
		// explain.
		return GateResult{Warning: "labdrian: the memory plan of " + workflowRef(b) + " names no project although its scope is " +
			strconv.Quote(sanitizeLine(string(plan.Scope))) + ", so this " + read.what + " was not checked against it."}
	}
	if read.check == checkEngramProject {
		return engramProjectGate(b, in.EngramName, s.ProjectIdentity, in.Call)
	}
	return longtermProjectGate(b, plan, in.Call)
}

// longtermProjectGate checks the project a longterm-mem query names against the
// plan's project, which is the workflow's shared identity (C23 M2): another
// project, or none, is denied. Under scope goal the goal it names is checked
// next (longtermGoalGate).
func longtermProjectGate(b Binding, plan memoryscope.Plan, call ToolCall) GateResult {
	planProject := plan.Filters.ProjectID
	query := call.query()
	if !query.Named {
		return GateResult{Warning: "labdrian: the input of this longterm-mem query is not a JSON object, so its project was not checked against the memory plan of " +
			workflowRef(b) + "."}
	}
	given := query.Project
	if given == "" {
		// Absent, null, not text, or empty: the query names no project.
		return GateResult{Deny: true, Reason: "labdrian: this longterm-mem query names no project (the project argument is missing, not a string, or empty), " +
			"but the memory plan of " + workflowRef(b) + " permits only project " + strconv.Quote(sanitizeLine(planProject)) + ". " + queryAdvice(planProject)}
	}
	if given != planProject {
		return GateResult{Deny: true, Reason: "labdrian: this longterm-mem query asks for project " + quoteValue(given) +
			", but the memory plan of " + workflowRef(b) + " permits only project " + strconv.Quote(sanitizeLine(planProject)) + ". " + queryAdvice(planProject)}
	}
	if plan.Scope == memoryscope.ScopeGoal {
		return longtermGoalGate(b, plan, query.Goal)
	}
	return GateResult{}
}

// longtermGoalGate checks the goal a longterm-mem query names against the plan's
// goal under scope goal (C23 M5 decision 3, docs/architecture/adr-c23-m5-goal-filter.md):
// another goal, or none, is denied, as another project or none is. Under any other
// scope the goal is never looked at.
func longtermGoalGate(b Binding, plan memoryscope.Plan, given string) GateResult {
	planProject, planGoal := plan.Filters.ProjectID, plan.Filters.GoalID
	if planGoal == "" {
		// A plan of scope goal always has a goal; this cannot be reached through
		// Resolve, and the gate does not deny on what it cannot explain.
		return GateResult{Warning: "labdrian: the memory plan of " + workflowRef(b) + " names no goal although its scope is " +
			strconv.Quote(string(memoryscope.ScopeGoal)) + ", so the goal of this longterm-mem query was not checked against it."}
	}
	if given == "" {
		// Absent, null, not text, or empty: the query names no goal.
		return GateResult{Deny: true, Reason: "labdrian: this longterm-mem query names no goal (the goal argument is missing, not a string, or empty), " +
			"but the memory plan of " + workflowRef(b) + " permits only goal " + strconv.Quote(sanitizeLine(planGoal)) + ". " + goalAdvice(planProject, planGoal)}
	}
	if given != planGoal {
		return GateResult{Deny: true, Reason: "labdrian: this longterm-mem query asks for goal " + quoteValue(given) +
			", but the memory plan of " + workflowRef(b) + " permits only goal " + strconv.Quote(sanitizeLine(planGoal)) + ". " + goalAdvice(planProject, planGoal)}
	}
	return GateResult{}
}

// goalAdvice tells the model and the user how to go on after a query denied for its goal.
func goalAdvice(planProject, planGoal string) string {
	return "Query project " + strconv.Quote(sanitizeLine(planProject)) + " with goal " + strconv.Quote(sanitizeLine(planGoal)) +
		", or stop following this workflow with 'labdrian workflow unbind'."
}

// engramProjectGate checks the project an Engram mem_search or mem_context names
// against the name Engram gives the workflow's identity (engramName of identity,
// C23 M2 decision 3): another project, none, or every project is denied.
func engramProjectGate(b Binding, engramName func(string) string, identity string, call ToolCall) GateResult {
	var want string
	if engramName != nil {
		want = engramName(identity)
	}
	if want == "" {
		return GateResult{Warning: "labdrian: the Engram name of the project of " + workflowRef(b) +
			" is not known here, so the project of this Engram read was not checked against its memory plan."}
	}
	read := call.query()
	if !read.Named {
		return GateResult{Warning: "labdrian: the input of this Engram read is not a JSON object, so its project was not checked against the memory plan of " +
			workflowRef(b) + "."}
	}
	permits := "the memory plan of " + workflowRef(b) + " permits only project " + strconv.Quote(sanitizeLine(want)) + ". " + engramAdvice(want)
	switch {
	case read.AllProjects:
		return GateResult{Deny: true, Reason: "labdrian: this Engram read asks for every project (all_projects is true), but " + permits}
	case read.Project == "":
		// Absent, null, not text, or empty: the read names no project.
		return GateResult{Deny: true, Reason: "labdrian: this Engram read names no project (the project argument is missing, not a string, or empty), but " + permits}
	case read.Project != want:
		return GateResult{Deny: true, Reason: "labdrian: this Engram read asks for project " + quoteValue(read.Project) + ", but " + permits}
	}
	return GateResult{}
}

// engramAdvice tells the model and the user how to go on after a denied Engram read.
func engramAdvice(want string) string {
	return "Name project " + strconv.Quote(sanitizeLine(want)) + " without all_projects, or stop following this workflow with 'labdrian workflow unbind'."
}

// quoteValue renders a value taken from the tool input as a quoted Go string cut
// to maxDetailRunes characters. Quoting escapes every control, format, and
// invalid character (a line break, an escape sequence, a bidirectional override)
// as visible text, so the value is safe on one line, and unlike sanitizing it
// keeps a difference that is only white space visible: "proj-1 " is not shown
// as "proj-1".
func quoteValue(s string) string {
	return strconv.Quote(clip(strings.ToValidUTF8(s, ""), maxDetailRunes))
}

// queryAdvice tells the model and the user how to go on after a denied query.
func queryAdvice(planProject string) string {
	return "Query project " + strconv.Quote(sanitizeLine(planProject)) + ", or stop following this workflow with 'labdrian workflow unbind'."
}
