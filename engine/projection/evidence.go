package projection

import (
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// ReadEvidence is what is recorded of a memory read that ran (C23 M7,
// docs/architecture/adr-c23-m7-read-evidence.md): the workflow it is recorded for, the tool, the
// source of the plan it read, and what the call named. Every value is one sanitized line of at
// most maxDetailRunes runes.
type ReadEvidence struct {
	// ProjectID and WorkflowID name the workflow the read ran under.
	ProjectID, WorkflowID string
	Tool                  string
	Source                memoryscope.Source
	// Project and Goal are what a read that names a project names (a longterm-mem query, an
	// Engram mem_search or mem_context); empty for any other read.
	Project, Goal string
	// IDs holds the record a read by id names (mem_get_observation, mem_timeline, a longterm-mem
	// get), and is empty otherwise.
	IDs []string
	// Skill is the procedural skill a Skill call, or a Read of its SKILL.md, used; empty otherwise.
	Skill string
}

// ReadRecord is one line of a workflow's memory-read evidence: the read, when the hook saw it, and
// what the runtime says of the call (its id and session) and of its response (a digest and a
// length, never the response itself).
type ReadRecord struct {
	Evidence       ReadEvidence
	At             time.Time
	CallID         string
	SessionID      string
	ResponseSHA256 string
	ResponseBytes  int64
}

// ReadEvidenceLog is where the memory-read evidence of each workflow is appended (decision 1: a
// sidecar of its own, beside the workflow log, which it never touches).
type ReadEvidenceLog interface {
	Append(ReadRecord) error
}

// Evidence is what is recorded of the call in in, which ran, and whether there is anything to
// record. It is exactly the memory reads the gate checks (decision 2): an Engram read, a
// longterm-mem query or get, or a use of a procedural skill the lock records, in a repository bound
// to a workflow of ours that is not closed. Every other call, a write and promote included, is
// not recorded, and nor is a use of a skill under a lock that could not be read: whether it was
// procedural cannot be told. It is total and pure.
func Evidence(in GateInput) (ReadEvidence, bool) {
	if in.Binding.Classification != ClassificationOwned {
		return ReadEvidence{}, false
	}
	w := in.Workflow
	if w == nil || w.Classification != workflow.ClassificationOwned {
		return ReadEvidence{}, false
	}
	switch w.State.Status {
	case workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused:
	default:
		return ReadEvidence{}, false
	}
	b := in.Binding.Binding
	ev := ReadEvidence{ProjectID: b.ProjectID, WorkflowID: b.WorkflowID, Tool: evidenceLine(in.Call.Name)}
	if read, ok := memoryReadOf(in.Call.Name); ok {
		ev.Source = read.source
		if read.check == checkSourceOnly {
			if id := evidenceLine(readArg(in.Call.ReadRecordID)); id != "" {
				ev.IDs = []string{id}
			}
		} else {
			q := in.Call.query()
			ev.Project, ev.Goal = evidenceLine(q.Project), evidenceLine(q.Goal)
		}
		return ev, true
	}
	use := skillUseOf(in.Call)
	if (!use.isSkill && !use.isRead) || in.Procedural.Unreadable != "" {
		return ReadEvidence{}, false
	}
	id, ok := in.Procedural.match(use)
	if !ok {
		return ReadEvidence{}, false
	}
	ev.Source, ev.Skill = memoryscope.SourceProceduralSkills, evidenceLine(id)
	return ev, true
}

// EvidenceWarning is the line the agent is given when the evidence of a read could not be written
// (decision 4): which read, which workflow, and why.
func EvidenceWarning(ev ReadEvidence, err error) string {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return "labdrian: the evidence of this memory read (" + evidenceLine(ev.Tool) + ") was not recorded for " +
		workflowRef(Binding{ProjectID: ev.ProjectID, WorkflowID: ev.WorkflowID}) + detailIn(detail) + "."
}

// evidenceLine is s as one sanitized line of at most maxDetailRunes runes.
func evidenceLine(s string) string { return clip(sanitizeLine(s), maxDetailRunes) }
