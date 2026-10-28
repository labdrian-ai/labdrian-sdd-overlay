package oddtask_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
)

func TestParseCitationReadsTheSpecLine(t *testing.T) {
	cases := map[string]struct {
		prompt string
		want   oddtask.Citation
	}{
		"the form of the ODD protocol": {
			"Spec: odd/tasks/own-rdd.md, T2, S3-S4",
			oddtask.Citation{Path: "odd/tasks/own-rdd.md", Tasks: []string{"T2"}},
		},
		"an absolute path, several tasks and specs": {
			"Do it.\nSpec: /home/u/repo/odd/tasks/f.md, T11a, T11b, S14, S1-S3\nMore text.",
			oddtask.Citation{Path: "/home/u/repo/odd/tasks/f.md", Tasks: []string{"T11a", "T11b"}},
		},
		"leading white space and a trailing carriage return": {
			"\t  Spec: odd/tasks/a.b_c-1.md, T7\r\nrest",
			oddtask.Citation{Path: "odd/tasks/a.b_c-1.md", Tasks: []string{"T7"}},
		},
		"a path that is cleaned": {
			"Spec: /r/odd/x/../tasks/./f.md, T1",
			oddtask.Citation{Path: "/r/odd/tasks/f.md", Tasks: []string{"T1"}},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, reason := oddtask.ParseCitation(c.prompt)
			if reason != "" {
				t.Fatalf("reason = %q, want none", reason)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("citation = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestParseCitationRefusesWhatIsNotOneCitation(t *testing.T) {
	const none = `the prompt cites no task (it has no line "Spec: odd/tasks/<feature>.md, T<n>")`
	const many = `the prompt has more than one "Spec:" line`
	const bad = `the "Spec:" line does not name a feature document odd/tasks/<feature>.md and at least one task T<n>`
	cases := map[string]struct {
		prompt string
		want   oddtask.Reason
	}{
		"an empty prompt":                        {"", none},
		"no Spec line":                           {"Implement T2 of odd/tasks/f.md", none},
		"a Spec word that does not start a line": {"see Spec: odd/tasks/f.md, T1", none},
		"no space after the colon":               {"Spec:odd/tasks/f.md, T1", none},
		"two Spec lines":                         {"Spec: odd/tasks/f.md, T1\nSpec: odd/tasks/f.md, T2", many},
		"no item":                                {"Spec: odd/tasks/f.md", bad},
		"only specs":                             {"Spec: odd/tasks/f.md, S14", bad},
		"an item that is neither":                {"Spec: odd/tasks/f.md, T1, fix", bad},
		"a task with two letters":                {"Spec: odd/tasks/f.md, T1ab", bad},
		"a task in capitals":                     {"Spec: odd/tasks/f.md, T1A", bad},
		"a spec range without its second S":      {"Spec: odd/tasks/f.md, T1, S1-2", bad},
		"an empty item":                          {"Spec: odd/tasks/f.md, T1, ", bad},
		"not a feature document directory":       {"Spec: docs/f.md, T1", bad},
		"a directory that is not tasks":          {"Spec: odd/plans/f.md, T1", bad},
		"a path of two parts":                    {"Spec: tasks/f.md, T1", bad},
		"a path that is only a name":             {"Spec: f.md, T1", bad},
		"tasks outside odd":                      {"Spec: x/tasks/f.md, T1", bad},
		"a document in a subdirectory":           {"Spec: odd/tasks/sub/f.md, T1", bad},
		"a name that starts with a dot":          {"Spec: odd/tasks/.f.md, T1", bad},
		"a name with another extension":          {"Spec: odd/tasks/f.txt, T1", bad},
		"no name":                                {"Spec: odd/tasks/.md, T1", bad},
		"a path that cleans out of the shape":    {"Spec: odd/tasks/x/../../f.md, T1", bad},
		"an empty path":                          {"Spec: , T1", bad},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, got := oddtask.ParseCitation(c.prompt); got != c.want {
				t.Fatalf("reason = %q, want %q", got, c.want)
			}
		})
	}
}

// doc builds a feature document with the given task lines and Log lines.
func doc(tasks []string, log []string) string {
	return "# Feature\n\nheader\n\n## Specs\n\n- **S1** x\n\n## Tasks\n\n" + strings.Join(tasks, "\n") +
		"\n\n## Log\n\n" + strings.Join(log, "\n") + "\n"
}

const shaperL38 = "- **L38** (2026-10-28, shaper): Design."

func TestDecideAllowsATaskMarkedAndRecordedByTheShaper(t *testing.T) {
	text := doc([]string{
		"- [ ] T11 — S14 — delegated — big — shaped: L38 — commit: -",
		"- [x] T11a — S14 (a) — delegated — domain — shaped: L38 — commit: -",
	}, []string{"- **L37** (2026-10-28, owner): ask", shaperL38})
	d := oddtask.ParseDocument(text)
	for _, tasks := range [][]string{{"T11"}, {"T11a"}, {"T11a", "T11"}} {
		if got := oddtask.Decide("/r/odd/tasks/f.md", d, tasks); got != "" {
			t.Errorf("Decide(%v) = %q, want allowed", tasks, got)
		}
	}
}

func TestDecideNamesTheFirstTaskThatFails(t *testing.T) {
	const p = "/r/odd/tasks/f.md"
	text := doc([]string{
		"- [ ] T1 — S1 — delegated — ok — shaped: L38 — commit: -",
		"- [ ] T2 — S1 — delegated — unmarked — commit: -",
		"- [ ] T3 — S1 — delegated — orphan — shaped: L9 — commit: -",
		"- [ ] T4 — S1 — delegated — dup — shaped: L38 — commit: -",
		"- [x] T4 — S1 — delegated — dup — shaped: L38 — commit: -",
		"- [ ] T5 — S1 — delegated — leading zero — shaped: L038 — commit: -",
		"- [ ] T6 — S1 — delegated — zero — shaped: L0 — commit: -",
		"- [ ] T7 — S1 — delegated — twice — shaped: L38 — shaped: L38 — commit: -",
		"- [ ] T8 — S1 — delegated — prose shaped: L38 — commit: -",
		"- [ ] T9 — S1 — delegated — shaped: L38 extra — commit: -",
		"- [ ] T10 — S1 — delegated — shaped: L38\r",
		"- [ ] T12 — S1 — delegated — shaped: l38 — commit: -",
		"- [ ] T13 — S1 — delegated — shaped: L10 — commit: -",
		"- [ ] T14 — S1 — delegated — shaped: L11 — commit: -",
	}, []string{
		"- **L10** (2026-10-28, parent): not the shaper",
		"- **L11** (2026-10-28, shaper): once",
		"- **L11** (2026-10-29, shaper): twice",
		shaperL38,
	})
	d := oddtask.ParseDocument(text)
	cases := []struct {
		tasks []string
		want  oddtask.Reason
	}{
		{[]string{"T1", "T2", "T3"}, "task T2 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T3"}, "task T3 of " + p + ` is marked "shaped: L9", but the ## Log of the document has no entry L9 written by the shaper`},
		{[]string{"T4"}, "task T4 appears more than once in the ## Tasks section of " + p},
		{[]string{"T99"}, "task T99 is not in the ## Tasks section of " + p},
		{[]string{"T5"}, "task T5 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T6"}, "task T6 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T7"}, "task T7 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T8"}, "task T8 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T9"}, "task T9 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T10"}, ""},
		{[]string{"T12"}, "task T12 of " + p + ` is not marked as designed by the shaper (its line has no "shaped: L<n>")`},
		{[]string{"T13"}, "task T13 of " + p + ` is marked "shaped: L10", but the ## Log of the document has no entry L10 written by the shaper`},
		{[]string{"T14"}, "task T14 of " + p + ` is marked "shaped: L11", but the ## Log of the document has no entry L11 written by the shaper`},
	}
	for _, c := range cases {
		if got := oddtask.Decide(p, d, c.tasks); got != c.want {
			t.Errorf("Decide(%v) = %q, want %q", c.tasks, got, c.want)
		}
	}
}

