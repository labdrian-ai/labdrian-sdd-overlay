package roles

import "testing"

func mustParse(t *testing.T, data string) RoleHandoff {
	t.Helper()
	h, err := ParseRoleHandoff([]byte(data))
	if err != nil {
		t.Fatalf("ParseRoleHandoff() = %v, want nil\ndata: %s", err, data)
	}
	return h
}

func recordJSON(seq int, prevSHA, from, to, status, resumeReason string) string {
	statusField := `"status": "` + status + `"`
	if resumeReason != "" {
		statusField += `, "resume_reason": "` + resumeReason + `"`
	}
	return `{
  "version": 1,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "chain_id": "chain-1",
  "seq": ` + itoa(seq) + `,
  "from_role": "` + from + `",
  "to_role": "` + to + `",
  "prev_sha256": "` + prevSHA + `",
  "payload_kind": "diff",
  "payload_sha256": "` + validDigest + `",
  "evidence": [],
  "context": {"summary": "s", "decisions": [], "open_questions": []},
  ` + statusField + `
}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func chainRecord(t *testing.T, data string) ChainRecord {
	t.Helper()
	return ChainRecord{Raw: []byte(data), Handoff: mustParse(t, data)}
}

// chainToBuilder builds a valid two-record chain ending with a completed
// handoff to builder: seq1 shaper->estimator, seq2 estimator->builder.
func chainToBuilder(t *testing.T) (c1, c2 ChainRecord) {
	t.Helper()
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 = chainRecord(t, r1)
	r2 := recordJSON(2, sha256Hex(c1.Raw), "estimator", "builder", "completed", "")
	c2 = chainRecord(t, r2)
	return c1, c2
}

// chainToDelivery builds a valid four-record chain reaching a completed
// handoff to delivery: shaper->estimator->builder->reviewer->delivery.
func chainToDelivery(t *testing.T) []ChainRecord {
	t.Helper()
	c1, c2 := chainToBuilder(t)
	r3 := recordJSON(3, sha256Hex(c2.Raw), "builder", "reviewer", "completed", "")
	c3 := chainRecord(t, r3)
	r4 := recordJSON(4, sha256Hex(c3.Raw), "reviewer", "delivery", "completed", "")
	c4 := chainRecord(t, r4)
	return []ChainRecord{c1, c2, c3, c4}
}

func TestVerifyChainAcceptsWellFormedChain(t *testing.T) {
	if err := VerifyChain(chainToDelivery(t)); err != nil {
		t.Fatalf("VerifyChain() = %v, want nil", err)
	}
}

func TestVerifyChainRejectsBadFirstRecordFromRole(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "builder", "sweeper", "completed", "")
	c1 := chainRecord(t, r1)
	if err := VerifyChain([]ChainRecord{c1}); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: seq 1 from_role must be prototyper or shaper")
	}
}

func TestVerifyChainRejectsBadFirstRecordPrevSHA(t *testing.T) {
	r1 := recordJSON(1, validDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	if err := VerifyChain([]ChainRecord{c1}); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: seq 1 prev_sha256 must be the empty digest")
	}
}

func TestVerifyChainRejectsBrokenHash(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	r2 := recordJSON(2, otherDigest, "estimator", "builder", "completed", "")
	c2 := chainRecord(t, r2)
	if err := VerifyChain([]ChainRecord{c1, c2}); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: prev_sha256 does not match the previous record's bytes")
	}
}

func TestVerifyChainRejectsSeqGap(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	r3 := recordJSON(3, sha256Hex(c1.Raw), "estimator", "builder", "completed", "")
	c3 := chainRecord(t, r3)
	if err := VerifyChain([]ChainRecord{c1, c3}); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: seq must be contiguous")
	}
}

func TestVerifyChainRejectsFromRoleNotMatchingPreviousToRole(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	r2 := recordJSON(2, sha256Hex(c1.Raw), "builder", "reviewer", "completed", "")
	c2 := chainRecord(t, r2)
	if err := VerifyChain([]ChainRecord{c1, c2}); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: from_role must equal previous to_role")
	}
}

func TestVerifyChainAllowsRepeatAfterInterrupted(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	r2 := recordJSON(2, sha256Hex(c1.Raw), "estimator", "builder", "interrupted", "network down")
	c2 := chainRecord(t, r2)
	r3 := recordJSON(3, sha256Hex(c2.Raw), "estimator", "builder", "completed", "")
	c3 := chainRecord(t, r3)
	if err := VerifyChain([]ChainRecord{c1, c2, c3}); err != nil {
		t.Fatalf("VerifyChain() = %v, want nil: a repeat of the interrupted transition is allowed", err)
	}
}

func TestVerifyChainRejectsNonRepeatAfterInterrupted(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	r2 := recordJSON(2, sha256Hex(c1.Raw), "estimator", "builder", "interrupted", "network down")
	c2 := chainRecord(t, r2)
	r3 := recordJSON(3, sha256Hex(c2.Raw), "builder", "reviewer", "completed", "")
	c3 := chainRecord(t, r3)
	if err := VerifyChain([]ChainRecord{c1, c2, c3}); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: after an interrupted record the next must repeat from/to")
	}
}

func TestVerifyChainRejectsRecordAfterTerminalDelivery(t *testing.T) {
	chain := chainToDelivery(t)
	last := chain[len(chain)-1]
	badNext := ChainRecord{
		Handoff: RoleHandoff{
			Version: HandoffVersion, ProjectID: "proj-1", GoalID: "goal-1", ChainID: "chain-1",
			Seq: last.Handoff.Seq + 1, FromRole: RoleReviewer, ToRole: RoleBuilder,
			PrevSHA256: sha256Hex(last.Raw), PayloadKind: "diff", PayloadSHA256: validDigest,
			Evidence: []Evidence{}, Context: Context{Summary: "s", Decisions: []string{}, OpenQuestions: []string{}},
			Status: StatusCompleted,
		},
	}
	if err := VerifyChain(append(append([]ChainRecord{}, chain...), badNext)); err == nil {
		t.Fatalf("VerifyChain() = nil, want error: nothing may follow a terminal completed-delivery record")
	}
}

func TestResumeReportsRoleFromLastRecord(t *testing.T) {
	c1, c2 := chainToBuilder(t)
	state, err := Resume([]ChainRecord{c1, c2})
	if err != nil {
		t.Fatalf("Resume() = %v, want nil", err)
	}
	if state.Role != RoleBuilder || state.Interrupted || state.Terminal {
		t.Fatalf("Resume() = %+v, want role=builder, interrupted=false, terminal=false", state)
	}
}

func TestResumeReportsInterruptedRoleAndReason(t *testing.T) {
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	c1 := chainRecord(t, r1)
	r2 := recordJSON(2, sha256Hex(c1.Raw), "estimator", "builder", "interrupted", "network down")
	c2 := chainRecord(t, r2)
	state, err := Resume([]ChainRecord{c1, c2})
	if err != nil {
		t.Fatalf("Resume() = %v, want nil", err)
	}
	if state.Role != RoleBuilder || !state.Interrupted || state.ResumeReason != "network down" {
		t.Fatalf("Resume() = %+v, want role=builder, interrupted=true, reason=%q", state, "network down")
	}
}

func TestResumeReportsTerminalAtDelivery(t *testing.T) {
	state, err := Resume(chainToDelivery(t))
	if err != nil {
		t.Fatalf("Resume() = %v, want nil", err)
	}
	if !state.Terminal || state.Role != RoleDelivery {
		t.Fatalf("Resume() = %+v, want terminal=true, role=delivery", state)
	}
}

func TestResumeRejectsEmptyChain(t *testing.T) {
	if _, err := Resume(nil); err == nil {
		t.Fatalf("Resume(nil) = nil, want error")
	}
}

func TestNextReturnsAllowedRolesFromCurrentPosition(t *testing.T) {
	c1, c2 := chainToBuilder(t)
	next, err := Next([]ChainRecord{c1, c2})
	if err != nil {
		t.Fatalf("Next() = %v, want nil", err)
	}
	want := []Role{RoleSweeper, RolePolisher, RoleReviewer}
	if len(next) != len(want) {
		t.Fatalf("Next() = %v, want %v", next, want)
	}
	for i := range want {
		if next[i] != want[i] {
			t.Fatalf("Next() = %v, want %v", next, want)
		}
	}
}

func TestNextReturnsEmptyAtTerminal(t *testing.T) {
	next, err := Next(chainToDelivery(t))
	if err != nil {
		t.Fatalf("Next() = %v, want nil", err)
	}
	if len(next) != 0 {
		t.Fatalf("Next() = %v, want empty at terminal", next)
	}
}
