package skills

// Checks of the two overlay skills that the shaper, the builder and delivery follow in place of
// gentle's SDD skills (C15, S1 and S2), read from the repository the way role_chain_artifact_test.go
// reads the role-chain skill: odd-feature-doc, the format of odd/tasks/<feature>.md and the part each
// role plays in it, and test-first, the RED -> GREEN -> REFACTOR rule of the builder. Their registry
// entries are checked in odd_role_skills_registry_test.go, and the role bodies that cite them in
// role_agents_artifact_test.go.

import (
	"reflect"
	"strings"
	"testing"
)

// oddRoleSkills are the two skills, by the name of their directory.
var oddRoleSkills = []string{"odd-feature-doc", "test-first"}

// oddRoleSkillFile reads skills/<name>/SKILL.md: the whole file, its frontmatter and its body.
func oddRoleSkillFile(t *testing.T, name string) (whole, frontmatter, body string) {
	t.Helper()
	whole = readRepoFile(t, skillsRepoRoot(t), "skills/"+name+"/SKILL.md")
	var err error
	if frontmatter, body, err = SplitSkillFile([]byte(whole)); err != nil {
		t.Fatalf("skills/%s/SKILL.md: %v", name, err)
	}
	return whole, frontmatter, body
}

// oddSkillTable reads the first table of text: its header cells, and the cells of each row under the
// separator line.
func oddSkillTable(text string) (header []string, rows [][]string) {
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		var cells []string
		for _, cell := range strings.Split(strings.Trim(line, "|"), "|") {
			cells = append(cells, strings.TrimSpace(cell))
		}
		switch {
		case header == nil:
			header = cells
		case strings.Trim(cells[0], "-") == "":
			// the separator line under the header
		default:
			rows = append(rows, cells)
		}
	}
	return header, rows
}

// oddRule is one rule of a skill and the fragments of its text that carry it.
type oddRule struct {
	what      string
	fragments []string
}

// assertRules fails for every fragment of every rule that the text lacks, naming the rule.
func assertRules(t *testing.T, skill, section, text string, rules []oddRule) {
	t.Helper()
	for _, rule := range rules {
		for _, fragment := range rule.fragments {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s: the section %s must say that it %s: %q is missing", skill, section, rule.what, fragment)
			}
		}
	}
}

// Both skills are named for their directory (the Pi package build refuses a name that differs), open
// their description with the trigger words the skill registry matches on, and carry the license, author
// and version that the lint requires.
func TestOddRoleSkillsAreNamedForTheirDirectoryAndOpenTheirDescriptionWithTheTrigger(t *testing.T) {
	for _, name := range oddRoleSkills {
		t.Run(name, func(t *testing.T) {
			_, frontmatter, _ := oddRoleSkillFile(t, name)
			assertStringValue(t, parseYAMLScalar(t, frontmatter, "name"), name)
			if description := parseYAMLScalar(t, frontmatter, "description"); !strings.HasPrefix(description, "Trigger: ") {
				t.Errorf("the description = %q, want it to open with %q, as the style guide asks", description, "Trigger: ")
			}
			for _, key := range []string{"license", "author", "version"} {
				if parseYAMLScalar(t, frontmatter, key) == "" {
					t.Errorf("the frontmatter key %q is empty", key)
				}
			}
		})
	}
}

// Both skills are within the lint budget (the hard and the recommended bounds of the body): no hard
// error and no warning, so that a role agent loads a short file.
func TestOddRoleSkillsAreWithinTheLintBudget(t *testing.T) {
	for _, name := range oddRoleSkills {
		t.Run(name, func(t *testing.T) {
			whole, _, body := oddRoleSkillFile(t, name)
			hard, warnings := LintSkillFile([]byte(whole))
			for _, err := range hard {
				t.Errorf("hard lint error: %v", err)
			}
			for _, w := range warnings {
				t.Errorf("lint warning [%s]: %s", w.Rule, w.Msg)
			}
			if tokens := EstimateBodyTokens(body); tokens > BodyRecommendedTokens {
				t.Errorf("the body is an estimated %d tokens, want at most %d", tokens, BodyRecommendedTokens)
			}
		})
	}
}

