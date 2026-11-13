package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
)

// The tests of `migrate-card-pages` (S6 (c)-(e) of the store-agnostic promotion), through the command. The world is
// the promote tests': memorycard memories 1-4 of project golden-project, where memory 2 was imported from Engram
// observation 1 and memory 4 from observation 3. The pages are written by hand in the shapes older releases left
// (legacyEngramPage, legacyCardPage), each with the precedence entry longterm-mem recorded for it.

var migrationInstant = time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)

const migrationBackupDir = ".vault-meta/card-page-migration-20310203T040506Z"

func sha(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// legacyEngramPage is a page as `promote --id N` of an Engram observation wrote it: engram_* keys and a footer.
func legacyEngramPage(address, title string, observation int, kind, revision string) string {
	id := string(rune('0' + observation))
	return "---\ntype: concept\ntitle: \"" + title + "\"\naddress: " + address + "\naliases:\n  - \"" + title + "\"\ncreated: 2026-08-01\nupdated: 2026-08-02\ntags:\n  - \"memory\"\n  - \"" + kind + "\"\nstatus: developing\nrelated: []\nsources: []\n" +
		"engram_id: " + id + "\nengram_sync_id: sync-" + id + "\nengram_type: " + kind + "\nengram_revision: " + revision + "\nproject: " + promoteGoldenProject + "\n---\n# " + title + "\n\nBody of " + title + ".\n\n---\nPromoted from Engram observation " + id + ", revision " + revision + ".\n"
}

// legacyCardPage is a page as `promote --source memorycard` wrote it before the store-agnostic keys.
func legacyCardPage(address, title string, memoryID int, kind, revision string) string {
	id := string(rune('0' + memoryID))
	return "---\ntype: concept\ntitle: \"" + title + "\"\naddress: " + address + "\naliases:\n  - \"" + title + "\"\ncreated: 2026-09-01\nupdated: 2026-09-02\ntags:\n  - \"memory\"\n  - \"" + kind + "\"\nstatus: developing\nrelated: []\nsources: []\n" +
		"memorycard_id: " + id + "\nmemorycard_type: " + kind + "\nmemorycard_revision: " + revision + "\nproject: " + promoteGoldenProject + "\n---\n# " + title + "\n\nBody of " + title + ".\n\n---\nPromoted from memorycard memory " + id + ", revision " + revision + ".\n"
}

// currentCardPage is a page of the store-agnostic keys, as `promote --source memorycard` writes one now.
func currentCardPage(address, title string, memoryID int, kind, revision string) string {
	id := string(rune('0' + memoryID))
	return "---\ntype: concept\ntitle: \"" + title + "\"\naddress: " + address + "\naliases:\n  - \"" + title + "\"\ncreated: 2026-10-01\nupdated: 2026-10-02\ntags:\n  - \"memory\"\n  - \"" + kind + "\"\nstatus: developing\nrelated: []\nsources: []\n" +
		"source: memorycard\nsource_id: " + id + "\nsource_type: " + kind + "\nsource_revision: " + revision + "\nproject: " + promoteGoldenProject + "\n---\n# " + title + "\n\nBody of " + title + ".\n\n---\nPromoted from memorycard memory " + id + ", revision " + revision + ".\n"
}

// recordPage writes the page at its address and, when tracked, the precedence entry longterm-mem recorded for it.
func recordPage(t *testing.T, w *promoteGoldenWorld, address, content string, tracked bool) {
	t.Helper()
	writeVaultFile(t, w, "wiki/memory/"+address+".md", content)
	if !tracked {
		return
	}
	const manifest = ".raw/.longterm-mem-manifest.json"
	entries := map[string]map[string]any{}
	if data, err := os.ReadFile(filepath.Join(w.vaultRoot, manifest)); err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			t.Fatal(err)
		}
	}
	end := strings.Index(content[4:], "---\n") + 4 + 4
	entries[address] = map[string]any{"body_hash": sha(content[end:]), "frontmatter_hash": sha(content[:end]), "promoted_revision": 1}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeVaultFile(t, w, manifest, string(data)+"\n")
}

