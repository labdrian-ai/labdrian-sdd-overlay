// Package workflow defines the standalone workflow lifecycle record: an
// append-only, hash-chained event log that references one Goal (by id and
// digest), one Workflow Profile (by name), and optionally one role chain,
// without reusing the role chain itself. It is pure data and a pure state
// machine: no filesystem access, no subprocess execution, and no clock
// reads happen in this package. Timestamps are supplied by the caller.
package workflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// EventVersion is the only WorkflowEvent wire version this package accepts.
const EventVersion = 1

// MaxIdentifierLength bounds every identifier this package validates
// (workflow_id, project_id, role_chain_id). The workflow store
// (engine/workflow's on-disk JSONL log) uses workflow_id and project_id as
// file path components — <state home>/labdrian/workflows/<project_id>/
// <workflow_id>.jsonl — so identifiers must be safe single path components on
// every supported platform: see ValidateIdentifier. 128 runes is far beyond
// any realistic slug, UUID, or content-addressed id (a UUID is 36
// characters, a SHA-256 hex digest 64) while still keeping every path this
// package builds well under common filesystem path-length limits (typically
// 255 bytes per component).
const MaxIdentifierLength = 128

// Maximum lengths for the free-text fields this package validates. These
// bound resource exhaustion from an untrusted event source (see Replay's
// doc comment): a caller decoding events from disk or from another process
// must not accept unbounded free text before these limits are enforced.
//
// GoalID and Stage are short caller-chosen names (a Goal's own goal_id, or
// one of a Workflow Profile's declared stage names), so 256 runes is
// generous headroom over any realistic name while still catching a
// runaway value. Capability names are shorter still (a handful of fixed
// strings this package and its callers define, such as "gentle-ai-review"),
// so 128 runes is ample. Reason and Detail are free-form human explanation
// (a close reason, a classification detail, an observation detail) that may
// legitimately run to a few sentences or short paragraph, so 4096 runes (a
// few paragraphs of text) leaves room for a genuine explanation without
// accepting an unbounded document.
const (
	MaxGoalIDLength                = 256
	MaxStageLength                 = 256
	MaxReasonLength                = 4096
	MaxObservationCapabilityLength = 128
	MaxObservationDetailLength     = 4096
)

// MaxEventBytes bounds the overall size of one WorkflowEvent JSON document
// that ParseWorkflowEvent will decode, independent of the field-level
// bounds above. It protects a caller that parses events from an untrusted
// source (disk, network, another process) from an arbitrarily large
// document before any field is even inspected. 64 KiB is a large multiple of
// the sum of every field-level bound above (well under 16 KiB even with
// every free-text field and every allowed observation populated at its
// maximum), so a legitimate event never approaches this ceiling; it exists
// purely to reject a hostile or corrupted document cheaply, before decoding.
const MaxEventBytes = 64 * 1024

// ErrEventTooLarge is returned by ParseWorkflowEvent when the input exceeds
// MaxEventBytes.
var ErrEventTooLarge = errors.New("workflow: event document exceeds the maximum size")

// Kind is one member of the closed WorkflowEvent kind vocabulary.
type Kind string

// The closed kind vocabulary.
const (
	KindCreated       Kind = "created"
	KindStarted       Kind = "started"
	KindPaused        Kind = "paused"
	KindResumed       Kind = "resumed"
	KindStageRecorded Kind = "stage_recorded"
	KindVerified      Kind = "verified"
	KindClosed        Kind = "closed"
)

// knownKinds lists every Kind in the closed vocabulary.
var knownKinds = map[Kind]bool{
	KindCreated:       true,
	KindStarted:       true,
	KindPaused:        true,
	KindResumed:       true,
	KindStageRecorded: true,
	KindVerified:      true,
	KindClosed:        true,
}

// Outcome is the closed vocabulary for a closed event's outcome.
type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomeAbandoned Outcome = "abandoned"
)

// Observation status vocabulary. A declared dependency that is unavailable
// is recorded as such; it is never silently approved or hidden.
const (
	ObservationAvailable   = "available"
	ObservationUnavailable = "unavailable"
)

