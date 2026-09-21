package skillstale_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"

	_ "modernc.org/sqlite"
)

const fixtureProject = "retirement-detector-fixture"

func TestParseProjectLock_ReadsSharedFixture(t *testing.T) {
	data := readFixture(t)
	lock, err := skillstale.ParseProjectLock(data)
	if err != nil {
		t.Fatalf("ParseProjectLock: %v", err)
	}
	if lock.Version != 1 {
		t.Fatalf("lock version = %d, want 1", lock.Version)
	}
	if len(lock.Skills) != 5 {
		t.Fatalf("len(lock.Skills) = %d, want 5", len(lock.Skills))
	}
	ids := make([]string, 0, len(lock.Skills))
	for _, entry := range lock.Skills {
		ids = append(ids, entry.ID)
		if len(entry.Targets) != 2 {
			t.Errorf("%s has %d targets, want 2", entry.ID, len(entry.Targets))
		}
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("fixture ids are not sorted: %v", ids)
	}
}

func TestDetect_ReportsStaleSignalsAndDoesNotMutate(t *testing.T) {
	fixture := makeFixture(t)
	beforeTree := snapshotTree(t, fixture.root)

	store, err := engram.Open(fixture.dbPath)
	if err != nil {
		t.Fatalf("engram.Open: %v", err)
	}
	beforeDB := snapshotTree(t, filepath.Dir(fixture.dbPath))

	got, err := skillstale.Detect(skillstale.Config{
		ProjectRoot: fixture.root,
		Project:     fixtureProject,
		Store:       store,
		Now:         fixture.now,
		PathEnv:     fixture.pathEnv,
	})
	if err != nil {
		store.Close()
		t.Fatalf("skillstale.Detect: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close Engram store: %v", err)
	}

	if gotTree := snapshotTree(t, fixture.root); !reflect.DeepEqual(gotTree, beforeTree) {
		t.Fatalf("detector mutated project tree:\nbefore=%v\nafter=%v", beforeTree, gotTree)
	}
	if gotDB := snapshotTree(t, filepath.Dir(fixture.dbPath)); !reflect.DeepEqual(gotDB, beforeDB) {
		t.Fatalf("detector mutated Engram files:\nbefore=%v\nafter=%v", beforeDB, gotDB)
	}

	byID := make(map[string]skillstale.Finding, len(got))
	for _, finding := range got {
		byID[finding.SkillID] = finding
	}
	if _, ok := byID["clean-skill"]; ok {
		t.Fatalf("clean skill was flagged: %+v", byID["clean-skill"])
	}

	removed := requireFinding(t, byID, "removed-path-skill")
	removedSignal := requireSignal(t, removed, skillstale.SignalRemoved)
	if removedSignal.Path != "removed.go" || removedSignal.Commit == "" {
		t.Errorf("removed signal = %+v, want removed.go with a commit", removedSignal)
	}

	moved := requireFinding(t, byID, "moved-path-skill")
	if signal := requireSignal(t, moved, skillstale.SignalMoved); signal.Path != "moved.go" || signal.NewPath != "moved-new.go" {
		t.Errorf("moved signal = %+v, want moved.go -> moved-new.go", signal)
	}
	for _, signal := range moved.Signals {
		if signal.Kind == skillstale.SignalRemoved {
			t.Errorf("renamed path must never also be reported as removed: %+v", moved.Signals)
		}
	}

	unresolved := requireFinding(t, byID, "unresolved-command-skill")
	if signal := requireSignal(t, unresolved, skillstale.SignalUnresolvedCommand); signal.Command != "definitely-missing-command" {
		t.Errorf("unresolved command signal = %+v, want definitely-missing-command", signal)
	}

	quiet := requireFinding(t, byID, "quiet-skill")
	if signal := requireSignal(t, quiet, skillstale.SignalQuiet); signal.Since != "2000-01-01T00:00:00Z" {
		t.Errorf("quiet signal = %+v, want original LastObserved", signal)
	}
}

func TestDetect_RemovedPathIsReportedRegardlessOfCandidateOrdering(t *testing.T) {
	fixture := makeFixture(t)
	rewriteCandidateLastObserved(t, fixture.dbPath, "procedural/candidates/repeated-success/removed-path-skill", "2099-01-01T00:00:00Z")

	store, err := engram.Open(fixture.dbPath)
	if err != nil {
		t.Fatalf("engram.Open: %v", err)
	}
	findings, err := skillstale.Detect(skillstale.Config{
		ProjectRoot: fixture.root,
		Project:     fixtureProject,
		Store:       store,
		Now:         fixture.now,
		PathEnv:     fixture.pathEnv,
	})
	_ = store.Close()
	if err != nil {
		t.Fatalf("skillstale.Detect: %v", err)
	}
	removed := findingByID(t, findings, "removed-path-skill")
	if signal := requireSignal(t, removed, skillstale.SignalRemoved); signal.Path != "removed.go" {
		t.Errorf("removed signal = %+v, want removed.go even when candidate LastObserved is newer", signal)
	}
}

type fixtureRepo struct {
	root    string
	dbPath  string
	pathEnv string
	now     time.Time
}