// migrationWorld is the vault of the migration tests: three pages the migration moves (c-000901 and c-000902 name
// observations 1 and 3, whose memories are 2 and 4; c-000903 names memory 1 by memorycard_id), a page of the
// store-agnostic keys it leaves alone (c-000904, memory 3), and four it refuses: c-000905 (observation 5, whose
// memory 3 already has c-000904), c-000906 (observation 9, which no import_map row names), c-000907 (observation 6,
// memory 5, edited by hand) and c-000908 (observation 7, memory 6, with no precedence entry). The Engram database
// is a path that is not there: nothing the migration does may open it.
func migrationWorld(t *testing.T) (*promoteGoldenWorld, string) {
	t.Helper()
	w, db := memorycardWorld(t)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", filepath.Join(w.home, "no-engram", "engram.db"))
	execOn(t, db, `insert into memories(id, project, type, title, body) values (5, 'golden-project', 'decision', 'Edited Page', 'Body.'), (6, 'golden-project', 'decision', 'Untracked Page', 'Body.')`)
	execOn(t, db, `insert into import_map values ('engram', '5', 1, 3), ('engram', '6', 1, 5), ('engram', '7', 1, 6)`)
	recordPage(t, w, "c-000901", legacyEngramPage("c-000901", "Pinned Decision", 1, "decision", "1"), true)
	recordPage(t, w, "c-000902", legacyEngramPage("c-000902", "Below Threshold", 3, "discovery", "2"), true)
	recordPage(t, w, "c-000903", legacyCardPage("c-000903", "Card Only", 1, "learning", "3"), true)
	recordPage(t, w, "c-000904", currentCardPage("c-000904", "Second Card", 3, "learning", "1"), true)
	recordPage(t, w, "c-000905", legacyEngramPage("c-000905", "Duplicate Of Three", 5, "learning", "1"), true)
	recordPage(t, w, "c-000906", legacyEngramPage("c-000906", "Nobody Imported Me", 9, "learning", "1"), true)
	edited := legacyEngramPage("c-000907", "Edited Page", 6, "decision", "1")
	recordPage(t, w, "c-000907", edited, true)
	writeVaultFile(t, w, "wiki/memory/c-000907.md", edited+"\nA line a person added.\n")
	recordPage(t, w, "c-000908", legacyEngramPage("c-000908", "Untracked Page", 7, "decision", "1"), false)
	return w, db
}

func runMigrate(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var code int
	stdout, stderr := captureStreams(t, func() {
		code = commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: migrationInstant}}.run(append([]string{"migrate-card-pages", "--project", promoteGoldenProject}, args...))
	})
	return code, stdout, stderr
}

// changedPaths lists what differs between two snapshots of the vault: files added, removed or changed.
func changedPaths(before, after map[string]string) []string {
	var changed []string
	for path, content := range after {
		if was, ok := before[path]; !ok || was != content {
			changed = append(changed, filepath.ToSlash(path))
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, filepath.ToSlash(path))
		}
	}
	sort.Strings(changed)
	return changed
}

func TestMigrateCardPages_TheDefaultIsADryRunThatWritesNothingAndCountsEachRefusal(t *testing.T) {
	w, db := migrationWorld(t)
	dbBefore := fileHash(t, db)
	before := cmdVaultSnapshot(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t)
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	for _, want := range []string{
		"would migrate: c-000901 (Engram observation 1, memorycard memory 2)\n",
		"would migrate: c-000902 (Engram observation 3, memorycard memory 4)\n",
		"would migrate: c-000903 (memorycard memory 1)\n",
		"7 page(s) carry legacy keys in " + w.vaultRoot + ": would migrate 3, would refuse 4\n",
		"  refused, edited since longterm-mem wrote it: 1\n",
		"  refused, not recorded as written by longterm-mem: 1\n",
		"  refused, no import_map row names its Engram observation: 1\n",
		"  refused, its memory already has a page: 1\n",
		"nothing was written (pass --apply to migrate)\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	for _, want := range []string{"refused c-000905: memory 3 already has the page c-000904", "refused c-000906: memorycard holds no memory imported from Engram observation 9", "refused c-000907: the page was edited locally", "refused c-000908: the page was not recorded"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if after := cmdVaultSnapshot(t, w.vaultRoot); len(changedPaths(before, after)) != 0 {
		t.Errorf("the dry run changed the vault at %v", changedPaths(before, after))
	}
	if fileHash(t, db) != dbBefore {
		t.Errorf("the memorycard database changed")
	}
}