// The skills are the overlay's own words (C15): neither carries a marker or a block of gentle-ai, and
// neither names the SDD artifacts and modules that the feature document and test-first replace.
func TestOddRoleSkillsCarryNoGentleMarkerAndNameNoSDDArtifact(t *testing.T) {
	for _, name := range oddRoleSkills {
		t.Run(name, func(t *testing.T) {
			whole, _, _ := oddRoleSkillFile(t, name)
			for _, banned := range []string{"<!--", "gentle-ai", "sdd-", "openspec", "strict-tdd", "Strict TDD", "TRIANGULATE", "apply-progress"} {
				if strings.Contains(whole, banned) {
					t.Errorf("skills/%s/SKILL.md names %q, which belongs to the SDD skills it replaces", name, banned)
				}
			}
		})
	}
}

// odd-feature-doc gives the format of odd/tasks/<feature>.md (C15, S1): the four parts in their order,
// the header, the specs that quote the user, the one line of a task, and the Log that only grows.
func TestOddFeatureDocSkillGivesTheFormatOfTheDocument(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "odd-feature-doc")
	headings, under := roleChainSections(body)
	if want := []string{"Activation Contract", "Hard Rules", "Decision Gates", "Output Contract"}; !reflect.DeepEqual(headings, want) {
		t.Errorf("the sections of the skill = %v, want %v", headings, want)
	}
	if !strings.Contains(under["Activation Contract"], "`odd/tasks/<feature>.md`") {
		t.Error("the Activation Contract must name `odd/tasks/<feature>.md`")
	}
	assertRules(t, "odd-feature-doc", "Hard Rules", under["Hard Rules"], []oddRule{
		{"reads the document up to the Log before acting", []string{"Read the document up to `## Log` before acting", "report which `S#` you covered"}},
		{"has four parts in order and no other", []string{"Four parts, in this order and no other: a header, `## Specs`, `## Tasks`, `## Log`."}},
		{"has a header of the objective, the branch, the strategy and the runner", []string{"Header, two or three lines: the objective, the branch, the delivery strategy, the test runner."}},
		{"quotes the user in each spec", []string{"quoted verbatim, in the user's language", "Never summarize a quote", "never add a requirement the user did not ask for", "mark an assumption as an assumption"}},
		{"has one line per task with its S#, route and commit", []string{"`- [ ] T1 — S1-S2 — inline — what it delivers — commit: -`", "The box is the status", "`inline` or `delegated`", "`-` until the task is committed"}},
		{"keeps the Log append-only with L1 the request", []string{"The Log is append-only", "`L1` is the user's request, verbatim", "`- **L<n>** (<date>, <who>): ...`", "Never edit, reorder or delete an entry"}},
		{"treats a requirement change as a new entry, one spec and one task", []string{"A requirement change: append the user's words to the Log, rewrite only the affected `S#`, reopen only its task."}},
		{"checks a task off only after observing it", []string{"Check a task off only after you observed its outcome and its checks."}},
	})
}

