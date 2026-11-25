package skills

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The evidence store (S4, T2) is absorbed from the playbook snapshot into skills/_shared/evidence/
// as overlay-owned code. These pins hold its files, its manifest rows, its deployed path and the
// provenance that travels with it.

const evidenceDirRel = "skills/_shared/evidence"

var evidenceStoreNames = []string{"evidence.sh", "evidence-rules.md", "LICENSE", "PROVENANCE.md"}

func TestEvidenceStore_ManifestHoldsFourClaudeSkillRows(t *testing.T) {
	rows := ReadManifestRows([]byte(readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")))
	for _, name := range evidenceStoreNames {
		path := "_shared/evidence/" + name
		var found []ManifestRow
		for _, row := range rows {
			if row.Path == path {
				found = append(found, row)
			}
		}
		if len(found) != 1 || found[0].Tag != "custom" || found[0].Route != "claude-skill" {
			t.Errorf("overlay.manifest rows for %s = %+v, want exactly one, tagged custom and routed claude-skill", path, found)
		}
	}
}

func TestEvidenceStore_DeploysToClaudeAndToNoOtherTarget(t *testing.T) {
	rows := ReadManifestRows([]byte(readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")))
	for _, tc := range []struct {
		target string
		dest   RouteDestinations
		root   string
	}{
		{"claude", RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: "/a"}, "/s"},
		{"codex", RouteDestinations{Target: "codex", SkillsRoot: "/c", AgentsRoot: "/ca"}, ""},
		{"opencode", RouteDestinations{Target: "opencode", SkillsRoot: "/o2", AgentsRoot: "/oa"}, ""},
	} {
		routes, err := ResolveRoutes(rows, false, testOverlay, tc.dest)
		if err != nil {
			t.Fatalf("ResolveRoutes for %s: %v", tc.target, err)
		}
		dest := map[string]string{}
		for _, r := range routes {
			dest[r.Path] = r.Dest
		}
		for _, name := range evidenceStoreNames {
			path := "_shared/evidence/" + name
			want := ""
			if tc.root != "" {
				want = tc.root + "/" + path
			}
			if got, ok := dest[path]; !ok || got != want {
				t.Errorf("%s for %s deploys to %q (row present %v), want %q", path, tc.target, got, ok, want)
			}
		}
	}
}

func TestEvidenceStore_LicenseAndProvenanceTravelWithTheCode(t *testing.T) {
	root := skillsRepoRoot(t)
	license := readRepoFile(t, root, evidenceDirRel+"/LICENSE")
	for _, want := range []string{"MIT License", "Copyright (c) 2026 Andres Frei"} {
		if !strings.Contains(license, want) {
			t.Errorf("LICENSE lacks %q", want)
		}
	}
	provenance := readRepoFile(t, root, evidenceDirRel+"/PROVENANCE.md")
	for _, want := range []string{
		"6de8566bf7f1150f49a75c1162c0f4cf87bf57c5",
		"v3.6.0",
		"andresfrei/playbook",
		"plugins/playbook-agents/evidence.sh",
		"plugins/playbook-agents/evidence-rules.md",
		"MIT",
		"`reviewed-each <wiki directory> <identifier>...`", // a mode the script gained (T4k)
		"as the mode they name",                            // the exit-4 lines of reviewed-each name it
		"never as a number",                                // reviewed and rule compare as text
	} {
		if !strings.Contains(provenance, want) {
			t.Errorf("PROVENANCE.md lacks %q", want)
		}
	}
}

func TestEvidenceStore_ScriptParsesAsPosixShell(t *testing.T) {
	overlaytest.RequireShAndAwk(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("sh: %v", err)
	}
	script := filepath.Join(skillsRepoRoot(t), evidenceDirRel, "evidence.sh")
	if out, err := exec.Command(sh, "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("sh -n %s: %v\n%s", script, err, out)
	}
}

// The git blob ids of the three files copied from the snapshot 6de8566 (v3.6.0), from
// `git -C ~/.local/share/playbook rev-parse 6de8566:<path>`. The snapshot never changes, so neither do these.
var evidenceSnapshotBlobs = map[string]string{
	"evidence.sh":       "09b44a09878454b0f96f81ca00431236b142fd59",
	"evidence-rules.md": "48c4bf91cda2322dc71f4fb080c9add7f6233031",
	"LICENSE":           "b6f9c476b8c3982bfd1b4368d61c83ea53e22dad",
}

// gitBlobID is the id git gives a file's bytes: SHA-1 of "blob <size>\0<bytes>". skills/** is -text, so the
// bytes on disk are the bytes of the blob.
func gitBlobID(content string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content)))
	return hex.EncodeToString(sum[:])
}

