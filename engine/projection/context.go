package projection

// The projection: the pure decision of what a session is told about the
// workflow its repository is bound to. Everything here is a function of its
// arguments. Nothing reads a file, an environment variable, or the clock, and
// nothing keeps state between calls, so the same binding and the same workflow
// state give the same bytes whichever process, session, or prompt asks. That is
// what lets a new session, or a restarted one, be told exactly what the last
// one was told.
//
// The caller (the hook command in engine/cmd) does the reading: it loads the
// binding and the workflow, calls Project, removes the binding when Project
// asks for that, and prints UserPromptSubmitOutput.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// HookEventUserPromptSubmit is the Claude Code hook event whose stdout is added
// to the session's context. It is the only event this package projects for.
const HookEventUserPromptSubmit = "UserPromptSubmit"

// MaxHookInputBytes bounds the hook input ParseHookInput accepts. A prompt can be
// long, and Claude Code sends it in the input, so the bound is generous; it
// exists so that a hostile or runaway writer cannot make the hook read without
// end.
const MaxHookInputBytes = 1 << 20

// MaxContextBytes bounds the projected context. A context this size costs a
// session about four thousand tokens on every prompt, far more than the few
// hundred a typical workflow needs; the bound only stops a workflow log that
// was edited by hand from filling the session.
const MaxContextBytes = 16384

// maxDetailRunes bounds the explanation copied from an error into a warning. A
// warning is shown to the user on every prompt while the problem lasts, so it
// has to stay short.
const maxDetailRunes = 200

// maxPanicRunes bounds the text of a recovered panic in the warning the user
// sees. A panic value can be any text, including a long dump, and the full
// value already goes to stderr; the user needs only enough to report it.
const maxPanicRunes = 120

// goalDigestRunes is how many leading characters of the goal digest the context
// shows. The digest is 64 hex digits; twelve are enough to tell two goals apart
// at a glance and keep the line short. The full digest stays in the workflow log.
const goalDigestRunes = 12

// ErrHookInputTooLarge is returned by ParseHookInput when the input exceeds
// MaxHookInputBytes.
var ErrHookInputTooLarge = errors.New("projection: hook input exceeds the maximum size")

// truncationMarker ends a context that had to be cut to MaxContextBytes. It
// starts on a line of its own and is ASCII, so it can never be cut itself.
var truncationMarker = "\n[labdrian: projected context truncated at " + strconv.Itoa(MaxContextBytes) + " bytes]"

// HookInput is what the projection keeps of the JSON a Claude Code hook
// receives on stdin: the event's name and the working directory of the session
// (cleaned, see ParseHookInput).
// Nothing else is read. In particular the session and the prompt are not: the
// projection depends on neither, so that every session, and every prompt,
// gets the same view of the workflow.
type HookInput struct {
	// HookEventName is hook_event_name, or empty when the input has none.
	HookEventName string
	// Cwd is the session's working directory as a cleaned absolute path, or empty
	// when the input has none or it is not an absolute path.
	Cwd string
}

// ParseHookInput reads the fields the projection needs from a hook's stdin. The
// input is Claude Code's, not ours, so it is decoded leniently: it must be a
// JSON object no larger than MaxHookInputBytes, and fields it does not name are
// ignored, including ones a later Claude Code adds. A field this function
// reads with a value of the wrong type is an error, and so is a document that is
// not one object.
func ParseHookInput(data []byte) (HookInput, error) {
	if len(data) > MaxHookInputBytes {
		return HookInput{}, fmt.Errorf("parse hook input: %w: %d bytes exceeds the maximum of %d", ErrHookInputTooLarge, len(data), MaxHookInputBytes)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return HookInput{}, errors.New("parse hook input: the input is not a JSON object")
	}
	var wire struct {
		HookEventName string `json:"hook_event_name"`
		Cwd           string `json:"cwd"`
	}
	if err := json.Unmarshal(trimmed, &wire); err != nil {
		return HookInput{}, fmt.Errorf("parse hook input: %w", err)
	}
	in := HookInput{HookEventName: wire.HookEventName}
	if filepath.IsAbs(wire.Cwd) {
		// Cleaned, so dot segments, repeated separators, and a trailing slash
		// never make one directory look like two.
		in.Cwd = filepath.Clean(wire.Cwd)
	}
	return in, nil
}