// odd-feature-doc says what each role writes in the document and what it never touches (C15, S1): the
// shaper writes the specs and the tasks, the builder checks off its tasks and appends its evidence to the
// Log, delivery closes the document in place with a last entry. No other role has a part.
func TestOddFeatureDocSkillGivesEachRoleItsPart(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "odd-feature-doc")
	_, under := roleChainSections(body)
	gates := under["Decision Gates"]
	if !strings.Contains(gates, "Write only the part of your role.") {
		t.Error("the Decision Gates must tell a role to write only its own part")
	}
	header, rows := oddSkillTable(gates)
	if want := []string{"Role", "Writes", "Never"}; !reflect.DeepEqual(header, want) {
		t.Fatalf("the columns of the table = %v, want %v", header, want)
	}
	var roles []string
	byRole := map[string][]string{}
	for _, row := range rows {
		if len(row) != 3 {
			t.Fatalf("the row %v has %d cells, want 3", row, len(row))
		}
		roles = append(roles, row[0])
		byRole[row[0]] = row
	}
	if want := []string{"shaper", "builder", "delivery"}; !reflect.DeepEqual(roles, want) {
		t.Fatalf("the roles of the table = %v, want %v", roles, want)
	}
	for _, part := range []struct {
		role          string
		writes, never []string
	}{
		{"shaper",
			[]string{"the header", "`## Specs`", "`## Tasks` with every box open", "`L1`, the request exactly as the prompt quotes it"},
			[]string{"check a box", "write code"}},
		{"builder",
			[]string{"`[x]` and the commit on each finished task", "one Log entry", "the `S#` covered", "the commands run with their exact results", "the `test-first` table"},
			[]string{"edit a spec", "an earlier entry"}},
		{"delivery",
			[]string{"one final Log entry with the state at close", "unfinished tasks and missing artifacts named as unfinished"},
			[]string{"copy, move or rewrite the document"}},
	} {
		for _, fragment := range part.writes {
			if !strings.Contains(byRole[part.role][1], fragment) {
				t.Errorf("what the %s writes must say %q; got %q", part.role, fragment, byRole[part.role][1])
			}
		}
		for _, fragment := range part.never {
			if !strings.Contains(byRole[part.role][2], fragment) {
				t.Errorf("what the %s never does must say %q; got %q", part.role, fragment, byRole[part.role][2])
			}
		}
	}
}

// test-first is the rule of the builder in place of sdd-apply/strict-tdd.md (C15, S2): no code before a
// failing test that was seen to fail, the least test and the least code, one test for each rule of the
// spec and one for each existing behavior the change touches, green after every refactoring step,
// assertions that can fail, and no evidence made up.
func TestTestFirstSkillGivesTheRedGreenRefactorRule(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "test-first")
	headings, under := roleChainSections(body)
	if want := []string{"Activation Contract", "Hard Rules", "Decision Gates", "Output Contract"}; !reflect.DeepEqual(headings, want) {
		t.Errorf("the sections of the skill = %v, want %v", headings, want)
	}
	assertRules(t, "test-first", "Hard Rules", under["Hard Rules"], []oddRule{
		{"runs the tests of an existing file before changing it, as the safety net", []string{"Before you change an existing file, run its tests; that run is the safety net."}},
		{"allows no code before a failing test that was seen to fail", []string{"No code before a failing test", "RED is a run you saw fail for the reason the `S#` gives"}},
		{"asks for the least test and the least code", []string{"no more test than it takes to fail", "no more code than it takes to pass"}},
		{"asks for one RED test per rule of the spec", []string{"One RED test per rule of the `S#`", "through the public interface", "every observable effect of the rule", "covering the cases the rule names"}},
		{"asks for one test for each existing behavior touched", []string{"one test that its previous behavior still holds", "passing before you change anything", "Add no other cases."}},
		{"fixes the code and never the test to reach GREEN", []string{"GREEN: fix the code, never the test.", "Run only the relevant tests during the cycle and the whole suite once, at the end of the task."}},
		{"keeps the tests green through the REFACTOR", []string{"REFACTOR: run the tests after every step and revert the step that turns them red."}},
		{"asks for assertions that can fail", []string{"calls production code", "would fail if the code were wrong"}},
		{"keeps the tests off real state", []string{"Tests never touch real state", "`mktemp -d` directory outside the worktree", "report its path"}},
		{"never invents evidence", []string{"Never invent RED or GREEN evidence, or a runner."}},
	})
}