// The two texts the License line of PROVENANCE.md may have, one for each state of LICENSE.
const (
	licenseLineIdentical = "- License: MIT, in `LICENSE` beside this file (the snapshot `LICENSE`, byte-identical)."
	licenseLineChanged   = "- License: MIT, in `LICENSE` beside this file (changed since the snapshot `LICENSE`; see \"Changed since the snapshot\")."
)

// changedSinceTheSnapshot is the text under "## Changed since the snapshot" of PROVENANCE.md, up to the next
// heading or the end of the file.
func changedSinceTheSnapshot(t *testing.T, provenance string) string {
	t.Helper()
	_, after, ok := strings.Cut(provenance, "## Changed since the snapshot\n")
	if !ok {
		t.Fatal(`PROVENANCE.md has no "## Changed since the snapshot" section`)
	}
	section, _, _ := strings.Cut(after, "\n## ")
	return strings.TrimSpace(section)
}

func TestEvidenceStore_CopiesMatchTheSnapshotOrAreListedAsChanged(t *testing.T) {
	root := skillsRepoRoot(t)
	provenance := readRepoFile(t, root, evidenceDirRel+"/PROVENANCE.md")
	section := changedSinceTheSnapshot(t, provenance)
	changed := 0
	for name, blob := range evidenceSnapshotBlobs {
		equal := gitBlobID(readRepoFile(t, root, evidenceDirRel+"/"+name)) == blob
		listed := strings.Contains(section, "`"+name+"`")
		switch {
		case equal && listed:
			t.Errorf("%s equals the snapshot but the Changed section names it", name)
		case !equal && !listed:
			t.Errorf("%s differs from the snapshot and the Changed section does not name it", name)
		}
		if !equal {
			changed++
		}
	}
	if changed == 0 && section != "- Nothing." {
		t.Errorf("all three copies equal the snapshot, so the Changed section is exactly %q, got %q", "- Nothing.", section)
	}
	if changed > 0 {
		for _, line := range strings.Split(section, "\n") {
			if strings.TrimSpace(line) == "- Nothing." {
				t.Errorf("%d copies differ from the snapshot, so the Changed section must not say %q", changed, "- Nothing.")
			}
		}
	}

	// The License line is tied to the blob of LICENSE in both states: while it equals the snapshot's the line
	// is exactly licenseLineIdentical, and while it differs the line is exactly licenseLineChanged. No word
	// of the line is a key, so a reworded claim fails either way.
	licenseLine, found := "", false
	for _, line := range strings.Split(provenance, "\n") {
		if strings.HasPrefix(line, "- License:") {
			licenseLine, found = line, true
			break
		}
	}
	if !found {
		t.Errorf("PROVENANCE.md has no %q line", "- License:")
		return
	}
	got, want := gitBlobID(readRepoFile(t, root, evidenceDirRel+"/LICENSE")), evidenceSnapshotBlobs["LICENSE"]
	switch {
	case got == want && licenseLine != licenseLineIdentical:
		t.Errorf("LICENSE equals the snapshot, so the License line is exactly %q, got %q", licenseLineIdentical, licenseLine)
	case got != want && licenseLine != licenseLineChanged:
		t.Errorf("LICENSE differs from the snapshot (blob %s, not %s), so the License line is exactly %q, got %q", got, want, licenseLineChanged, licenseLine)
	}
}

// The files of the evidence store are deployed beside the skills and read in another repository, which has no task
// document: none of them names a section, task or log entry of one (S, T or L and a number, or OD- and a number;
// review 00564ea07ad9-5 F4, T4g, L62 choice 4).
func TestEvidenceStore_NamesNoSectionTaskOrLogEntryOfATaskDocument(t *testing.T) {
	root := skillsRepoRoot(t)
	for _, name := range evidenceStoreNames {
		if found := taskDocumentReferences(readRepoFile(t, root, evidenceDirRel+"/"+name)); len(found) > 0 {
			t.Errorf("%s names %q, a reference into a task document that the deployed file cannot carry", name, found[0])
		}
	}
}

