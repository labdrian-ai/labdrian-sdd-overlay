package skillsfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// byPath is the entries of a tree in the order of their paths: the store's order is its own, and
// no test here depends on it.
func byPath(entries []skills.SkillEntry) []skills.SkillEntry {
	sorted := append([]skills.SkillEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Rel < sorted[j].Rel })
	return sorted
}

// describe is one line per entry, for a failure message.
func describe(entries []skills.SkillEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Rel+" ("+e.Type.String()+", "+string(e.Data)+")")
	}
	return out
}

func TestApprovalsReadEveryFileOfTheSkillWithItsBytes(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	put(t, filepath.Join(root, "alpha", "scripts", "run.sh"), "echo hi\n")
	put(t, filepath.Join(root, "alpha", "references", "deep", "notes.md"), "notes")
	put(t, filepath.Join(root, "alpha", ".hidden"), "a dot-prefixed file is a file of the skill")
	put(t, filepath.Join(root, "alpha", "empty"), "")
	put(t, filepath.Join(root, "beta", "SKILL.md"), "another skill, not listed with alpha")
	if err := os.MkdirAll(filepath.Join(root, "alpha", "an-empty-directory"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := skillsfs.Approvals{}.ReadSkillTree(root, "alpha")
	if err != nil {
		t.Fatalf("ReadSkillTree: %v", err)
	}

	want := []skills.SkillEntry{
		{Rel: ".hidden", Data: []byte("a dot-prefixed file is a file of the skill")},
		{Rel: "SKILL.md", Data: []byte("skill bytes")},
		{Rel: "empty", Data: []byte{}},
		{Rel: "references/deep/notes.md", Data: []byte("notes")},
		{Rel: "scripts/run.sh", Data: []byte("echo hi\n")},
	}
	got := byPath(entries)
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %d files, slash-separated below the directory of the skill and no directory", describe(got), len(want))
	}
	for i := range want {
		if got[i].Rel != want[i].Rel || got[i].Type != 0 || string(got[i].Data) != string(want[i].Data) {
			t.Errorf("entry %d = %v, want %v", i, describe(got[i:i+1]), describe(want[i:i+1]))
		}
	}
}

// The one permission an approval covers is the owner's execute bit (mode & 0100), what git versions.
// A file that is executable only for the group or the others is not executable here, and the other
// bits (write, group, others) are not told at all: a different umask must not stale an approval of
// files nobody changed.
func TestApprovalsReportTheOwnerExecuteBitOfEachFileAndNoOtherBit(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	files := map[string]struct {
		mode       fs.FileMode
		executable bool
	}{
		"plain":                          {0o644, false},
		"private":                        {0o600, false},
		"writable-by-the-group":          {0o664, false},
		"executable-by-the-group-only":   {0o654, false},
		"executable-by-the-others-only":  {0o645, false},
		"executable-by-group-and-others": {0o655, false},
		"executable":                     {0o755, true},
		"executable-by-the-owner-only":   {0o700, true},
		"executable-and-group-writable":  {0o775, true},
		"executable-and-read-only":       {0o555, true},
	}
	for name, tc := range files {
		path := filepath.Join(root, "alpha", "scripts", name)
		put(t, path, "echo "+name+"\n")
		if err := os.Chmod(path, tc.mode); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := skillsfs.Approvals{}.ReadSkillTree(root, "alpha")
	if err != nil {
		t.Fatalf("ReadSkillTree: %v", err)
	}
	got := map[string]skills.SkillEntry{}
	for _, e := range entries {
		got[e.Rel] = e
	}
	for name, tc := range files {
		e, listed := got["scripts/"+name]
		switch {
		case !listed:
			t.Errorf("scripts/%s is not listed (%v)", name, describe(entries))
		case e.Executable != tc.executable:
			t.Errorf("scripts/%s with mode %04o: Executable = %v, want %v", name, tc.mode, e.Executable, tc.executable)
		case string(e.Data) != "echo "+name+"\n" || e.Type != 0:
			t.Errorf("scripts/%s = %v, want its bytes read whatever its mode", name, describe([]skills.SkillEntry{e}))
		}
	}
	if skill := got["SKILL.md"]; skill.Executable {
		t.Errorf("SKILL.md with mode 0644 is reported executable")
	}
}

func TestApprovalsReportTheRecordAtTheRootWithoutItsBytes(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	put(t, filepath.Join(root, "alpha", skills.ApprovalRecordName), `{"record": true}`)
	// Its mode is not read either: a digest leaves the record out whatever it holds or is.
	if err := os.Chmod(filepath.Join(root, "alpha", skills.ApprovalRecordName), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := skillsfs.Approvals{}.ReadSkillTree(root, "alpha")
	if err != nil {
		t.Fatalf("ReadSkillTree: %v", err)
	}
	var record *skills.SkillEntry
	for i := range entries {
		if entries[i].Rel == skills.ApprovalRecordName {
			record = &entries[i]
		}
	}
	if record == nil || !record.Type.IsRegular() || len(record.Data) != 0 || record.Executable {
		t.Fatalf("entries = %v, want the record listed as a regular file whose bytes and mode are not read: no approval covers them", describe(entries))
	}
}

// A record that cannot be read is told as that, by ReadRecord, and is not taken for a skill that
// cannot be read: listing the files of the skill does not open it.
func TestApprovalsListASkillWhoseRecordCannotBeRead(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	record := filepath.Join(root, "alpha", skills.ApprovalRecordName)
	put(t, record, `{"record": true}`)
	if err := os.Chmod(record, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(record, 0o644) })
	if _, err := os.ReadFile(record); err == nil {
		t.Skip("file permissions are not enforced for this process (root or an equivalent); an unreadable record cannot be simulated")
	}

	if _, err := (skillsfs.Approvals{}).ReadSkillTree(root, "alpha"); err != nil {
		t.Errorf("ReadSkillTree: %v, want the files of the skill listed whether or not its record can be read", err)
	}
	if _, err := (skillsfs.Approvals{}).ReadRecord(root, "alpha"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadRecord of a record that cannot be read: %v, want a failure that is not fs.ErrNotExist", err)
	}
}

func TestApprovalsReportALinkWithoutFollowingIt(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	put(t, filepath.Join(root, "elsewhere", "secret.md"), "outside the skill")
	if err := os.Symlink("SKILL.md", filepath.Join(root, "alpha", "alias.md")); err != nil {
		t.Skipf("no symbolic link here: %v", err)
	}
	if err := os.Symlink("nothing-is-here", filepath.Join(root, "alpha", "dangling.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "elsewhere"), filepath.Join(root, "alpha", "dir-link")); err != nil {
		t.Fatal(err)
	}

	entries, err := skillsfs.Approvals{}.ReadSkillTree(root, "alpha")
	if err != nil {
		t.Fatalf("ReadSkillTree: %v, want a link reported, not a failure", err)
	}

	got := map[string]skills.SkillEntry{}
	for _, e := range entries {
		got[e.Rel] = e
	}
	for _, name := range []string{"alias.md", "dangling.md", "dir-link"} {
		if e, ok := got[name]; !ok || e.Type&fs.ModeSymlink == 0 || len(e.Data) != 0 {
			t.Errorf("%s = %+v (listed: %v), want a symbolic link with no bytes", name, e, ok)
		}
	}
	if _, followed := got["dir-link/secret.md"]; followed {
		t.Errorf("entries = %v: a link to a directory was walked into", describe(entries))
	}
	if len(entries) != 4 {
		t.Errorf("entries = %v, want SKILL.md and the three links", describe(entries))
	}
}

func TestApprovalsSayAMissingFileWithTheErrorThatMeansAbsent(t *testing.T) {
	root := t.TempDir()
	if _, err := (skillsfs.Approvals{}).ReadSkillTree(root, "none"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadSkillTree of a skill that is not there: %v, want fs.ErrNotExist", err)
	}
	if _, err := (skillsfs.Approvals{}).ReadRecord(root, "none"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadRecord of a skill with no record: %v, want fs.ErrNotExist", err)
	}
}

func TestApprovalsReadTheRecordTheDomainNames(t *testing.T) {
	root := t.TempDir()
	put(t, skills.ApprovalRecordPath(root, "alpha"), `{"record": true}`)

	record, err := skillsfs.Approvals{}.ReadRecord(root, "alpha")
	if err != nil || string(record) != `{"record": true}` {
		t.Errorf("ReadRecord = %q, %v, want the bytes of alpha/.approval.json", record, err)
	}
}

// What is not a directory at the place of the skill is a failure of the system, and is not
// fs.ErrNotExist, so the domain refuses instead of taking it for a skill that is not there; and a
// directory that is a link is not followed into the files of another directory.
func TestApprovalsDoNotTakeWhatIsNotADirectoryForASkillOrForNone(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "a-file"), "not a directory")
	put(t, filepath.Join(root, "elsewhere", "SKILL.md"), "the skill of another directory")
	linked := true
	if err := os.Symlink("elsewhere", filepath.Join(root, "linked")); err != nil {
		linked = false
	}

	for _, path := range []string{"a-file", "linked"} {
		if path == "linked" && !linked {
			continue
		}
		entries, err := skillsfs.Approvals{}.ReadSkillTree(root, path)
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadSkillTree(%q) = %v, %v, want a failure that is not fs.ErrNotExist", path, describe(entries), err)
		}
	}
}

// A record that is a directory is not "no record": the failure is the system's, and is not
// fs.ErrNotExist, so the domain refuses instead of taking it for an absent approval.
func TestApprovalsDoNotTakeAnUnreadableRecordForAnAbsentOne(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "alpha", skills.ApprovalRecordName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (skillsfs.Approvals{}).ReadRecord(root, "alpha"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadRecord of a directory: %v, want a failure that is not fs.ErrNotExist", err)
	}
}

// A file of the skill that cannot be read is a skill that cannot be listed: it is never left out of
// the digest, which would let it change with the approval still holding.
func TestApprovalsDoNotLeaveOutAFileTheyCannotRead(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	script := filepath.Join(root, "alpha", "scripts", "run.sh")
	put(t, script, "echo hi\n")
	if err := os.Chmod(script, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(script, 0o644) })
	if _, err := os.ReadFile(script); err == nil {
		t.Skip("file permissions are not enforced for this process (root or an equivalent); an unreadable file cannot be simulated")
	}

	entries, err := skillsfs.Approvals{}.ReadSkillTree(root, "alpha")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadSkillTree = %v, %v, want a failure that is not fs.ErrNotExist", describe(entries), err)
	}
}
