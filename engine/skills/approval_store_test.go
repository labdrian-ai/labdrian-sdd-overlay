package skills

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The approval of a global skill is judged from what an ApprovalRecordStore says and from nothing
// else: these tests hand the domain a store that is no file system at all.

// memoryApprovals holds SKILL.md files and records by the path the domain names them with.
type memoryApprovals struct {
	skills              map[string][]byte
	records             map[string][]byte
	skillErr, recordErr error
	asked               []string
}

func (m *memoryApprovals) ReadSkill(sourceRoot, path string) ([]byte, error) {
	m.asked = append(m.asked, "skill "+path)
	if m.skillErr != nil {
		return nil, m.skillErr
	}
	if data, ok := m.skills[path]; ok {
		return data, nil
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

func approvalRecordFor(t *testing.T, id string, skillMD []byte) []byte {
	t.Helper()
	data, err := SerializeApprovalRecord(ApprovalRecord{Skill: id, SHA256: SkillDigest(skillMD), ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCheckApprovalsJudgesFromTheStoreAlone(t *testing.T) {
	approved, stale, unapproved := []byte("approved skill"), []byte("changed after approval"), []byte("never approved")
	store := &memoryApprovals{
		skills: map[string][]byte{"good": approved, "stale": stale, "bare": unapproved},
		records: map[string][]byte{
			"good":  approvalRecordFor(t, "good", approved),
			"stale": approvalRecordFor(t, "stale", []byte("what was approved")),
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
	if want := "skill good|record good|skill stale|record stale|skill bare|record bare|skill gone"; strings.Join(store.asked, "|") != want {
		t.Errorf("the store was asked %v, want the SKILL.md of each skill, then its record, in the order of the registry", store.asked)
	}
}

func TestCheckApprovalsTakesAnUnreadableSkillOrRecordForUnverifiableNotForAbsent(t *testing.T) {
	boom := errors.New("input/output error")
	skillFails := &memoryApprovals{skillErr: boom}
	divs, sum := CheckApprovals(globalRegistry("a"), "/o", skillFails)
	if len(divs) != 1 || divs[0].Class != DivApprovalUnverifiable || sum.SkillFileMissing != 0 {
		t.Errorf("a SKILL.md that cannot be read: %+v, %+v, want one unverifiable divergence", divs, sum)
	}
	recordFails := &memoryApprovals{skills: map[string][]byte{"a": []byte("x")}, recordErr: boom}
	divs, _ = CheckApprovals(globalRegistry("a"), "/o", recordFails)
	if len(divs) != 1 || divs[0].Class != DivApprovalUnverifiable || !strings.Contains(divs[0].Detail, "input/output error") {
		t.Errorf("a record that cannot be read: %+v, want one unverifiable divergence that says why", divs)
	}
}

func TestReadApprovalStatusAsksTheStoreForTheRecordOnly(t *testing.T) {
	store := &memoryApprovals{}
	st, err := ReadApprovalStatus("/o", "alpha", []byte("md"), store)
	if err != nil || st.State != ApprovalAbsent {
		t.Fatalf("ReadApprovalStatus = %+v, %v, want absent", st, err)
	}
	if strings.Join(store.asked, "|") != "record alpha" {
		t.Errorf("the store was asked %v, want the record of alpha and nothing else", store.asked)
	}
}

func TestTheVerbsThatJudgeApprovalRefuseWithoutAStoreWired(t *testing.T) {
	registry := filepath.Join(t.TempDir(), "skills.registry.yaml")
	writeTestFile(t, registry, "version: \"1\"\nskills:\n")
	for _, verb := range []string{"approve"} {
		deps := testDeps(nil, func(string) ([]byte, error) { return nil, errors.New("must not be read") }, testRegistries(nil), nil, noopLocker{})
		deps.Approvals = nil
		var out, errOut bytes.Buffer
		code := -1
		SkillsCoreAt(verb, []string{verb, "--registry", registry, "--source-root", "/o/skills"}, deps, &out, &errOut, func(c int) { code = c })
		if want := "error: skills " + verb + ": no approval record store is wired, so it cannot tell whether a skill is approved\n"; code != 1 || errOut.String() != want || out.Len() != 0 {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1 and %q", verb, code, out.String(), errOut.String(), want)
		}
	}
}