// taskDocumentIdentifier is the pattern of the lint above and of its table test, as the task document writes its
// identifiers: S, T or L and a number, or OD- and a number, each with lowercase letters and any dashed parts of
// lowercase letters and digits (T4g, T3c-a2, OD-10b, and the T4k-only that is no identifier but names one). It cannot
// tell prose such as L2-cache from an identifier, so taskDocumentProse lists the prose by name.
var taskDocumentIdentifier = regexp.MustCompile(`\b[STL][0-9]+[a-z]*(?:-[a-z0-9]+)*\b|\bOD-[0-9]+[a-z]*(?:-[a-z0-9]+)*\b`)

// taskDocumentProse is the prose that the pattern matches and that names no identifier, each by its whole text: a false
// positive is fixed by adding its exact text here, which a review sees (reviews 66a1a0398abe-3 F2, 4f714c67e87c-2 F2
// and 4f714c67e87c-5 F1, T4k, T4q).
var taskDocumentProse = map[string]bool{"L2-cache": true, "S5-based": true}

// taskDocumentReferences is every identifier of a task document that text names: a match of the pattern that is not,
// as a whole, a prose term of the list.
func taskDocumentReferences(text string) []string {
	var found []string
	for _, match := range taskDocumentIdentifier.FindAllString(text, -1) {
		if !taskDocumentProse[match] {
			found = append(found, match)
		}
	}
	return found
}

// Every prose term is one whole match of the pattern, so no entry is dead (T4q, L78 choice 1).
func TestTheTaskDocumentProseTermsAreEachOneWholeMatch(t *testing.T) {
	for term := range taskDocumentProse {
		if got := taskDocumentIdentifier.FindString(term); got != term {
			t.Errorf("the pattern finds %q in the prose term %q, so the term is never dropped as a whole", got, term)
		}
	}
}

func TestTheTaskDocumentPatternMatchesEveryIdentifierForm(t *testing.T) {
	for _, id := range []string{"S6", "T4g", "T3c-f", "T3c-a2", "L64", "OD-11", "OD-10b",
		"T4k-only", "L72-style", "T3c-ab", "L72-x", "OD-11-era", "L2-cache-line"} {
		if got := taskDocumentReferences("see " + id + " here"); !reflect.DeepEqual(got, []string{id}) {
			t.Errorf("the pattern finds %q in %q, want %q", got, "see "+id+" here", id)
		}
	}
	for _, text := range []string{"ES2015", "SHA-256", "UTF-8", "level 2", "L2-cache", "S5-based", "see L2-cache here", "see S5-based here"} {
		if got := taskDocumentReferences(text); got != nil {
			t.Errorf("the pattern finds %q in %q, want no match", got, text)
		}
	}
	if got := taskDocumentReferences("L67-L72"); !reflect.DeepEqual(got, []string{"L67", "L72"}) {
		t.Errorf("the pattern finds %q in %q, want L67 and L72", got, "L67-L72")
	}
}

// shellCall is a use of one of the words that shellCalls looks for: the line it starts on, the word, and the rest of
// its logical line (continued lines joined) after the word.
type shellCall struct {
	line       int
	word, rest string
}

var shellCallWords = []string{"cd", "dirname", "basename"}