// ProjectionInput is everything Project decides from: the classification of the
// repository's binding as the binding store reported it, and, only when the
// binding is owned, the workflow it names as the workflow store reported it. A
// nil Workflow with an owned binding means the workflow could not be loaded.
type ProjectionInput struct {
	Binding  Loaded
	Workflow *workflow.Loaded
}

// ProjectionResult is what Project decides.
//
// Context is text for the session, or empty. Warning is one line for the user,
// or empty. Unbind asks the caller to remove the repository's binding, which
// Project does only for a workflow it read and understood to be closed. The
// three are independent of the session and the prompt. When Unbind is set the
// caller tries the removal and then calls AfterUnbind, so the note says what
// really happened to the binding.
type ProjectionResult struct {
	Context string
	Warning string
	Unbind  bool

	// closedNote is the part of a closed workflow's Context that does not depend
	// on the removal, kept so that AfterUnbind can complete it.
	closedNote string
}

// AfterUnbind completes the note of a closed workflow with what the caller's
// removal of the binding did, and returns the result; any other result is
// returned unchanged. removed and err are what Store.UnbindIfUnchanged returned:
// the note says the binding was removed only when removed is true and err is
// nil; when err is ErrBindingChanged, or there is neither an error nor a
// removal (the binding was already gone), the note says the binding was left
// alone; any other err is a failure, and the note says so, why, and that
// 'labdrian workflow unbind' finishes it by hand (the same note would otherwise
// repeat on every prompt with no way out). It may be
// called more than once: it always starts from the same note.
func (r ProjectionResult) AfterUnbind(removed bool, err error) ProjectionResult {
	if !r.Unbind || r.closedNote == "" {
		return r
	}
	switch {
	case errors.Is(err, ErrBindingChanged):
		// Not a failure: another process bound something else, which is not ours
		// to remove.
		r.Context = r.closedNote + " This repository's binding to it had already changed, so it was left alone."
	case err != nil:
		r.Context = r.closedNote + " Removing this repository's binding to it failed" + detailIn(err.Error()) +
			", so it is still bound and this note repeats on every prompt until it is removed: run 'labdrian workflow unbind'."
	case removed:
		r.Context = r.closedNote + " This repository's binding to it was removed."
	default:
		r.Context = r.closedNote + " This repository's binding to it had already changed, so it was left alone."
	}
	return r
}

// StoreWarning is the one line shown to the user when the binding store could
// not be opened or read: a real Go error, not a binding that is absent or
// unusable, which Project reports itself. The hook cannot tell whether the
// repository is bound, so it cannot project anything, and it says so instead of
// staying silent. The error text is sanitized and bounded like every detail.
func StoreWarning(err error) string {
	return "labdrian: the workflow binding store could not be read" + detailIn(err.Error()) +
		"; no workflow is projected, and if this repository is bound its workflow is not being followed in this session."
}

// PanicWarning is the one line shown to the user when the hook recovered from
// an internal panic. The panic value is sanitized and cut to a short length, so
// it can never spread over lines or fill the screen; the full value goes to
// stderr, where whoever debugs the hook reads it.
func PanicWarning(recovered any) string {
	return "labdrian: the projection hook hit an internal error (" + clip(sanitizeLine(fmt.Sprint(recovered)), maxPanicRunes) +
		"); nothing was projected this time and the prompt was not affected."
}

// Project decides what a session is told about the workflow its repository is
// bound to.
//
//   - No binding: nothing. An unbound repository is not the projection's
//     business, so it says nothing at all.
//   - A binding that is foreign, malformed, or unavailable: one warning, and
//     nothing is projected or removed. The store never touches a file that is
//     not ours, so neither does this.
//   - An owned binding to a workflow that is absent, foreign, malformed,
//     drifted, or unavailable, or that has a status this package does not know:
//     one warning, nothing is projected, and Unbind is never set. An unknown
//     state is not a closed workflow, and only a closed workflow is unbound.
//   - An owned binding to a closed workflow: a one-line context saying so, and
//     Unbind, so the next prompt is silent.
//   - An owned binding to a created, running, or paused workflow: the context,
//     bounded to MaxContextBytes.
//
// It is pure: it reads nothing but its argument, and it never fails.
func Project(in ProjectionInput) ProjectionResult {
	switch in.Binding.Classification {
	case ClassificationAbsent:
		return ProjectionResult{}
	case ClassificationOwned:
		return projectOwned(in.Binding.Binding, in.Workflow)
	default:
		return ProjectionResult{Warning: bindingWarning(in.Binding)}
	}
}

