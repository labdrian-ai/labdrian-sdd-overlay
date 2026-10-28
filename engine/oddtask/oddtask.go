// Package oddtask is the vocabulary of the builder guard (own-rdd S14): what a prompt cites as the
// task to build, what a feature document says about that task, and the pure rule that decides whether
// the builder may be launched on it. The shaper marks a task line it designed with the segment
// "shaped: L<n>", where L<n> is the Log entry (<date>, shaper) that records the design; the guard
// refuses a builder launch whose prompt does not cite such a task.
//
// The package reads no file and runs no command: the caller hands it the prompt and the text of the
// document. It is a speed bump against forgetting the shaper, not a boundary against forging the mark.
package oddtask

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Reason is why a builder launch is refused, worded to follow "labdrian builder guard: ". The empty
// Reason means the launch is allowed.
type Reason string

// MaxDocumentBytes is the largest feature document the guard reads.
const MaxDocumentBytes = 8 << 20

// Citation is the task a prompt cites: the path as the prompt wrote it, cleaned, and the task ids in
// the order they were written.
type Citation struct {
	Path  string
	Tasks []string
}

const specPrefix = "Spec: "

var (
	taskID      = regexp.MustCompile(`^T[0-9]+[a-z0-9]*$`)
	specRef     = regexp.MustCompile(`^S[0-9]+(-S[0-9]+)?$`)
	featureName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ParseCitation reads the "Spec:" line of a prompt: exactly one line that, after its leading white
// space and a trailing carriage return are dropped, starts "Spec: " and reads
// "Spec: <path>, <item>[, <item>...]". The path, cleaned, ends in odd/tasks/<feature>.md; an item is a
// task id (T<digits> followed by lowercase letters and digits) or a spec reference (S<digits> or S<digits>-S<digits>), and at least one is
// a task id. The Reason is empty when the prompt cites tasks.
func ParseCitation(prompt string) (Citation, Reason) {
	var line string
	found := 0
	for _, l := range strings.Split(prompt, "\n") {
		l = strings.TrimSuffix(strings.TrimLeft(l, " \t\v\f"), "\r")
		if strings.HasPrefix(l, specPrefix) {
			line = strings.TrimPrefix(l, specPrefix)
			found++
		}
	}
	switch {
	case found == 0:
		return Citation{}, ReasonNoCitation
	case found > 1:
		return Citation{}, ReasonManyCitations
	}
	fields := strings.Split(line, ", ")
	cleaned := ""
	if fields[0] != "" {
		cleaned = path.Clean(fields[0])
	}
	if !documentShape(cleaned) {
		return Citation{}, ReasonBadCitation
	}
	c := Citation{Path: cleaned}
	for _, item := range fields[1:] {
		switch {
		case taskID.MatchString(item):
			c.Tasks = append(c.Tasks, item)
		case specRef.MatchString(item):
		default:
			return Citation{}, ReasonBadCitation
		}
	}
	if len(c.Tasks) == 0 {
		return Citation{}, ReasonBadCitation
	}
	return c, ""
}

// documentShape is whether p, cleaned, ends in odd/tasks/<feature>.md.
func documentShape(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) < 3 {
		return false
	}
	n := len(parts)
	name, ok := strings.CutSuffix(parts[n-1], ".md")
	return ok && parts[n-2] == "tasks" && parts[n-3] == "odd" && featureName.MatchString(name)
}

// Document is what the guard keeps of a feature document: the lines of its ## Tasks section by task id,
// and how many ## Log entries each number has that start "- **L<n>** (<date>, shaper)".
type Document struct {
	tasks   map[string][]string
	shapers map[int]int
}

var (
	taskLine   = regexp.MustCompile(`^- \[[ x]\] (T[0-9]+[a-z0-9]*) — `)
	shapedMark = regexp.MustCompile(`^shaped: L([1-9][0-9]*)$`)
	shaperLog  = regexp.MustCompile(`^- \*\*L([1-9][0-9]*)\*\* \([0-9]{4}-[0-9]{2}-[0-9]{2}, shaper\)`)
)

// ParseDocument reads the text of a feature document. The ## Tasks section runs from the line
// "## Tasks" to the next line that starts "## "; the ## Log section likewise.
func ParseDocument(text string) Document {
	d := Document{tasks: map[string][]string{}, shapers: map[int]int{}}
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "## ") {
			section = line
			continue
		}
		switch section {
		case "## Tasks":
			if m := taskLine.FindStringSubmatch(line); m != nil {
				d.tasks[m[1]] = append(d.tasks[m[1]], line)
			}
		case "## Log":
			if m := shaperLog.FindStringSubmatch(line); m != nil {
				n, _ := strconv.Atoi(m[1]) // a number too long to fit never matches a task line the same way.
				d.shapers[n]++
			}
		}
	}
	return d
}

// mark is the number n of the one segment "shaped: L<n>" of a task line, or 0 when the line has none or
// more than one segment that begins "shaped:", or that one is not exactly "shaped: L<n>".
func mark(line string) int {
	n := 0
	count := 0
	for _, seg := range strings.Split(line, " — ") {
		if !strings.HasPrefix(seg, "shaped:") {
			continue
		}
		count++
		if m := shapedMark.FindStringSubmatch(seg); m != nil {
			n, _ = strconv.Atoi(m[1])
		}
	}
	if count != 1 {
		return 0
	}
	return n
}

// Decide says whether the builder may be launched on tasks of the document at path, the resolved
// absolute path that its reasons name. Each task must be the one line of the ## Tasks section that
// begins with its id, carry the one mark "shaped: L<n>", and have exactly one ## Log entry L<n> written
// by the shaper. The first task that fails is named. The Reason is empty when every task passes.
func Decide(path string, doc Document, tasks []string) Reason {
	for _, task := range tasks {
		lines := doc.tasks[task]
		switch {
		case len(lines) == 0:
			return Reason(fmt.Sprintf("task %s is not in the ## Tasks section of %s", task, path))
		case len(lines) > 1:
			return Reason(fmt.Sprintf("task %s appears more than once in the ## Tasks section of %s", task, path))
		}
		n := mark(lines[0])
		if n == 0 {
			return Reason(fmt.Sprintf(`task %s of %s is not marked as designed by the shaper (its line has no "shaped: L<n>")`, task, path))
		}
		if doc.shapers[n] != 1 {
			return Reason(fmt.Sprintf(`task %s of %s is marked "shaped: L%d", but the ## Log of the document has no entry L%d written by the shaper`, task, path, n, n))
		}
	}
	return ""
}