// shellCalls finds each cd, dirname and basename that is written as shell code: at the start of a word, outside
// comments, outside single quotes and outside double quotes except inside a "$(...)" there. A backslash and a line
// break join two lines; the call keeps the number of its first line. A quote state runs across lines. A here-document
// is refused, not read: "<<" written as shell code (not in quotes, a comment or "$((...))") is reported as a call of
// the word "<<", since its body would be read as code. Bound: a call built from a variable, eval, command or
// backquotes is not seen, and neither is a word that is an argument and not a command (echo cd is seen).
func shellCalls(script string) []shellCall {
	type context struct {
		quoted bool // double quotes; else code
		arith  bool // opened by "$((": a "<<" there is a shift
		depth  int  // open "(" of this code context, not counting its "$("
	}
	isName := func(b byte) bool {
		return b == '_' || b == '-' || b == '.' || b == '/' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	var calls []shellCall
	line, wordStart := 1, true
	stack := []context{{}}
	for i := 0; i < len(script); i++ {
		c, top := script[i], &stack[len(stack)-1]
		switch {
		case c == '\\' && i+1 < len(script):
			i++
			if script[i] == '\n' {
				line++
			}
			wordStart = false
		case c == '\n':
			line++
			wordStart = !top.quoted
		case top.quoted && c == '"':
			stack, wordStart = stack[:len(stack)-1], false
		case c == '$' && i+1 < len(script) && script[i+1] == '(':
			stack, wordStart = append(stack, context{arith: i+2 < len(script) && script[i+2] == '('}), true
			i++
		case top.quoted:
		case c == '<' && i+1 < len(script) && script[i+1] == '<' && !top.arith:
			calls = append(calls, shellCall{line: line, word: "<<", rest: logicalRest(script[i+2:])})
			i++
			wordStart = false
		case c == '\'':
			for i++; i < len(script) && script[i] != '\''; i++ {
				if script[i] == '\n' {
					line++
				}
			}
			wordStart = false
		case c == '"':
			stack, wordStart = append(stack, context{quoted: true}), false
		case c == '#' && wordStart:
			for i+1 < len(script) && script[i+1] != '\n' {
				i++
			}
		case c == '(':
			top.depth++
			wordStart = true
		case c == ')':
			if top.depth > 0 {
				top.depth--
			} else if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			wordStart = true
		default:
			if wordStart {
				for _, word := range shellCallWords {
					if strings.HasPrefix(script[i:], word) && (i+len(word) == len(script) || !isName(script[i+len(word)])) {
						calls = append(calls, shellCall{line: line, word: word, rest: logicalRest(script[i+len(word):])})
						i += len(word) - 1
						break
					}
				}
			}
			wordStart = c == ' ' || c == '\t' || c == ';' || c == '&' || c == '|' || c == '`'
		}
	}
	return calls
}

// logicalRest is the text up to the next line break that no backslash continues, continued lines joined.
func logicalRest(text string) string {
	var rest strings.Builder
	for i := 0; i < len(text) && text[i] != '\n'; i++ {
		if text[i] == '\\' && i+1 < len(text) && text[i+1] == '\n' {
			i++
			continue
		}
		rest.WriteByte(text[i])
	}
	return rest.String()
}

func TestShellCallsReadsOnlyShellCode(t *testing.T) {
	for _, c := range []struct {
		name, script string
		want         []shellCall
	}{
		{"dirname in a substitution", `x=$(dirname "$p")`, []shellCall{{1, "dirname", ` "$p")`}}},
		{"a word in double quotes", `echo "dirname failed"`, nil},
		{"a word in a comment", "f # dirname", nil},
		{"a word in single quotes", `'dirname'`, nil},
		{"cd in a substitution", `x=$(cd "$d" && pwd)`, []shellCall{{1, "cd", ` "$d" && pwd)`}}},
		{"basename in a substitution in double quotes", `echo "$(basename "$f")"`, []shellCall{{1, "basename", ` "$f")"`}}},
		{"a continued line keeps its first line", "a\nx=$(dirname \\\n\"$p\")", []shellCall{{2, "dirname", ` "$p")`}}},
		{"a continued line with the dashes on the next", "dirname \\\n-- \"$p\"", []shellCall{{1, "dirname", ` -- "$p"`}}},
		{"a hash inside quotes ends nothing", `echo "a # b"; dirname -- x`, []shellCall{{1, "dirname", " -- x"}}},
		{"a hash of a parameter ends nothing", "echo ${#x}; cd -- y", []shellCall{{1, "cd", " -- y"}}},
		{"quotes of both kinds, then a call", `echo 'it'"s"; cd x`, []shellCall{{1, "cd", " x"}}},
		{"an escaped quote opens nothing", `echo \"; cd x`, []shellCall{{1, "cd", " x"}}},
		{"a quote that spans lines", "echo 'a\ncd x'\ncd y", []shellCall{{3, "cd", " y"}}},
		{"a word inside a longer name", "mycd x; dirname2 y; x.cd z; $cd", nil},
		{"a here-document is reported, its body is not read", "cat <<EOF\nit's here\nEOF\ncd /x\n", []shellCall{{1, "<<", "EOF"}}},
		{"a here-document with a dash and quotes", "cat <<-'EOF'", []shellCall{{1, "<<", "-'EOF'"}}},
		{"a shift in arithmetic is no here-document", "x=$((1 << 2)); cd -- y", []shellCall{{1, "cd", " -- y"}}},
		{"two angle brackets in double quotes", `echo "<<"`, nil},
		{"two angle brackets in a comment", "f # <<", nil},
	} {
		if got := shellCalls(c.script); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: shellCalls(%q) = %v, want %v", c.name, c.script, got, c.want)
		}
	}
}