func projectOwned(b Binding, w *workflow.Loaded) ProjectionResult {
	if w == nil || w.Classification != workflow.ClassificationOwned {
		return ProjectionResult{Warning: workflowWarning(b, w)}
	}
	switch w.State.Status {
	case workflow.StatusClosed:
		note := closedNote(b, w.State)
		return ProjectionResult{Context: note, Unbind: true, closedNote: note}
	case workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused:
		return ProjectionResult{Context: boundContext(buildContext(b, *w))}
	default:
		return ProjectionResult{Warning: statusWarning(b, w.State.Status)}
	}
}

// --- warnings and the closed note -------------------------------------------

func bindingWarning(l Loaded) string {
	return "labdrian: this repository's workflow binding is " + sanitizeLine(string(l.Classification)) + detailIn(l.Detail) +
		"; no workflow is projected. Run 'labdrian workflow binding' to inspect it. " +
		"'labdrian workflow unbind' refuses to touch a binding file that is not usable, so fix or move that file by hand."
}

// workflowRef names the bound workflow in a message.
func workflowRef(b Binding) string {
	return "workflow " + sanitizeLine(b.WorkflowID) + " of project " + sanitizeLine(b.ProjectID)
}

// inspectAdvice tells the user how to look at a bound workflow and how to stop
// following it.
func inspectAdvice(b Binding) string {
	return "Run 'labdrian workflow status --project " + sanitizeLine(b.ProjectID) + " --workflow " + sanitizeLine(b.WorkflowID) +
		"' to inspect it, or 'labdrian workflow unbind' to stop following it."
}

func workflowWarning(b Binding, w *workflow.Loaded) string {
	var problem string
	switch {
	case w == nil:
		problem = "is unavailable (it was not loaded)"
	case w.Classification == workflow.ClassificationAbsent:
		problem = "does not exist"
	case w.Classification == workflow.ClassificationForeign,
		w.Classification == workflow.ClassificationMalformed,
		w.Classification == workflow.ClassificationDrifted,
		w.Classification == workflow.ClassificationUnavailable:
		problem = "is " + string(w.Classification) + detailIn(w.Detail)
	default:
		problem = "is in an unrecognized state (" + sanitizeLine(string(w.Classification)) + ")" + detailIn(w.Detail)
	}
	return "labdrian: " + workflowRef(b) + ", which this repository is bound to, " + problem + "; nothing is projected. " + inspectAdvice(b)
}

func statusWarning(b Binding, status workflow.Status) string {
	return "labdrian: " + workflowRef(b) + ", which this repository is bound to, has an unrecognized status " +
		strconv.Quote(sanitizeLine(string(status))) + "; nothing is projected. " + inspectAdvice(b)
}

// closedNote is the start of the one line a session gets when its workflow has
// closed. It claims nothing about the binding: the caller removes it after this
// text is built, the removal is best effort, and AfterUnbind adds what happened.
func closedNote(b Binding, s workflow.State) string {
	outcome := ""
	if s.CloseOutcome == workflow.OutcomeCompleted || s.CloseOutcome == workflow.OutcomeAbandoned {
		outcome = " (" + string(s.CloseOutcome) + ")"
	}
	return "labdrian workflow projection: " + workflowRef(b) + " is closed" + outcome + ". No workflow is projected."
}

// detailIn renders an explanation as " (text)", or nothing when there is none.
func detailIn(detail string) string {
	text := clip(sanitizeLine(detail), maxDetailRunes)
	if text == "" {
		return ""
	}
	return " (" + text + ")"
}

// --- the context ------------------------------------------------------------