func TestDecideLooksOnlyInTheTasksSectionForTasksAndInTheLogSectionForEntries(t *testing.T) {
	const p = "/r/odd/tasks/f.md"
	text := "# F\n\n## Specs\n\n- [ ] T1 — S1 — in specs — shaped: L5 — commit: -\n- **L5** (2026-10-28, shaper): in specs\n\n" +
		"## Tasks\n\n- [ ] T2 — S1 — real — shaped: L5 — commit: -\n\n### Sub\n\n- [ ] T3 — S1 — under a subheading — shaped: L5 — commit: -\n\n" +
		"## Notes\n\n- [ ] T4 — S1 — in notes — shaped: L5 — commit: -\n\n## Log\n\n- **L1** (2026-10-28, owner): x\n\n## Later\n\n- **L5** (2026-10-28, shaper): after the log\n"
	d := oddtask.ParseDocument(text)
	cases := []struct {
		task string
		want oddtask.Reason
	}{
		{"T1", "task T1 is not in the ## Tasks section of " + p},
		{"T4", "task T4 is not in the ## Tasks section of " + p},
		// "### " does not start "## ", so a task under a subheading is still in the section.
		{"T3", "task T3 of " + p + ` is marked "shaped: L5", but the ## Log of the document has no entry L5 written by the shaper`},
		{"T2", "task T2 of " + p + ` is marked "shaped: L5", but the ## Log of the document has no entry L5 written by the shaper`},
	}
	for _, c := range cases {
		if got := oddtask.Decide(p, d, []string{c.task}); got != c.want {
			t.Errorf("Decide(%s) = %q, want %q", c.task, got, c.want)
		}
	}
}

