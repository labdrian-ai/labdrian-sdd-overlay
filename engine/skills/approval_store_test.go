package skills

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// The approval of a global skill is judged from what an ApprovalRecordStore says and from nothing
// else: these tests hand the domain a store that is no file system at all.

// memoryApprovals holds the files of skills and their records by the path the domain names them with.
type memoryApprovals struct {
	trees              map[string][]SkillEntry
	records            map[string][]byte
	treeErr, recordErr error
	asked              []string
}

func (m *memoryApprovals) ReadSkillTree(sourceRoot, path string) ([]SkillEntry, error) {
	m.asked = append(m.asked, "tree "+path)
	if m.treeErr != nil {
		return nil, m.treeErr
	}
	if entries, ok := m.trees[path]; ok {
		return entries, nil
	}
	return nil, fs.ErrNotExist
}

func (m *memoryApprovals) ReadRecord(sourceRoot, id string) ([]byte, error) {
	m.asked = append(m.asked, "record "+id)
	if m.recordErr != nil {
		return nil, m.recordErr
	}
	if data, ok := m.records[id]; ok {
		return data, nil
	}
	return nil, fs.ErrNotExist
}

func globalRegistry(ids ...string) Registry {
	var reg Registry
	reg.Version = "1"
	for _, id := range ids {
		reg.Skills = append(reg.Skills, Entry{ID: id, Path: id, Source: Source{Type: "custom"},
			Install: Install{DefaultScope: "global", Targets: []string{"claude"}}, Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"}})
	}
	return reg
}

// approvalRecordFor is the record of version 2 that approves exactly these files of skill id.
func approvalRecordFor(t *testing.T, id string, entries []SkillEntry) []byte {
	t.Helper()
	data, err := SerializeApprovalRecord(ApprovalRecord{Skill: id, SHA256: mustDigest(t, entries), ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCheckApprovalsJudgesFromTheStoreAlone(t *testing.T) {
	approved, stale, unapproved := skillMDOnly("approved skill"), skillMDOnly("changed after approval"), skillMDOnly("never approved")
	store := &memoryApprovals{
		trees: map[string][]SkillEntry{"good": approved, "stale": stale, "bare": unapproved},
		records: map[string][]byte{
			"good":  approvalRecordFor(t, "good", approved),
			"stale": approvalRecordFor(t, "stale", skillMDOnly("what was approved")),
		},
	}
	divs, sum := CheckApprovals(globalRegistry("good", "stale", "bare", "gone"), "/overlay/skills", store)
	if sum.Global != 4 || sum.Approved != 1 || sum.SkillFileMissing != 1 {
		t.Errorf("summary = %+v, want 4 global, 1 approved, 1 whose SKILL.md is not there", sum)
	}
	got := map[string]DivergenceClass{}
	for _, d := range divs {
		got[d.Path] = d.Class
	}
	if len(divs) != 2 || got["stale"] == "" || got["bare"] == "" || got["good"] != "" || got["gone"] != "" {
		t.Errorf("divergences = %+v, want the stale and the unapproved skill and no other", divs)
	}
	if want := "tree good|record good|tree stale|record stale|tree bare|record bare|tree gone"; strings.Join(store.asked, "|") != want {
		t.Errorf("the store was asked %v, want the files of each skill, then its record, in the order of the registry", store.asked)
	}
}

// What an approval covers is the domain's to say and the store's to report: a file the store lists
// beside SKILL.md is part of what the record must have approved.
func TestCheckApprovalsCoversEveryFileTheStoreReports(t *testing.T) {
	alone := skillMDOnly("skill")
	withScript := append(skillMDOnly("skill"), SkillEntry{Rel: "scripts/run.sh", Data: []byte("echo hi\n")})
	store := &memoryApprovals{
		trees:   map[string][]SkillEntry{"a": withScript},
		records: map[string][]byte{"a": approvalRecordFor(t, "a", alone)},
	}

	divs, sum := CheckApprovals(globalRegistry("a"), "/o", store)
	if len(divs) != 1 || divs[0].Class != DivApprovalStale || sum.Approved != 0 {
		t.Fatalf("a skill that gained a script after it was approved: %+v, %+v, want one APPROVAL_STALE", divs, sum)
	}

	store.records["a"] = approvalRecordFor(t, "a", withScript)
	divs, sum = CheckApprovals(globalRegistry("a"), "/o", store)
	if len(divs) != 0 || sum.Approved != 1 {
		t.Fatalf("a skill approved with its script: %+v, %+v, want it approved", divs, sum)
	}
}

func TestCheckApprovalsSetsAsideASkillWhoseTreeHoldsNoSKILLmd(t *testing.T) {
	store := &memoryApprovals{trees: map[string][]SkillEntry{"a": {{Rel: "README.md", Data: []byte("not a skill")}}}}
	divs, sum := CheckApprovals(globalRegistry("a"), "/o", store)
	if len(divs) != 0 || sum.SkillFileMissing != 1 {
		t.Errorf("a directory with no SKILL.md: %+v, %+v, want it set aside for the cross-checks to report", divs, sum)
	}
	if strings.Join(store.asked, "|") != "tree a" {
		t.Errorf("the store was asked %v, want its record left unread: nothing is being approved", store.asked)
	}
}

func TestCheckApprovalsTakesAnUnreadableSkillOrRecordForUnverifiableNotForAbsent(t *testing.T) {
	boom := errors.New("input/output error")
	skillFails := &memoryApprovals{treeErr: boom}
	divs, sum := CheckApprovals(globalRegistry("a"), "/o", skillFails)
	if len(divs) != 1 || divs[0].Class != DivApprovalUnverifiable || sum.SkillFileMissing != 0 {
		t.Errorf("the files of a skill that cannot be read: %+v, %+v, want one unverifiable divergence", divs, sum)
	}
	if len(divs) == 1 && !strings.Contains(divs[0].Detail, "input/output error") {
		t.Errorf("the divergence %+v does not say why", divs[0])
	}
	recordFails := &memoryApprovals{trees: map[string][]SkillEntry{"a": skillMDOnly("x")}, recordErr: boom}
	divs, _ = CheckApprovals(globalRegistry("a"), "/o", recordFails)
	if len(divs) != 1 || divs[0].Class != DivApprovalUnverifiable || !strings.Contains(divs[0].Detail, "input/output error") {
		t.Errorf("a record that cannot be read: %+v, want one unverifiable divergence that says why", divs)
	}
}

func TestReadApprovalStatusAsksTheStoreForTheRecordOnly(t *testing.T) {
	store := &memoryApprovals{}
	st, err := ReadApprovalStatus("/o", "alpha", skillMDOnly("md"), store)
	if err != nil || st.State != ApprovalAbsent {
		t.Fatalf("ReadApprovalStatus = %+v, %v, want absent", st, err)
	}
	if strings.Join(store.asked, "|") != "record alpha" {
		t.Errorf("the store was asked %v, want the record of alpha and nothing else", store.asked)
	}
}