// buildContext renders the context of an open (created, running, or paused)
// workflow. The lines whose length the workflow log alone decides (the
// unavailable dependencies, the recorded stages) come last, so that if the
// whole has to be cut to MaxContextBytes it is only ever those that go.
func buildContext(b Binding, w workflow.Loaded) string {
	state := w.State
	wf, project := sanitizeLine(b.WorkflowID), sanitizeLine(b.ProjectID)
	flags := "--project " + project + " --workflow " + wf

	lines := []string{
		"labdrian workflow projection: this repository follows the workflow below. Its state is read from the workflow log on disk, not from this session.",
		"workflow: " + wf + " (project: " + project + ")",
		"profile: " + orElse(sanitizeLine(state.Profile), blankField),
	}

	switch state.Status {
	case workflow.StatusCreated:
		lines = append(lines, "status: created (not started yet: run 'labdrian workflow start "+flags+"' before recording stages)")
	case workflow.StatusPaused:
		lines = append(lines,
			"status: paused",
			"PAUSED: this workflow is paused, so do not advance it (record no stage, run no verify) until it is resumed with: labdrian workflow resume "+flags)
	default:
		lines = append(lines, "status: "+string(state.Status))
	}

	lines = append(lines, "goal: "+orElse(sanitizeLine(state.GoalID), blankField)+" (digest "+sanitizeLine(prefixRunes(state.GoalDigest, goalDigestRunes))+")")

	recorded := state.Stages
	current := "none yet"
	if len(recorded) > 0 {
		current = orElse(sanitizeLine(recorded[len(recorded)-1]), blankField)
	}
	lines = append(lines, "current stage: "+current, "next stage: "+nextStage(state.Profile, recorded))

	lines = append(lines, memoryPlanLines(state.Profile, b.ProjectID, state.GoalID)...)
	lines = append(lines, claudeCapabilityLine())
	lines = append(lines, "progress commands: labdrian workflow stage "+flags+" --stage <name>; "+
		"labdrian workflow verify "+flags+" --goal <goal file>; "+
		"labdrian workflow close "+flags+" --outcome completed|abandoned")

	if unavailable := unavailableDependencies(w.Events); len(unavailable) > 0 {
		lines = append(lines, "unavailable dependencies at the last recorded event: "+strings.Join(unavailable, ", "))
	}
	names := make([]string, len(recorded))
	for i, stage := range recorded {
		names[i] = orElse(sanitizeLine(stage), blankField)
	}
	listed := "none"
	if len(names) > 0 {
		listed = strings.Join(names, ", ")
	}
	lines = append(lines, "stages recorded ("+strconv.Itoa(len(recorded))+"): "+listed)

	return strings.Join(lines, "\n")
}

// nextStage names the stage to record next. The workflow log never says: the
// lifecycle admits exactly the profile's next declared stage, so the answer is
// the entry of the profile's list after the ones recorded. When the profile does
// not resolve, or the recorded stages are not a prefix of its list (a log that
// was edited by hand), there is no next stage to name, and the line says so
// instead of guessing.
func nextStage(profileName string, recorded []string) string {
	profile, err := workflowprofile.Resolve(profileName)
	if err != nil {
		return "not available: profile " + strconv.Quote(sanitizeLine(profileName)) + " does not resolve, so stage guidance is omitted"
	}
	if len(recorded) > len(profile.Stages) {
		return "not available: the recorded stages do not follow the declared order of profile " + strconv.Quote(profile.Name) + ", so stage guidance is omitted"
	}
	for i, stage := range recorded {
		if stage != profile.Stages[i].Name {
			return "not available: the recorded stages do not follow the declared order of profile " + strconv.Quote(profile.Name) + ", so stage guidance is omitted"
		}
	}
	if len(recorded) == len(profile.Stages) {
		return "none: every declared stage is recorded"
	}
	return sanitizeLine(profile.Stages[len(recorded)].Name)
}

// memoryPlanLines renders the memory plan of the workflow's profile: the profile
// ceiling, resolved for the project and goal, exactly as 'memory plan' prints it.
// The plan is only a description of what may be read. It is stated as such,
// because nothing here queries or enforces anything.
func memoryPlanLines(profileName, projectID, goalID string) []string {
	unavailable := func(err error) []string {
		return []string{"memory plan: not available: " + clip(sanitizeLine(err.Error()), maxDetailRunes)}
	}
	directive, err := memoryscope.DefaultFor(profileName)
	if err != nil {
		return unavailable(err)
	}
	plan, err := memoryscope.Resolve(directive, projectID, goalID)
	if err != nil {
		return unavailable(err)
	}
	sources := make([]string, len(plan.Sources))
	for i, source := range plan.Sources {
		sources[i] = string(source)
	}
	lines := []string{"memory plan (read-only: it executes no query and grants no memory write): " +
		"scope=" + string(plan.Scope) +
		" sources=" + orElse(strings.Join(sources, ","), noneField) +
		" project_id=" + orElse(sanitizeLine(plan.Filters.ProjectID), noneField) +
		" goal_id=" + orElse(sanitizeLine(plan.Filters.GoalID), noneField) +
		" write=" + plan.Write}
	if len(plan.OmittedFilters) > 0 {
		lines = append(lines, "omitted filters: "+strings.Join(plan.OmittedFilters, ", "))
	}
	return lines
}