func TestMigrateCardPages_ApplyGivesEveryLegacyPageTheStoreAgnosticKeysAndNothingElse(t *testing.T) {
	w, db := migrationWorld(t)
	dbBefore := fileHash(t, db)
	before := cmdVaultSnapshot(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t, "--apply")
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, want %d (four pages are refused)\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	for _, want := range []string{"migrated: c-000901 (Engram observation 1, memorycard memory 2)", "migrated 3 of 7 page(s) carrying legacy keys", "refused 4", filepath.Join(w.vaultRoot, filepath.FromSlash(migrationBackupDir))} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	after := cmdVaultSnapshot(t, w.vaultRoot)
	var others []string
	for _, path := range changedPaths(before, after) {
		if !strings.HasPrefix(path, migrationBackupDir+"/") {
			others = append(others, path)
		}
	}
	if want := []string{".raw/.longterm-mem-manifest.json", "wiki/memory/c-000901.md", "wiki/memory/c-000902.md", "wiki/memory/c-000903.md"}; strings.Join(others, " ") != strings.Join(want, " ") {
		t.Errorf("the vault changed at %v, want exactly %v (every other page, the catalog and the log byte-identical)", others, want)
	}

	// An Engram page: the identity of the memory imported from the observation, at that memory's revision and type;
	// the address, created, body and every other key stay, and engram_sync_id goes with the other legacy keys.
	wantOne := strings.Replace(before["wiki/memory/c-000901.md"],
		"engram_id: 1\nengram_sync_id: sync-1\nengram_type: decision\nengram_revision: 1\n",
		"source: memorycard\nsource_id: 2\nsource_type: decision\nsource_revision: 1\n", 1)
	if got := after["wiki/memory/c-000901.md"]; got != wantOne {
		t.Errorf("c-000901 =\n%s\nwant\n%s", got, wantOne)
	}
	// Observation 3 is memory 4, a discovery at revision 1: the page said revision 2.
	if got := after["wiki/memory/c-000902.md"]; !strings.Contains(got, "source: memorycard\nsource_id: 4\nsource_type: discovery\nsource_revision: 1\n") || strings.Contains(got, "engram_") {
		t.Errorf("c-000902 =\n%s", got)
	}
	// A memorycard page keeps its memory, its revision and its type.
	wantCard := strings.Replace(before["wiki/memory/c-000903.md"],
		"memorycard_id: 1\nmemorycard_type: learning\nmemorycard_revision: 3\n",
		"source: memorycard\nsource_id: 1\nsource_type: learning\nsource_revision: 3\n", 1)
	if got := after["wiki/memory/c-000903.md"]; got != wantCard {
		t.Errorf("c-000903 =\n%s\nwant\n%s", got, wantCard)
	}

	// The sidecar: the migrated pages' frontmatter hashes follow their new frontmatter, nothing else of an entry
	// changes, and the other pages' entries are what they were.
	var was, now map[string]map[string]any
	if err := json.Unmarshal([]byte(before[".raw/.longterm-mem-manifest.json"]), &was); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(after[".raw/.longterm-mem-manifest.json"]), &now); err != nil {
		t.Fatal(err)
	}
	for address, entry := range was {
		page := after["wiki/memory/"+address+".md"]
		end := strings.Index(page[4:], "---\n") + 4 + 4
		moved := address == "c-000901" || address == "c-000902" || address == "c-000903"
		if !moved {
			if a, b := mustJSON(t, entry), mustJSON(t, now[address]); a != b {
				t.Errorf("the entry of %s changed: %s, was %s", address, b, a)
			}
			continue
		}
		if now[address]["frontmatter_hash"] != sha(page[:end]) || now[address]["body_hash"] != entry["body_hash"] || now[address]["promoted_revision"] != entry["promoted_revision"] {
			t.Errorf("the entry of %s = %v, want the frontmatter hash of its new block (%s) and the body hash and revision it had", address, now[address], sha(page[:end]))
		}
	}

	// The backup holds every file that was rewritten, as it was.
	for _, rel := range []string{".raw/.longterm-mem-manifest.json", "wiki/memory/c-000901.md", "wiki/memory/c-000902.md", "wiki/memory/c-000903.md"} {
		if after[migrationBackupDir+"/"+rel] != before[rel] {
			t.Errorf("the backup does not hold %s as it was", rel)
		}
	}
	if n := len(changedPaths(before, after)) - len(others); n != 4 {
		t.Errorf("the backup holds %d files, want 4 (three pages and the sidecar)", n)
	}
	if fileHash(t, db) != dbBefore {
		t.Errorf("the memorycard database changed")
	}
}

