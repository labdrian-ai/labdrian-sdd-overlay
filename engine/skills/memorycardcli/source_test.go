package memorycardcli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

type runnerFunc func(context.Context, []string, string, ...string) ([]byte, []byte, error)

func (f runnerFunc) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	return f(ctx, env, bin, args...)
}

func answering(stdout string) runnerFunc {
	return func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
		return []byte(stdout), nil, nil
	}
}

func TestSourceRunsTheListVerbWithBothFamiliesAndTheGoalPrefix(t *testing.T) {
	var gotEnv []string
	var gotBin string
	var gotArgs []string
	runner := runnerFunc(func(_ context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
		gotEnv, gotBin, gotArgs = env, bin, args
		return []byte(`{"project":"demo","memories":[]}`), nil, nil
	})
	if _, err := (Source{Runner: runner}).ListCandidateObservations("demo"); err != nil {
		t.Fatalf("ListCandidateObservations: %v", err)
	}
	wantArgs := []string{"list", "--project", "demo",
		"--topic-prefix", "procedural/candidates/", "--topic-prefix", "procedural/drafts/", "--topic-prefix", "goal:", "--json"}
	if gotEnv != nil || gotBin != "memorycard" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command = env %v, %q %q; want inherited env memorycard %q", gotEnv, gotBin, gotArgs, wantArgs)
	}
	if _, err := (Source{Executable: "/opt/mc", Runner: runner}).ListCandidateObservations("demo"); err != nil || gotBin != "/opt/mc" {
		t.Fatalf("Executable not honoured: bin = %q, err = %v", gotBin, err)
	}
}

func TestSourceSortsRowsIntoFamiliesKeepingOrderAndEveryScope(t *testing.T) {
	out := `{"project":"demo","memories":[` +
		`{"id":1,"topic_key":"goal:g-42/procedural/drafts/a/one","scope":"project","title":"t","body":"draft"},` +
		`{"id":2,"topic_key":"goal:g-42/other/thing","scope":"project","title":"t","body":"unrelated"},` +
		`{"id":3,"topic_key":"procedural/candidates/a/one","scope":"personal","title":"t","body":"candidate personal"},` +
		`{"id":4,"topic_key":"procedural/candidates/a/one","scope":"project","title":"t","body":"candidate project"},` +
		`{"id":5,"topic_key":"procedural/candidates/","scope":"project","title":"t","body":"prefix only"}]}`
	got, err := (Source{Runner: answering(out)}).ListCandidateObservations("demo")
	if err != nil {
		t.Fatalf("ListCandidateObservations: %v", err)
	}
	want := skills.CandidateObservations{
		Candidates: []skills.CandidateObservation{
			{TopicKey: "procedural/candidates/a/one", Content: "candidate personal"},
			{TopicKey: "procedural/candidates/a/one", Content: "candidate project"},
			{TopicKey: "procedural/candidates/", Content: "prefix only"},
		},
		Drafts: []skills.CandidateObservation{{TopicKey: "goal:g-42/procedural/drafts/a/one", Content: "draft"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observations = %+v; want %+v", got, want)
	}
}

func TestSourceAnEmptyMemoriesArrayIsAnEmptyProject(t *testing.T) {
	got, err := (Source{Runner: answering(`{"project":"demo","memories":[]}`)}).ListCandidateObservations("demo")
	if err != nil || len(got.Candidates) != 0 || len(got.Drafts) != 0 {
		t.Fatalf("got %+v, %v; want an empty, successful read", got, err)
	}
}

func TestSourceRefusesWhatItCannotTrust(t *testing.T) {
	cases := []struct {
		name string
		run  runnerFunc
		want string
	}{
		{"command failure relays stderr", func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
			return nil, []byte(`unknown command "list"`), errors.New("exit status 2")
		}, `unknown command "list"`},
		{"invalid json", answering("{"), "decode"},
		{"missing memories", answering(`{"project":"demo"}`), "memories"},
		{"null memories", answering(`{"project":"demo","memories":null}`), "memories"},
		{"split part row", answering(`{"memories":[{"id":1,"topic_key":"procedural/candidates/a/one/part-2","scope":"project","body":"x"}]}`), "part"},
		{"split part row under a goal", answering(`{"memories":[{"id":1,"topic_key":"goal:g/procedural/drafts/a/part-10","scope":"project","body":"x"}]}`), "part"},
		{"row without scope", answering(`{"memories":[{"id":1,"topic_key":"procedural/candidates/a/one","body":"x"}]}`), "scope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (Source{Runner: tc.run}).ListCandidateObservations("demo"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want text containing %q", err, tc.want)
			}
		})
	}
}

func TestSourceIgnoresAPartRowOutsideTheTwoFamilies(t *testing.T) {
	out := `{"memories":[{"id":1,"topic_key":"goal:g/notes/x/part-2","scope":"project","body":"x"}]}`
	if got, err := (Source{Runner: answering(out)}).ListCandidateObservations("demo"); err != nil || len(got.Candidates)+len(got.Drafts) != 0 {
		t.Fatalf("got %+v, %v; want a successful empty read", got, err)
	}
}

func TestSourceRejectsMissingConfigurationBeforeStartingCommand(t *testing.T) {
	if _, err := (Source{}).ListCandidateObservations("demo"); err == nil || !strings.Contains(err.Error(), "runner") {
		t.Fatalf("missing runner error = %v; want a runner refusal", err)
	}
	called := runnerFunc(func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
		t.Fatal("runner called for empty project")
		return nil, nil, nil
	})
	if _, err := (Source{Runner: called}).ListCandidateObservations(" "); err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("empty project error = %v; want a project refusal", err)
	}
}