func TestDecideDoesNotTakeATaskForItsPrefix(t *testing.T) {
	const p = "/r/odd/tasks/f.md"
	d := oddtask.ParseDocument(doc([]string{"- [ ] T11a — S1 — x — shaped: L38 — commit: -"}, []string{shaperL38}))
	if got, want := oddtask.Decide(p, d, []string{"T11"}), oddtask.Reason("task T11 is not in the ## Tasks section of "+p); got != want {
		t.Fatalf("Decide(T11) = %q, want %q", got, want)
	}
}

func TestMessageIsTheWholeDenial(t *testing.T) {
	want := `labdrian builder guard: why. The builder never designs: launch the shaper first to design the task and mark its line "shaped: L<n>", then launch the builder with the line "Spec: odd/tasks/<feature>.md, T<n>" in its prompt. This guard reads text only, so it is a speed bump, not a security boundary.`
	if got := oddtask.Message("why"); got != want {
		t.Fatalf("Message = %q, want %q", got, want)
	}
}

func TestReasonsWordedByTheSpec(t *testing.T) {
	cases := map[string]struct{ got, want string }{
		"no citation":   {string(oddtask.ReasonNoCitation), `the prompt cites no task (it has no line "Spec: odd/tasks/<feature>.md, T<n>")`},
		"many":          {string(oddtask.ReasonManyCitations), `the prompt has more than one "Spec:" line`},
		"bad":           {string(oddtask.ReasonBadCitation), `the "Spec:" line does not name a feature document odd/tasks/<feature>.md and at least one task T<n>`},
		"internal":      {string(oddtask.ReasonInternal), "the guard hit an internal error and could not check this launch"},
		"too large":     {string(oddtask.TooLargeReason(4194304)), "the Agent call is larger than 4194304 bytes and could not be checked"},
		"unreadable":    {string(oddtask.UnreadableReason("/r/odd/tasks/f.md", oddtask.CauseNotFound)), "the feature document /r/odd/tasks/f.md cannot be read (not found)"},
		"read error":    {(&oddtask.ReadError{Path: "/p", Cause: oddtask.CauseLink}).Error(), "the feature document /p cannot be read (" + oddtask.CauseLink + ")"},
		"cause link":    {oddtask.CauseLink, "a symbolic link"},
		"cause regular": {oddtask.CauseNotRegular, "not a regular file"},
		"cause large":   {oddtask.CauseTooLarge, "larger than 8 MiB"},
		"cause no cwd":  {oddtask.CauseNoCwd, "a relative path and no working directory"},
		"cause missing": {oddtask.CauseNotFound, "not found"},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", name, c.got, c.want)
		}
	}
}