// Observation is one recorded capability check attached to an event:
// whether a declared dependency (memory, runtime, auth, ...) was available
// when the event was produced.
type Observation struct {
	Capability string `json:"capability"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}

// Provenance is audit-only context about where an event was produced. It is
// not a lookup key: two worktrees of one project record different
// provenance while observing the same workflow (state lives outside the
// worktree, keyed by project_id).
type Provenance struct {
	WorktreeRoot string `json:"worktree_root"`
	GitHead      string `json:"git_head"`
}

// Checked is the verified-kind payload: the exact digests and profile name
// a structural verify checked against.
type Checked struct {
	ChainDigest     string `json:"chain_digest"`
	GoalDigest      string `json:"goal_digest"`
	Profile         string `json:"profile"`
	RoleChainDigest string `json:"role_chain_digest,omitempty"`
}

// WorkflowEvent is one record of a workflow's append-only event log: data
// only, no execution authority. It binds to the previous record in its
// chain by prev_digest (see EventDigest and VerifyEvents), and carries
// exactly the payload fields its Kind requires; every other kind-specific
// field must be zero-valued (see Validate).
type WorkflowEvent struct {
	Version      int           `json:"version"`
	WorkflowID   string        `json:"workflow_id"`
	ProjectID    string        `json:"project_id"`
	Seq          int           `json:"seq"`
	PrevDigest   string        `json:"prev_digest"`
	Kind         Kind          `json:"kind"`
	At           string        `json:"at"`
	Provenance   Provenance    `json:"provenance"`
	Observations []Observation `json:"observations"`

	// created-only payload.
	GoalID      string `json:"goal_id,omitempty"`
	GoalDigest  string `json:"goal_digest,omitempty"`
	Profile     string `json:"profile,omitempty"`
	RoleChainID string `json:"role_chain_id,omitempty"`

	// stage_recorded-only payload.
	Stage string `json:"stage,omitempty"`

	// verified-only payload.
	Checked *Checked `json:"checked,omitempty"`

	// closed-only payload.
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// workflowEventFields lists every known top-level WorkflowEvent wire field
// across every kind's payload; the payload fields not used by a given kind
// must still be absent or zero (checked by Validate), but the field name
// itself is always allowed so strict parsing does not need per-kind field
// lists.
var workflowEventFields = []string{
	"version", "workflow_id", "project_id", "seq", "prev_digest", "kind", "at",
	"provenance", "observations",
	"goal_id", "goal_digest", "profile", "role_chain_id",
	"stage",
	"checked",
	"outcome", "reason",
}

var (
	provenanceFields  = []string{"worktree_root", "git_head"}
	observationFields = []string{"capability", "status", "detail"}
	checkedFields     = []string{"chain_digest", "goal_digest", "profile", "role_chain_digest"}
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// gitHeadPattern requires a full commit id: exactly 40 lowercase hex
// characters (a full SHA-1 object id) or exactly 64 (a full SHA-256 object
// id). Provenance.git_head exists specifically so a post-incident
// investigation can pin down the exact commit an event was produced from;
// an abbreviated id is not reliably unique and defeats that purpose.
var gitHeadPattern = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// identifierAllowed is the closed allowlist ValidateIdentifier enforces:
// ASCII letters, digits, '.', '_', and '-'. Every character outside this
// set is rejected, including control characters (e.g. CR/LF), ':' (which on
// Windows NTFS introduces an alternate data stream), path separators, and
// spaces.
var identifierAllowed = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseWorkflowEvent strictly parses one WorkflowEvent record: bounded
// overall size, valid UTF-8, no duplicate keys at any depth, no unknown
// fields at any depth (exact case), no trailing data, the pinned version,
// and every field shape via Validate.
func ParseWorkflowEvent(data []byte) (WorkflowEvent, error) {
	if len(data) > MaxEventBytes {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w: %d bytes exceeds the maximum of %d", ErrEventTooLarge, len(data), MaxEventBytes)
	}
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "workflow event", workflowEventFields); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}
	if err := checkNestedWorkflowFields(raw); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}

	var e WorkflowEvent
	if err := jsonstrict.DecodeStrict(data, "workflow event", &e); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}
	if err := e.Validate(); err != nil {
		return WorkflowEvent{}, fmt.Errorf("parse workflow event: %w", err)
	}
	return e, nil
}

// checkNestedWorkflowFields rejects unknown keys in every nested record
// object with exact-case matching: encoding/json matches struct fields
// without regard to case and the last matching key wins, so a case-variant
// key could carry a value a case-sensitive reader of the same bytes would
// never see.
func checkNestedWorkflowFields(raw map[string]json.RawMessage) error {
	isNull := func(v json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(v), []byte("null")) }

	if v, ok := raw["provenance"]; ok && !isNull(v) {
		if err := jsonstrict.CheckKnownFields(v, "workflow event provenance", provenanceFields); err != nil {
			return fmt.Errorf("provenance: %w", err)
		}
	}
	if v, ok := raw["checked"]; ok && !isNull(v) {
		if err := jsonstrict.CheckKnownFields(v, "workflow event checked", checkedFields); err != nil {
			return fmt.Errorf("checked: %w", err)
		}
	}
	v, ok := raw["observations"]
	if !ok || isNull(v) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(v, &items); err != nil {
		return fmt.Errorf("observations: %w", err)
	}
	for i, item := range items {
		if err := jsonstrict.CheckKnownFields(item, "workflow event observation", observationFields); err != nil {
			return fmt.Errorf("observations[%d]: %w", i, err)
		}
	}
	return nil
}

// Marshal returns the indented, human-readable JSON representation of a
// valid event: fields in contract order, two-space indentation, and exactly
// one trailing newline. This is a separate encoding from the canonical
// digest encoding: it is not the input EventDigest hashes, and the on-disk
// JSONL workflow store never writes this form (it writes MarshalLine's
// compact single-line form instead). Use Marshal only when a human will read
// the output directly.
func (e WorkflowEvent) Marshal() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, fmt.Errorf("marshal workflow event: %w", err)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal workflow event: %w", err)
	}
	return append(data, '\n'), nil
}

// MarshalLine returns the canonical digest encoding of a valid event as one
// compact JSON line plus exactly one trailing newline: this is the form the
// on-disk JSONL workflow store writes, and the bytes before the trailing
// newline are exactly what EventDigest hashes. It is distinct from Marshal's
// indented, human-readable encoding, which is not the digest input and is
// never written to the store.
func (e WorkflowEvent) MarshalLine() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, fmt.Errorf("marshal workflow event line: %w", err)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow event line: %w", err)
	}
	return append(data, '\n'), nil
}

// ValidateIdentifier checks that value is safe to use as a single file path
// component on every supported platform and as a stable lookup key: non-
// blank, at most MaxIdentifierLength runes, composed only of the characters
// in identifierAllowed (ASCII letters, digits, '.', '_', '-'), and not
// starting with '.' (which also excludes the "." and ".." segments). This
// allowlist additionally excludes control characters (e.g. CR/LF), ':'
// (which on Windows NTFS introduces an alternate data stream), spaces, and
// every path separator.
func ValidateIdentifier(name, value string) error {
	switch {
	case value == "":
		return fmt.Errorf("%s must not be blank", name)
	case len([]rune(value)) > MaxIdentifierLength:
		return fmt.Errorf("%s exceeds the maximum identifier length of %d runes", name, MaxIdentifierLength)
	case !identifierAllowed.MatchString(value):
		return fmt.Errorf("%s %q contains a character outside the allowed set [A-Za-z0-9._-]", name, value)
	case strings.HasPrefix(value, "."):
		return fmt.Errorf("%s %q must not start with '.'", name, value)
	}
	return nil
}

// Validate checks every field shape of e, including that exactly the
// payload fields its Kind requires are populated and every other
// kind-specific field is zero-valued. It is first-error-wins.
func (e WorkflowEvent) Validate() error {
	if e.Version != EventVersion {
		return fmt.Errorf("version must be %d, got %d", EventVersion, e.Version)
	}
	if err := ValidateIdentifier("workflow_id", e.WorkflowID); err != nil {
		return err
	}
	if err := ValidateIdentifier("project_id", e.ProjectID); err != nil {
		return err
	}
	if e.Seq < 0 {
		return fmt.Errorf("seq must be >= 0, got %d", e.Seq)
	}
	if e.Seq == 0 {
		if e.PrevDigest != "" {
			return fmt.Errorf("prev_digest must be empty at seq 0, got %q", e.PrevDigest)
		}
	} else if !sha256HexPattern.MatchString(e.PrevDigest) {
		return fmt.Errorf("prev_digest must be 64 lowercase hex characters at seq > 0, got %q", e.PrevDigest)
	}
	if !knownKinds[e.Kind] {
		return fmt.Errorf("kind %q is not a known workflow event kind", e.Kind)
	}
	if err := validateUTCTimestamp(e.At); err != nil {
		return err
	}
	if err := e.Provenance.validate(); err != nil {
		return err
	}
	if err := validateObservations(e.Observations); err != nil {
		return err
	}
	return e.validatePayload()
}

func (p Provenance) validate() error {
	if p.WorktreeRoot != "" && !filepath.IsAbs(p.WorktreeRoot) {
		return fmt.Errorf("provenance.worktree_root must be an absolute path or empty, got %q", p.WorktreeRoot)
	}
	if p.GitHead != "" && !gitHeadPattern.MatchString(p.GitHead) {
		return fmt.Errorf("provenance.git_head must be a lowercase hex string or empty, got %q", p.GitHead)
	}
	return nil
}

func validateObservations(observations []Observation) error {
	if observations == nil {
		return fmt.Errorf("observations must be a non-null array")
	}
	for i, o := range observations {
		if strings.TrimSpace(o.Capability) == "" {
			return fmt.Errorf("observations[%d].capability must not be blank", i)
		}
		if len([]rune(o.Capability)) > MaxObservationCapabilityLength {
			return fmt.Errorf("observations[%d].capability exceeds the maximum length of %d runes", i, MaxObservationCapabilityLength)
		}
		if o.Status != ObservationAvailable && o.Status != ObservationUnavailable {
			return fmt.Errorf("observations[%d].status must be %q or %q, got %q", i, ObservationAvailable, ObservationUnavailable, o.Status)
		}
		if len([]rune(o.Detail)) > MaxObservationDetailLength {
			return fmt.Errorf("observations[%d].detail exceeds the maximum length of %d runes", i, MaxObservationDetailLength)
		}
	}
	return nil
}

// validatePayload checks that e carries exactly the payload fields its Kind
// requires and that every other kind-specific field is zero-valued.
func (e WorkflowEvent) validatePayload() error {
	blankCreated := e.GoalID == "" && e.GoalDigest == "" && e.Profile == "" && e.RoleChainID == ""
	blankStage := e.Stage == ""
	blankChecked := e.Checked == nil
	blankClosed := e.Outcome == "" && e.Reason == ""

	switch e.Kind {
	case KindCreated:
		if !blankStage || !blankChecked || !blankClosed {
			return fmt.Errorf("kind %q must not carry stage, checked, or closed payload fields", e.Kind)
		}
		return e.validateCreatedPayload()
	case KindStageRecorded:
		if !blankCreated || !blankChecked || !blankClosed {
			return fmt.Errorf("kind %q must not carry created, checked, or closed payload fields", e.Kind)
		}
		if strings.TrimSpace(e.Stage) == "" {
			return fmt.Errorf("stage must not be blank")
		}
		if len([]rune(e.Stage)) > MaxStageLength {
			return fmt.Errorf("stage exceeds the maximum length of %d runes", MaxStageLength)
		}
		return nil
	case KindVerified:
		if !blankCreated || !blankStage || !blankClosed {
			return fmt.Errorf("kind %q must not carry created, stage, or closed payload fields", e.Kind)
		}
		return e.validateCheckedPayload()
	case KindClosed:
		if !blankCreated || !blankStage || !blankChecked {
			return fmt.Errorf("kind %q must not carry created, stage, or checked payload fields", e.Kind)
		}
		return e.validateClosedPayload()
	default: // started, paused, resumed: no payload.
		if !blankCreated || !blankStage || !blankChecked || !blankClosed {
			return fmt.Errorf("kind %q must not carry any kind-specific payload fields", e.Kind)
		}
		return nil
	}
}

func (e WorkflowEvent) validateCreatedPayload() error {
	if strings.TrimSpace(e.GoalID) == "" {
		return fmt.Errorf("goal_id must not be blank")
	}
	if len([]rune(e.GoalID)) > MaxGoalIDLength {
		return fmt.Errorf("goal_id exceeds the maximum length of %d runes", MaxGoalIDLength)
	}
	if !sha256HexPattern.MatchString(e.GoalDigest) {
		return fmt.Errorf("goal_digest must be 64 lowercase hex characters, got %q", e.GoalDigest)
	}
	if _, err := workflowprofile.Resolve(e.Profile); err != nil {
		return fmt.Errorf("profile %q is not a known workflow profile: %w", e.Profile, err)
	}
	if e.RoleChainID != "" {
		if err := ValidateIdentifier("role_chain_id", e.RoleChainID); err != nil {
			return err
		}
	}
	return nil
}

func (e WorkflowEvent) validateCheckedPayload() error {
	if e.Checked == nil {
		return fmt.Errorf("checked must be present for kind %q", KindVerified)
	}
	c := *e.Checked
	if !sha256HexPattern.MatchString(c.ChainDigest) {
		return fmt.Errorf("checked.chain_digest must be 64 lowercase hex characters, got %q", c.ChainDigest)
	}
	if !sha256HexPattern.MatchString(c.GoalDigest) {
		return fmt.Errorf("checked.goal_digest must be 64 lowercase hex characters, got %q", c.GoalDigest)
	}
	if _, err := workflowprofile.Resolve(c.Profile); err != nil {
		return fmt.Errorf("checked.profile %q is not a known workflow profile: %w", c.Profile, err)
	}
	if c.RoleChainDigest != "" && !sha256HexPattern.MatchString(c.RoleChainDigest) {
		return fmt.Errorf("checked.role_chain_digest must be 64 lowercase hex characters or empty, got %q", c.RoleChainDigest)
	}
	return nil
}

func (e WorkflowEvent) validateClosedPayload() error {
	switch Outcome(e.Outcome) {
	case OutcomeCompleted:
		if e.Reason != "" {
			return fmt.Errorf("reason must be empty when outcome is %q", OutcomeCompleted)
		}
	case OutcomeAbandoned:
		if strings.TrimSpace(e.Reason) == "" {
			return fmt.Errorf("reason must not be blank when outcome is %q", OutcomeAbandoned)
		}
		if len([]rune(e.Reason)) > MaxReasonLength {
			return fmt.Errorf("reason exceeds the maximum length of %d runes", MaxReasonLength)
		}
	default:
		return fmt.Errorf("outcome must be %q or %q, got %q", OutcomeCompleted, OutcomeAbandoned, e.Outcome)
	}
	return nil
}

// validateUTCTimestamp requires an RFC3339 timestamp expressed in UTC (a "Z"
// offset), so two nodes recording the same instant produce byte-identical
// "at" values.
func validateUTCTimestamp(value string) error {
	if !strings.HasSuffix(value, "Z") {
		return fmt.Errorf("at must be an RFC3339 UTC timestamp ending in \"Z\", got %q", value)
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("at must be a valid RFC3339 timestamp: %w", err)
	}
	return nil
}

// EventDigest returns the SHA-256 hex digest of e's canonical digest
// encoding: the compact (non-indented) JSON bytes of e with no trailing
// newline, exactly the bytes MarshalLine writes before its own trailing
// newline. This is the one canonical encoding used for hashing; Marshal's
// indented form is a separate, human-readable encoding and is never hashed.
// The digest is what the next record's prev_digest chains to (see
// VerifyEvents). It is deterministic: the same event always produces the
// same digest, and changing any field changes the digest.
func EventDigest(e WorkflowEvent) (string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("workflow event digest: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyEvents checks one workflow's event log: every event parses and
// validates, every event shares one workflow_id and one project_id, seq is
// contiguous starting at 0, and each event's prev_digest equals the
// EventDigest of the event immediately before it. It does not check
// lifecycle-transition legality; use Replay (or CheckTransition per event)
// for that. It is first-error-wins and performs no I/O.
func VerifyEvents(events []WorkflowEvent) error {
	if len(events) == 0 {
		return fmt.Errorf("workflow: verify events: at least one event is required")
	}
	workflowID := events[0].WorkflowID
	projectID := events[0].ProjectID
	var prevDigest string
	for i, e := range events {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("workflow: verify events: event %d: %w", i, err)
		}
		if e.Seq != i {
			return fmt.Errorf("workflow: verify events: event at index %d has seq %d, want %d", i, e.Seq, i)
		}
		if e.WorkflowID != workflowID {
			return fmt.Errorf("workflow: verify events: event %d has workflow_id %q, want %q", i, e.WorkflowID, workflowID)
		}
		if e.ProjectID != projectID {
			return fmt.Errorf("workflow: verify events: event %d has project_id %q, want %q", i, e.ProjectID, projectID)
		}
		if i == 0 {
			if e.PrevDigest != "" {
				return fmt.Errorf("workflow: verify events: event 0 prev_digest must be empty, got %q", e.PrevDigest)
			}
		} else if e.PrevDigest != prevDigest {
			return fmt.Errorf("workflow: verify events: event %d prev_digest %q does not match event %d digest %q", i, e.PrevDigest, i-1, prevDigest)
		}
		digest, err := EventDigest(e)
		if err != nil {
			return fmt.Errorf("workflow: verify events: event %d: %w", i, err)
		}
		prevDigest = digest
	}
	return nil
}