// claudeCapabilityLine renders what the engine declares Claude Code does not
// fully support, from the declaration itself.
func claudeCapabilityLine() string {
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		return "capability limits (" + capability.TargetClaude + "): not available: " + clip(sanitizeLine(err.Error()), maxDetailRunes)
	}
	return capabilityLimitsLine(d)
}

// capabilityLimitsLine lists the claims of d that are not supported, as
// name=status in the declaration's order. It is generated from the declaration,
// never written by hand, so it changes with it.
func capabilityLimitsLine(d capability.Declaration) string {
	var limits []string
	for _, c := range d.Claims {
		if c.Status != capability.Supported {
			limits = append(limits, string(c.Capability)+"="+string(c.Status))
		}
	}
	prefix := "capability limits (" + d.Target + "): "
	if len(limits) == 0 {
		return prefix + "none: every capability is supported"
	}
	return prefix + strings.Join(limits, ", ")
}

// unavailableDependencies names the capabilities the last recorded event
// observed as unavailable, in the order it recorded them. Earlier events are
// not consulted: each event probes every dependency again.
func unavailableDependencies(events []workflow.WorkflowEvent) []string {
	if len(events) == 0 {
		return nil
	}
	var names []string
	for _, o := range events[len(events)-1].Observations {
		if o.Status == workflow.ObservationUnavailable {
			names = append(names, orElse(sanitizeLine(o.Capability), blankField))
		}
	}
	return names
}

// orElse returns s, or placeholder when s is empty, so a line never has a hole.
// The placeholder says why the field is empty: "(blank)" for a field that
// should have had a value and lost it in sanitizing, "none" for one that is
// legitimately absent.
func orElse(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}

const (
	blankField = "(blank)"
	noneField  = "none"
)

// --- the hook output --------------------------------------------------------

// userPromptSubmitOutput is the JSON object a UserPromptSubmit hook prints.
// additionalContext must sit inside hookSpecificOutput, next to the event name:
// at the top level Claude Code ignores it without a word.
type userPromptSubmitOutput struct {
	HookSpecificOutput *userPromptSubmitSpecific `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string                    `json:"systemMessage,omitempty"`
}

type userPromptSubmitSpecific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// UserPromptSubmitOutput renders r as the one JSON object a UserPromptSubmit hook
// writes to stdout, followed by a newline: the context as additionalContext
// inside hookSpecificOutput, and the warning as systemMessage. A part that is
// empty is left out, and a result with neither yields no output at all, which
// Claude Code reads as "nothing to add". The JSON keeps <, >, and & as they are,
// so the transcript stays readable.
func (r ProjectionResult) UserPromptSubmitOutput() ([]byte, error) {
	if r.Context == "" && r.Warning == "" {
		return nil, nil
	}
	out := userPromptSubmitOutput{SystemMessage: r.Warning}
	if r.Context != "" {
		out.HookSpecificOutput = &userPromptSubmitSpecific{HookEventName: HookEventUserPromptSubmit, AdditionalContext: r.Context}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("projection: encode hook output: %w", err)
	}
	return buf.Bytes(), nil
}

// --- text hygiene -----------------------------------------------------------

// sanitizeLine makes s safe to place inside one line of the projected text. The
// workflow log validates the shape of its free-text fields, not their
// characters, so a log that was edited by hand can carry anything, and this
// text goes into a model's context and a user's terminal.
//
// White space of every kind, line breaks included, collapses to a single space
// (and is trimmed at both ends), so a field can never start a new line. Every
// other character that is not printable is removed: control characters,
// Unicode format characters (zero-width, bidirectional, tag), private-use and
// unassigned code points, and bytes that are not UTF-8.
func sanitizeLine(s string) string {
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = b.Len() > 0
		case unicode.IsPrint(r):
			if space {
				b.WriteByte(' ')
				space = false
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// prefixRunes returns the first n characters of s (all of s when it is shorter).
func prefixRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// clip returns s cut to at most maxRunes characters, with "..." when it was cut.
func clip(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return prefixRunes(s, maxRunes) + "..."
}

// boundContext cuts s to MaxContextBytes, ending it with truncationMarker, and
// leaves it alone when it fits. The cut falls on a character boundary.
func boundContext(s string) string {
	if len(s) <= MaxContextBytes {
		return s
	}
	cut := MaxContextBytes - len(truncationMarker)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncationMarker
}
