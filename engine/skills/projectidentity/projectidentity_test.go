package projectidentity_test

import (
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/projectidentity"
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// repositories answers the shared port from a table: one repository at /src/demo whose config is
// config, or the error err for every question.
type repositories struct {
	config string
	err    error
}

func (r repositories) Locate(dir string) (resolve.Repository, error) {
	if r.err != nil {
		return resolve.Repository{}, r.err
	}
	if dir != "/src/demo" {
		return resolve.Repository{}, resolve.ErrNotARepository
	}
	return resolve.Repository{WorktreeRoot: "/src/demo", MainWorktreeRoot: "/src/demo", CommonDir: "/src/demo/.git"}, nil
}

func (repositories) DeclaredProject(string) (string, bool, error) { return "", false, nil }

func (r repositories) GitConfig(string) (string, error) { return r.config, nil }

func shared(r repositories) skills.ProjectIdentity {
	return projectidentity.Shared{Rules: resolve.New(r)}
}

// The adapter hands the domain's question to the shared rules and their answer back: the explicit
// id, the origin, the main checkout's name, the directory outside a repository.
func TestSharedAnswersWithTheSharedRules(t *testing.T) {
	origin := "[remote \"origin\"]\n\turl = git@github.com:acme/demo.git\n"
	for _, tt := range []struct {
		name   string
		config string
		q      skills.ProjectQuery
		want   skills.ProjectID
	}{
		{"the explicit id", origin, skills.ProjectQuery{Dir: "/src/demo", Explicit: "given"}, "given"},
		{"the origin", origin, skills.ProjectQuery{Dir: "/src/demo"}, "github.com/acme/demo"},
		{"the main checkout's name", "", skills.ProjectQuery{Dir: "/src/demo"}, "demo"},
		{"the directory outside a repository", "", skills.ProjectQuery{Dir: "/tmp/plain"}, "plain"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, ok, err := shared(repositories{config: tt.config}).Identify(tt.q)
			if err != nil || !ok || id != tt.want {
				t.Errorf("Identify(%+v) = %q, %v, %v, want %q", tt.q, id, ok, err, tt.want)
			}
		})
	}
}

// A repository that could not be read is the domain's "could not tell": the error, and no answer.
func TestSharedSaysItCannotTellWhenTheRulesCannot(t *testing.T) {
	boom := errors.New("cannot look")
	id, ok, err := shared(repositories{err: boom}).Identify(skills.ProjectQuery{Dir: "/src/demo"})
	if !errors.Is(err, boom) || ok || id != "" {
		t.Errorf("Identify = %q, %v, %v, want the error and no answer", id, ok, err)
	}
	// A person who names the project does not need to look.
	if id, ok, err := shared(repositories{err: boom}).Identify(skills.ProjectQuery{Dir: "/src/demo", Explicit: "given"}); err != nil || !ok || id != "given" {
		t.Errorf("Identify with an explicit id = %q, %v, %v, want it answered as given", id, ok, err)
	}
}