// regularFiles is cmdVaultSnapshot of the regular files only: a link to nothing in the vault is not a file to read.
func regularFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		files[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("list the vault: %v", err)
	}
	return files
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMigrateCardPages_ARefusedPageStaysByteIdenticalAndAnImportedOneIsNotMatchedByContent(t *testing.T) {
	w, _ := migrationWorld(t)
	before := cmdVaultSnapshot(t, w.vaultRoot)
	if code, so, se := runMigrate(t, "--apply"); code != exitRegistrationConflict {
		t.Fatalf("exit %d: %s %s", code, so, se)
	}
	after := cmdVaultSnapshot(t, w.vaultRoot)
	for _, address := range []string{"c-000904", "c-000905", "c-000906", "c-000907", "c-000908"} {
		rel := "wiki/memory/" + address + ".md"
		if after[rel] != before[rel] {
			t.Errorf("%s changed:\n%s\nwas:\n%s", rel, after[rel], before[rel])
		}
	}
	// c-000906 holds the very text of nothing imported and c-000905 the text of memory 3's page: neither is paired
	// by content, so both are still pages carrying legacy keys.
	if !strings.Contains(after["wiki/memory/c-000906.md"], "engram_id: 9\n") || !strings.Contains(after["wiki/memory/c-000905.md"], "engram_id: 5\n") {
		t.Errorf("a refused page lost its legacy keys")
	}
}