func makeFixture(t *testing.T) fixtureRepo {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q", "-b", "main")

	writeFile(t, root, "removed.go", "package fixture\n")
	writeFile(t, root, "moved.go", "package fixture\n// distinctive moved body\n")
	writeFile(t, root, "present.go", "package fixture\n")
	writeLock(t, root, readFixture(t))
	writeSkills(t, root)
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "initial fixture")

	runGit(t, root, "rm", "-q", "removed.go")
	runGit(t, root, "commit", "-q", "-m", "remove referenced path")
	runGit(t, root, "mv", "moved.go", "moved-new.go")
	runGit(t, root, "commit", "-q", "-m", "rename referenced path")

	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir command fixture: %v", err)
	}
	writeFileMode(t, bin, "present-command", "#!/bin/sh\nexit 0\n", 0o755)

	dbPath := fixtureDB(t)
	insertCandidate(t, dbPath, "removed-path-skill", "**Status**: registered\n**LastObserved**: 2025-12-31T00:00:00Z\n")
	insertCandidate(t, dbPath, "moved-path-skill", "**Status**: registered\n**LastObserved**: 2025-12-31T00:00:00Z\n")
	insertCandidate(t, dbPath, "clean-skill", "**Status**: registered\n**LastObserved**: 2025-12-31T00:00:00Z\n")
	insertCandidate(t, dbPath, "unresolved-command-skill", "**Status**: registered\n**LastObserved**: 2025-12-31T00:00:00Z\n")
	insertCandidate(t, dbPath, "quiet-skill", "**Status**: registered\n**LastObserved**: 2000-01-01T00:00:00Z\n")

	return fixtureRepo{
		root:    root,
		dbPath:  dbPath,
		pathEnv: bin,
		now:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func writeSkills(t *testing.T, root string) {
	t.Helper()
	const tick = "`"
	const fence = "```"
	contents := map[string]string{
		"clean-skill":              "---\nname: clean-skill\ndescription: \"Trigger: clean fixture.\"\n---\nUse " + tick + "present.go" + tick + " when needed.\n" + fence + "sh\npresent-command present.go\n" + fence + "\n",
		"moved-path-skill":         "---\nname: moved-path-skill\ndescription: \"Trigger: moved fixture.\"\n---\nThe old path is " + tick + "moved.go" + tick + ", but its content was renamed.\n",
		"quiet-skill":              "---\nname: quiet-skill\ndescription: \"Trigger: quiet fixture.\"\n---\nUse " + tick + "present.go" + tick + ".\n",
		"removed-path-skill":       "---\nname: removed-path-skill\ndescription: \"Trigger: removed fixture.\"\n---\nInspect " + tick + "removed.go" + tick + " before changing this procedure.\n",
		"unresolved-command-skill": "---\nname: unresolved-command-skill\ndescription: \"Trigger: command fixture.\"\n---\n" + fence + "sh\ndefinitely-missing-command present.go\n" + fence + "\n",
	}
	for id, content := range contents {
		writeFile(t, root, filepath.Join(".claude", "skills", id, "SKILL.md"), content)
		writeFile(t, root, filepath.Join(".agents", "skills", id, "SKILL.md"), content)
	}
}

func writeLock(t *testing.T, root string, data []byte) {
	t.Helper()
	writeFile(t, root, ".labdrian/procedural-skills.lock.json", string(data))
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/procedural-skills.lock.json")
	if err != nil {
		t.Fatalf("read lock fixture: %v", err)
	}
	return data
}

func fixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	schema, err := os.ReadFile(filepath.Join("..", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read Engram schema: %v", err)
	}
	path := filepath.Join(dir, "engram.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		t.Fatalf("apply Engram schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	return path
}

func insertCandidate(t *testing.T, dbPath, id, fields string) {
	t.Helper()
	lock, err := skillstale.ParseProjectLock(readFixture(t))
	if err != nil {
		t.Fatalf("parse fixture in insertCandidate: %v", err)
	}
	var candidate string
	for _, entry := range lock.Skills {
		if entry.ID == id {
			candidate = entry.Candidate
			break
		}
	}
	if candidate == "" {
		t.Fatalf("fixture has no candidate for %q", id)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open candidate database: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO observations (session_id, type, title, content, project, topic_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "sess-1", "pattern", id, fields, fixtureProject, candidate, "2025-12-31 00:00:00", "2025-12-31 00:00:00")
	if err != nil {
		t.Fatalf("insert candidate %q: %v", id, err)
	}
}

func rewriteCandidateLastObserved(t *testing.T, dbPath, candidate, lastObserved string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open candidate database: %v", err)
	}
	defer db.Close()
	content := "**Status**: registered\n**LastObserved**: " + lastObserved + "\n"
	if _, err := db.Exec(`UPDATE observations SET content = ? WHERE project = ? AND topic_key = ?`, content, fixtureProject, candidate); err != nil {
		t.Fatalf("rewrite candidate %q: %v", candidate, err)
	}
}

func requireFinding(t *testing.T, byID map[string]skillstale.Finding, id string) skillstale.Finding {
	t.Helper()
	finding, ok := byID[id]
	if !ok {
		t.Fatalf("no finding for %q; got ids %v", id, mapKeys(byID))
	}
	return finding
}

func findingByID(t *testing.T, findings []skillstale.Finding, id string) skillstale.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.SkillID == id {
			return finding
		}
	}
	t.Fatalf("no finding for %q; got %+v", id, findings)
	return skillstale.Finding{}
}

func requireSignal(t *testing.T, finding skillstale.Finding, kind skillstale.SignalKind) skillstale.Signal {
	t.Helper()
	for _, signal := range finding.Signals {
		if signal.Kind == kind {
			return signal
		}
	}
	t.Fatalf("finding %q has no %s signal: %+v", finding.SkillID, kind, finding.Signals)
	return skillstale.Signal{}
}

func mapKeys(values map[string]skillstale.Finding) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func snapshotTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	got := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return got
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	writeFileMode(t, root, rel, content, 0o644)
}

func writeFileMode(t *testing.T, root, rel, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// Keep the fixture's JSON shape checked by the package test itself, rather
// than making the byte-for-byte engine pin the only parser coverage.
func TestFixtureIsStrictJSON(t *testing.T) {
	var value map[string]any
	if err := json.Unmarshal(readFixture(t), &value); err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
}
