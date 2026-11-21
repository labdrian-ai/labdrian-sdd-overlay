package main

// The verb 'workflow goal-base' (labdrian-orchestrator-opencode T55, S3, L152): it prints the four fields of a Goal v3
// the engine knows, so the shaper copies them and writes only the judgment fields. These tests go through the public
// entry point of the workflow subcommand.

import (
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// oddProfileDirective is the memory default of the profile 'workflow open' creates, as the JSON the verb must print, so
// the test follows the profile and spells no source.
func oddProfileDirective(t *testing.T) string {
	t.Helper()
	profile, err := workflowprofile.Resolve(app.Profile)
	if err != nil {
		t.Fatal(err)
	}
	directive, err := memoryscope.DefaultFor(profile)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(directive)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestWorkflowGoalBasePrintsTheGoalFieldsTheEngineKnows(t *testing.T) {
	workflowTestStateHome(t)
	repo := filepath.Join(t.TempDir(), "checkout")
	gitRepository(t, repo, "git@github.com:acme/demo.git")

	r := runWorkflowTest([]string{"goal-base", "--feature", "my-feature"}, repo)

	want := `{"version":3,"project_id":"demo","goal_id":"my-feature","memory_directive":` + oddProfileDirective(t) + "}\n"
	if r.code != 0 || r.stderr != "" || r.stdout != want {
		t.Errorf("code=%d stderr=%q stdout=%q, want exit 0, empty stderr and stdout %q", r.code, r.stderr, r.stdout, want)
	}
}

func TestWorkflowGoalBaseNamesTheProjectAsTheSharedIdentityDoes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		build   func(t *testing.T) string
		extra   []string
		project string
	}{
		{"a declared name", func(t *testing.T) string {
			repo := filepath.Join(t.TempDir(), "checkout")
			gitRepository(t, repo, "git@github.com:acme/widgets.git")
			writeMemoryTestFile(t, repo, ".longterm-mem-project", "team.alpha\n")
			return repo
		}, nil, "team.alpha"},
		{"no remote, the main checkout's directory", func(t *testing.T) string {
			return fixtureRepo(t, "repo-x")
		}, nil, "repo-x"},
		{"a linked worktree of it", func(t *testing.T) string {
			return fixtureLinkedWorktree(t, fixtureRepo(t, "repo-x"), "side")
		}, nil, "repo-x"},
		{"--project", func(t *testing.T) string {
			return fixtureRepo(t, "repo-x")
		}, []string{"--project", "custom-id"}, "custom-id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workflowTestStateHome(t)
			r := runWorkflowTest(append([]string{"goal-base", "--feature", "my-feature"}, tt.extra...), tt.build(t))
			want := `{"version":3,"project_id":"` + tt.project + `","goal_id":"my-feature","memory_directive":` + oddProfileDirective(t) + "}\n"
			if r.code != 0 || r.stderr != "" || r.stdout != want {
				t.Errorf("code=%d stderr=%q stdout=%q, want exit 0, empty stderr and stdout %q", r.code, r.stderr, r.stdout, want)
			}
		})
	}
}

func TestWorkflowGoalBaseRefuses(t *testing.T) {
	plain := func(t *testing.T) string { return fixtureRepo(t, "repo-x") }
	myProject := func(t *testing.T) string {
		repo := fixtureRepo(t, "repo-x")
		writeMemoryTestFile(t, repo, ".longterm-mem-project", "My Project\n")
		return repo
	}
	broken := func(t *testing.T) string {
		dir := filepath.Join(t.TempDir(), "broken")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeMemoryTestFile(t, dir, ".git", "this is not a git file\n")
		return dir
	}
	for _, tt := range []struct {
		name  string
		build func(t *testing.T) string
		args  []string
		code  int
		want  string // the whole stderr, "" meaning the test computes it
	}{
		{"no feature", plain, []string{"goal-base"}, 1, "error: workflow goal-base: --feature is required\n"},
		{"a feature with no value", plain, []string{"goal-base", "--feature"}, 1, "error: workflow goal-base: --feature requires a value\n"},
		{"an unknown flag", plain, []string{"goal-base", "--feature", "f", "--x"}, 1, "error: workflow goal-base: unknown flag \"--x\"\n"},
		{"a positional argument", plain, []string{"goal-base", "--feature", "f", "x"}, 1, "error: workflow goal-base: unexpected argument \"x\"\n"},
		{"a feature that is no name", plain, []string{"goal-base", "--feature", "../x"}, 2, "error: workflow goal-base: feature \"../x\" is not a feature name\n"},
		{"a project outside the allowed set", plain, []string{"goal-base", "--feature", "f", "--project", "a b"}, 2,
			"error: workflow goal-base: project_id \"a b\" contains a character outside the allowed set [A-Za-z0-9._-]\n"},
		{"an identity that is no project id", myProject, []string{"goal-base", "--feature", "f"}, 2,
			"error: workflow goal-base: the project identity \"My Project\" gives no project id (project_id \"My Project\" contains a character outside the allowed set [A-Za-z0-9._-]); pass --project <id>\n"},
		{"an identity that cannot be resolved", broken, []string{"goal-base", "--feature", "f"}, 2, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workflowTestStateHome(t)
			cwd := tt.build(t)
			want := tt.want
			if want == "" {
				_, err := sharedProjectRules("").Resolve(cwd, "")
				if err == nil {
					t.Fatal("test bug: the resolver reads the broken repository")
				}
				want = "error: workflow goal-base: the project identity could not be resolved (" + err.Error() + "); pass --project <id>\n"
			}
			r := runWorkflowTest(tt.args, cwd)
			if r.code != tt.code || r.stdout != "" || r.stderr != want {
				t.Errorf("code=%d stdout=%q stderr=%q, want exit %d, empty stdout and stderr %q", r.code, r.stdout, r.stderr, tt.code, want)
			}
		})
	}
}

// goalBaseDigest hashes every regular file under each root, by path.
func goalBaseDigest(t *testing.T, roots ...string) map[string][sha256.Size]byte {
	t.Helper()
	digest := map[string][sha256.Size]byte{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() {
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				digest[path] = sha256.Sum256(raw)
			} else if !entry.IsDir() {
				digest[path] = [sha256.Size]byte{1}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return digest
}

func TestWorkflowGoalBaseWritesNothing(t *testing.T) {
	workflowTestStateHome(t)
	state := os.Getenv("XDG_STATE_HOME")
	repo := filepath.Join(t.TempDir(), "checkout")
	gitRepository(t, repo, "git@github.com:acme/demo.git")
	before := goalBaseDigest(t, repo, state)

	first := runWorkflowTest([]string{"goal-base", "--feature", "my-feature"}, repo)
	second := runWorkflowTest([]string{"goal-base", "--feature", "my-feature"}, repo)

	if first.code != 0 || first.stdout == "" || first.stdout != second.stdout {
		t.Errorf("runs: %+v and %+v, want two equal printed lines", first, second)
	}
	after := goalBaseDigest(t, repo, state)
	var changed []string
	for path, sum := range after {
		if before[path] != sum {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	if len(changed) != 0 {
		t.Errorf("the verb changed %s", strings.Join(changed, ", "))
	}
}