func TestMigrateCardPages_ASecondRunChangesNothingAndTheDryRunCountsWhatIsLeft(t *testing.T) {
	w, _ := migrationWorld(t)
	if code, so, se := runMigrate(t, "--apply"); code != exitRegistrationConflict {
		t.Fatalf("first run: exit %d: %s %s", code, so, se)
	}
	before := cmdVaultSnapshot(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t, "--apply")
	if code != exitRegistrationConflict {
		t.Fatalf("second run: exit %d, want %d (the four refused pages are still there)\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	if !strings.Contains(stdout, "migrated 0 of 4 page(s) carrying legacy keys") || strings.Contains(stdout, "backed up") {
		t.Errorf("stdout = %s, want nothing migrated, 4 pages left and no backup", stdout)
	}
	if after := cmdVaultSnapshot(t, w.vaultRoot); len(changedPaths(before, after)) != 0 {
		t.Errorf("the second run changed the vault at %v", changedPaths(before, after))
	}
}

// With only migratable pages, the dry run after the migration reports zero legacy pages and exit 0: the gate of the
// retirement of the legacy keys (S7).
func TestMigrateCardPages_TheDryRunAfterAFullMigrationReportsZeroLegacyPages(t *testing.T) {
	clean, _ := memorycardWorld(t)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", filepath.Join(clean.home, "no-engram", "engram.db"))
	recordPage(t, clean, "c-000901", legacyEngramPage("c-000901", "Pinned Decision", 1, "decision", "1"), true)
	if code, so, se := runMigrate(t, "--apply"); code != exitOK {
		t.Fatalf("clean world: exit %d: %s %s", code, so, se)
	}
	code, stdout, stderr := runMigrate(t)
	if code != exitOK || !strings.Contains(stdout, "0 page(s) carry legacy keys") {
		t.Errorf("clean world dry run: exit %d, stdout %q, stderr %q, want exit 0 and zero legacy pages", code, stdout, stderr)
	}
}

func TestMigrateCardPages_AFailedBackupWritesNothing(t *testing.T) {
	w, _ := migrationWorld(t)
	// The directory the backup would make is already there: the backup refuses to merge into it.
	writeVaultFile(t, w, migrationBackupDir+"/keep", "x")
	before := cmdVaultSnapshot(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t, "--apply")
	if code == exitOK || code == exitRegistrationConflict || !strings.Contains(stderr, "nothing was written") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want a failure that says nothing was written", code, stdout, stderr)
	}
	if after := cmdVaultSnapshot(t, w.vaultRoot); len(changedPaths(before, after)) != 0 {
		t.Errorf("a failed backup left the vault changed at %v", changedPaths(before, after))
	}
}

func TestMigrateCardPages_APageThatCannotBeReadStopsTheRunBeforeAnyWrite(t *testing.T) {
	w, _ := migrationWorld(t)
	if err := os.Symlink(filepath.Join(w.vaultRoot, "nowhere.md"), filepath.Join(w.vaultRoot, "wiki", "memory", "c-000555.md")); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	before := regularFiles(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t, "--apply")
	if code == exitOK || !strings.Contains(stderr, "c-000555.md") || !strings.Contains(stderr, "nothing was written") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want a failure that names the page and says nothing was written", code, stdout, stderr)
	}
	if after := regularFiles(t, w.vaultRoot); len(changedPaths(before, after)) != 0 {
		t.Errorf("the vault changed at %v", changedPaths(before, after))
	}
}

// The migration takes no argument besides its flags, as every command of the program.
func TestMigrateCardPages_TakesFlagsOnly(t *testing.T) {
	migrationWorld(t)
	if code, _, stderr := runMigrate(t, "c-000901"); code != exitUsage || !strings.Contains(stderr, "unexpected argument") {
		t.Errorf("exit %d, stderr %q, want usage", code, stderr)
	}
}

// S6 (d): the memories whose pages were migrated are not promoted a second time. The dry run of sync predicts the
// same promotions before and after the migration; what changes is only that the pages of memories 2 and 4 are their
// own now (counted as skipped) and no longer bridged ones.
func TestMigrateCardPages_SyncPredictsTheSamePromotionsAfterTheMigration(t *testing.T) {
	w, _ := migrationWorld(t)
	dry := []string{"sync", "--dry-run", "--project", promoteGoldenProject}
	_, planBefore, _ := runPromoteGolden(t, dry)
	if code, so, se := runMigrate(t, "--apply"); code != exitRegistrationConflict {
		t.Fatalf("migrate: exit %d: %s %s", code, so, se)
	}
	_, planAfter, _ := runPromoteGolden(t, dry)

	wouldPromote := func(plan string) []string {
		var lines []string
		for _, line := range strings.Split(plan, "\n") {
			if strings.HasPrefix(line, "  would promote:") {
				lines = append(lines, line)
			}
		}
		return lines
	}
	if strings.Join(wouldPromote(planBefore), "|") != strings.Join(wouldPromote(planAfter), "|") {
		t.Errorf("the dry run promotes other memories after the migration:\nbefore:\n%s\nafter:\n%s", planBefore, planAfter)
	}
	promotes := func(plan string) string {
		return strings.SplitN(strings.SplitN(plan, "would promote ", 2)[1], " memory(ies)", 2)[0]
	}
	if promotes(planBefore) != promotes(planAfter) {
		t.Errorf("the dry run says %s promotions before and %s after:\nbefore:\n%s\nafter:\n%s", promotes(planBefore), promotes(planAfter), planBefore, planAfter)
	}
	// And a real sync after the migration promotes none of the memories of the migrated pages.
	if code, so, se := runPromoteGolden(t, []string{"sync", "--project", promoteGoldenProject}); code != exitOK && code != exitRegistrationConflict {
		t.Fatalf("sync: exit %d: %s %s", code, so, se)
	}
	for _, name := range pageNames(t, w) {
		page := readVaultFile(t, w, "wiki/memory/"+name)
		for _, id := range []string{"2", "4"} {
			if strings.Count(page, "source_id: "+id+"\n") > 0 && name != "c-000901.md" && name != "c-000902.md" {
				t.Errorf("%s is a second page of memory %s", name, id)
			}
		}
	}
}