// Every cd, dirname and basename of the script ends its options, so a path that starts with "-" is read as a path
// (reviews 9b030f225615-2 F1 and 66a1a0398abe-5 F1, T4i, T4k). Only shell code counts: a comment, a quoted text or a
// message that names one of them is not a call (66a1a0398abe-3 F3).
func TestEvidenceStore_EveryCdDirnameAndBasenameEndsItsOptions(t *testing.T) {
	seen := map[string]bool{}
	for _, call := range shellCalls(readRepoFile(t, skillsRepoRoot(t), evidenceDirRel+"/evidence.sh")) {
		if call.word == "<<" {
			continue // a here-document is refused by its own test
		}
		seen[call.word] = true
		if !strings.HasPrefix(call.rest, " -- ") {
			t.Errorf("evidence.sh line %d: %q is not followed by \" -- \": %s%s", call.line, call.word, call.word, call.rest)
		}
	}
	for _, word := range shellCallWords {
		if !seen[word] {
			t.Errorf("shellCalls found no %s in evidence.sh, so this lint reads nothing of it", word)
		}
	}
}

// evidence.sh holds no here-document: the option lint above does not read a here-document's body, so one would hide
// the calls written after it (review 4f714c67e87c-5 F4, T4q).
func TestEvidenceStore_HoldsNoHereDocument(t *testing.T) {
	for _, call := range shellCalls(readRepoFile(t, skillsRepoRoot(t), evidenceDirRel+"/evidence.sh")) {
		if call.word == "<<" {
			t.Errorf("evidence.sh line %d writes a here-document: the option lint does not read its body, so write the text another way", call.line)
		}
	}
}

// unforcedEnvironReads returns, for every line of a script that holds the word ENVIRON, that line with its leading
// blanks dropped, unless it is exactly the one form <name> = ENVIRON["<NAME>"] "" (a plain variable assigned a read
// forced to text). A line whose first non-blank character is # is skipped: in sh and in awk it is a comment or the
// inside of a string, and neither is a read. One accepted form, not a list of refused ones, so a spelling no review
// has found yet is refused too (reviews 5e745160b611-2 F2 and 5e745160b611-5 F3, T4t).
func unforcedEnvironReads(script string) []string {
	word := regexp.MustCompile(`\bENVIRON\b`)
	form := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]* = ENVIRON\["[A-Z_][A-Z0-9_]*"\] ""$`)
	var found []string
	for _, line := range strings.Split(script, "\n") {
		code := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(code, "#") || !word.MatchString(code) || form.MatchString(code) {
			continue
		}
		found = append(found, code)
	}
	return found
}

// Every environment value that evidence.sh reads is read as a string, so the awk compares it with a cell as text and
// never as a number (reviews 4f714c67e87c-5 F5, T4q; 88fb598e73d3-5 F2, T4r; 5e745160b611-2 F2 and -5 F3, T4t).
func TestEvidenceStore_ReadsEveryEnvironmentValueAsAString(t *testing.T) {
	script := readRepoFile(t, skillsRepoRoot(t), evidenceDirRel+"/evidence.sh")
	if !regexp.MustCompile(`\bENVIRON\b`).MatchString(script) {
		t.Fatal("evidence.sh reads no environment value, so this lint reads nothing of it")
	}
	for _, read := range unforcedEnvironReads(script) {
		t.Errorf("evidence.sh uses %q with no empty string after its subscript, so awk takes the value for a number where it looks like one", read)
	}
}

func TestUnforcedEnvironReadsFindsEveryRead(t *testing.T) {
	for _, row := range []struct {
		name, line string
		want       int
	}{
		{"a read forced with one space", `x = ENVIRON["X"] ""`, 0},
		{"the form after blanks", `    want = ENVIRON["EVIDENCE_WANT"] ""`, 0},
		{"the form after a tab", "\twant = ENVIRON[\"EVIDENCE_WANT\"] \"\"", 0},
		{"a comment that names the word", `# ENVIRON is read once`, 0},
		{"an indented comment holding a read", `  # if (ENVIRON[x] > 0)`, 0},
		{"another word that ends in the name", `if (MYENVIRON["X"] > 0) y = 1`, 0},
		{"a negated read", `x = -ENVIRON["X"] ""`, 1},
		{"a read under not", `x = !ENVIRON["X"] ""`, 1},
		{"a read under an increment", `x = ++ENVIRON["X"] ""`, 1},
		{"a read forced with no space", `x = ENVIRON["X"]""`, 1},
		{"a subscript with blanks inside", `x = ENVIRON[ "X" ] ""`, 1},
		{"a variable subscript", `x = ENVIRON[name] ""`, 1},
		{"a variable subscript used as a number", `if (ENVIRON[name] > 0) y = 1`, 1},
		{"a read added to a number", `x = ENVIRON["X"] + 0`, 1},
		{"a forced read added to a number", `x = ENVIRON["X"] "" + 0`, 1},
		{"a forced read and a second statement", `x = ENVIRON["X"] ""; y = 1`, 1},
		{"a forced read and a trailing comment", `x = ENVIRON["X"] "" # note`, 1},
		{"a loop over the array", `for (k in ENVIRON) n++`, 1},
		{"a bracketed subscript", `x = ENVIRON[a[1]] ""`, 1},
		{"a chained assignment", `x = y = ENVIRON["X"] ""`, 1},
		{"the word in a printed string", `print "ENVIRON"`, 1},
		{"a subscript broken over two lines", "x = ENVIRON[\n\"X\"] \"\"", 1},
		{"two reads on one line", `x = ENVIRON["A"] "" ENVIRON["B"]`, 1},
	} {
		if got := unforcedEnvironReads(row.line); len(got) != row.want {
			t.Errorf("%s: %q gave %d entries %q, want %d", row.name, row.line, len(got), got, row.want)
		}
	}
}