// test-first says what to do where the cycle does not fit (C15, S2): a refactoring, tests that already
// fail, a runner that breaks, and a change no test can observe.
func TestTestFirstSkillSaysWhatToDoWhereTheCycleDoesNotFit(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "test-first")
	_, under := roleChainSections(body)
	header, rows := oddSkillTable(under["Decision Gates"])
	if want := []string{"Situation", "Do"}; !reflect.DeepEqual(header, want) {
		t.Fatalf("the columns of the table = %v, want %v", header, want)
	}
	for i, want := range []struct {
		situation string
		does      []string
	}{
		{"New or changed behavior", []string{"RED, GREEN, REFACTOR"}},
		{"Refactor, behavior must not change", []string{"Pin the current behavior with a passing test first"}},
		{"Tests fail before you touch anything", []string{"report them as pre-existing", "except the ones the prompt lists as known", "fix none"}},
		{"Runner fails for an infrastructure reason", []string{"`blocked`", "go on to the next task"}},
		{"Documentation only, no runner, or no meaningful RED", []string{"Name the exception", "`n/a` in the RED cell"}},
	} {
		if i >= len(rows) {
			t.Fatalf("the table has %d rows, want at least %d", len(rows), i+1)
		}
		if rows[i][0] != want.situation {
			t.Errorf("row %d is the situation %q, want %q", i+1, rows[i][0], want.situation)
		}
		for _, fragment := range want.does {
			if !strings.Contains(rows[i][1], fragment) {
				t.Errorf("for %q the skill must say %q; got %q", want.situation, fragment, rows[i][1])
			}
		}
	}
	if len(rows) != 5 {
		t.Errorf("the table has %d rows, want 5", len(rows))
	}
}

// Each skill is deployed by a manifest row of the overlay (custom, not from upstream) that takes the
// skill route, to the skills directory of Claude Code, Codex and Pi (C15, S5), like role-chain: the
// runtimes the role agents run on. The skill is the one file of its directory, so the row of SKILL.md
// is the only row of the skill.
func TestOddRoleSkillsAreDeployedByTheSkillRoutesToClaudeCodexAndPi(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	for _, name := range oddRoleSkills {
		path := name + "/SKILL.md"
		var found []ManifestRow
		for _, row := range rows {
			if strings.HasPrefix(row.Path, name+"/") {
				found = append(found, row)
			}
		}
		if want := []ManifestRow{{Path: path, Tag: "custom"}}; !reflect.DeepEqual(found, want) {
			t.Errorf("rows of %s/ = %+v, want exactly %+v", name, found, want)
			continue
		}
		for _, target := range []string{"claude", "codex", "pi"} {
			t.Run(name+"/"+target, func(t *testing.T) {
				routes, err := ResolveRoutes(rows, false, "/o", RouteDestinations{Target: target, SkillsRoot: "/h/skills", AgentsRoot: "/h/agents"})
				if err != nil {
					t.Fatalf("ResolveRoutes over the manifest of the repository: %v", err)
				}
				want := ResolvedRoute{Path: path, Route: RouteSkill, RepoRel: "skills/" + path, RepoSource: "/o/skills/" + path, Dest: "/h/skills/" + path}
				for _, r := range routes {
					if r.Path == want.Path {
						if r != want {
							t.Errorf("%s resolves to %+v, want %+v", path, r, want)
						}
						return
					}
				}
				t.Errorf("no route for %s", path)
			})
		}
	}
}

// test-first asks for one table of evidence (C15, S2): a row for each task and a cell for the safety net,
// RED, GREEN and REFACTOR, each the command that was run and its exact result. The table goes in the
// Log entry of the task, where odd-feature-doc has the builder put it, and a task done without a
// failing test first fails.
func TestTestFirstSkillReportsItsEvidenceInATable(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "test-first")
	_, under := roleChainSections(body)
	header, _ := oddSkillTable(under["Output Contract"])
	if want := []string{"Task", "Test", "Safety net", "RED", "GREEN", "REFACTOR"}; !reflect.DeepEqual(header, want) {
		t.Errorf("the columns of the evidence table = %v, want %v", header, want)
	}
	assertRules(t, "test-first", "Output Contract", under["Output Contract"], []oddRule{
		{"puts the table in the Log entry of the task and in the summary of the envelope", []string{"Put this table in the Log entry of the task and in the summary of your envelope"}},
		{"fills each cell with a command and its exact result", []string{"Each cell is the command you ran and its exact result, or `not run`."}},
		{"fails a task done without a failing test first", []string{"A task finished without a failing test first is FAILED, unless the last row applies."}},
	})
}