// S6 (e): the pages keep their addresses, a promote of a migrated page's memory answers that it is promoted, the
// index rebuild of a sync runs on the migrated vault, and doctor reports the same findings.
func TestMigrateCardPages_ExistingPagesKeepResolving(t *testing.T) {
	w, _ := migrationWorld(t)
	doctor := func() (int, []string) {
		code, stdout, stderr := runPromoteGolden(t, []string{"doctor", "--project", promoteGoldenProject, "--json"})
		var report ops.DoctorReport
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("decode the doctor's JSON: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
		// What a check finds is its name, its status and, for the eligibility of the pages, which page is judged against
		// which memory; the advice it adds to a finding names a command and is not a finding (S11 words it for the
		// pages that predate memorycard identity, and the migration is what makes it a plain refresh).
		judged := regexp.MustCompile(`c-\d+: memory \d+ is not pinned and has no topic_key`)
		var findings []string
		for _, check := range report.Checks {
			detail := check.Detail
			if check.Name == "promoted-pages-eligible" {
				detail = strings.Join(judged.FindAllString(detail, -1), "; ")
			}
			findings = append(findings, check.Name+" "+check.Status+" "+detail)
		}
		return code, findings
	}
	codeBefore, findingsBefore := doctor()
	namesBefore := pageNames(t, w)
	if code, so, se := runMigrate(t, "--apply"); code != exitRegistrationConflict {
		t.Fatalf("migrate: exit %d: %s %s", code, so, se)
	}

	if names := pageNames(t, w); strings.Join(names, " ") != strings.Join(namesBefore, " ") {
		t.Errorf("the pages are %v, were %v: an address changed", names, namesBefore)
	}
	// Memory 2 was bridged to the page c-000901 (a refusal, exit 6); now the page is its own.
	for id, address := range map[string]string{"2": "c-000901", "4": "c-000902", "1": "c-000903"} {
		code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", id})
		if code != exitOK || !strings.Contains(stdout, "already promoted as "+address) {
			t.Errorf("promote memory %s: exit %d, stdout %q, stderr %q, want exit 0 and `already promoted as %s`", id, code, stdout, stderr, address)
		}
	}
	if code, so, se := runPromoteGolden(t, []string{"sync", "--project", promoteGoldenProject}); code != exitOK && code != exitRegistrationConflict {
		t.Errorf("the sync of the migrated vault (its index rebuild): exit %d: %s %s", code, so, se)
	}
	codeAfter, findingsAfter := doctor()
	if codeBefore != codeAfter || strings.Join(findingsBefore, "\n") != strings.Join(findingsAfter, "\n") {
		t.Errorf("doctor's findings changed:\nbefore (exit %d):\n%s\nafter (exit %d):\n%s", codeBefore, strings.Join(findingsBefore, "\n"), codeAfter, strings.Join(findingsAfter, "\n"))
	}
}