// prose is a comment block or a document read as sentences: each line with its leading blanks dropped, then a leading run
// of # and the blanks after it dropped, the lines joined by one space and each run of blanks made one space. A phrase is
// then found wherever its lines break (81a7200c14c2-7 F2, T4x).
func prose(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(strings.TrimLeft(line, " \t"), "# \t")
	}
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

func TestProseJoinsTheLinesOfACommentBlock(t *testing.T) {
	for _, row := range []struct{ name, text, want string }{
		{"two comment lines", "#   a\n#   b", "a b"},
		{"a blank line between them", "#   a\n\n#   b", "a b"},
		{"blanks and a tab", "  x\n\ty  z", "x y z"},
		{"a markdown heading mark", "## Title\ntext", "Title text"},
		{"the wrapped claim of the verifier", "#   the caller's exec fails. ARG_MAX follows the\n#   stack limit. A missing", "the caller's exec fails. ARG_MAX follows the stack limit. A missing"},
	} {
		if got := prose(row.text); got != row.want {
			t.Errorf("%s: prose(%q) = %q, want %q", row.name, row.text, got, row.want)
		}
	}
	if !strings.Contains(prose("#   ARG_MAX follows the\n#   stack limit."), "follows the stack limit") {
		t.Error("a phrase wrapped over two comment lines is not found")
	}
}

// The bound of one environment string is stated in pages, since Linux's MAX_ARG_STRLEN is 32 pages and 128 KiB only
// where a page is 4 KiB (reviews 88fb598e73d3-5 F1 and 88fb598e73d3-4.polisher, T4r). Both limits are named, and what
// the caller sees past the second: ARG_MAX stops the exec of sh itself, so the script does not run (review
// 5e745160b611-4.polisher F2, T4t). Every needle, wanted or refused, is searched in the text read as prose, so a phrase
// is found or refused wherever its lines break (81a7200c14c2-7 F2, T4x).
func TestEvidenceStore_NamesTheEnvironmentBoundInPages(t *testing.T) {
	root := skillsRepoRoot(t)
	for _, file := range []string{"evidence.sh", "PROVENANCE.md"} {
		text := prose(readRepoFile(t, root, evidenceDirRel+"/"+file))
		for _, want := range []string{"32 pages", "128 KiB with 4 KiB pages", "ARG_MAX", "the caller's exec fails"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q", file, want)
			}
		}
		if strings.Contains(text, "128 KiB on Linux") {
			t.Errorf("%s says 128 KiB on Linux, which holds only with 4 KiB pages", file)
		}
		if strings.Contains(text, "follows the stack limit") {
			t.Errorf("%s says ARG_MAX follows the stack limit, where the text may state only what was measured (d0108e5f1245-5 F2)", file)
		}
	}
	if text := prose(readRepoFile(t, root, evidenceDirRel+"/evidence.sh")); !strings.Contains(text, "between 128 KiB and 6 MiB") {
		t.Errorf("evidence.sh lacks %q, the measured range of ARG_MAX (d0108e5f1245-5 F2)", "between 128 KiB and 6 MiB")
	}
}
