package engramexport

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

type runnerFunc func(context.Context, []string, string, ...string) ([]byte, []byte, error)

func (f runnerFunc) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	return f(ctx, env, bin, args...)
}

func TestSourceExportsProjectAndFiltersBothTopicFamilies(t *testing.T) {
	temp := t.TempDir()
	runner := runnerFunc(func(_ context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
		if env != nil || bin != "engram" || len(args) != 4 || args[0] != "export" || args[2] != "--project" || args[3] != "demo" {
			t.Fatalf("command = env %v, %q %q; want inherited env engram export <file> --project demo", env, bin, args)
		}
		path := args[1]
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("export target existed before invocation: stat %q = %v", path, err)
		}
		data := `{"observations":[` +
			`{"topic_key":"procedural/candidates/repeated-success/one","content":"candidate"},` +
			`{"topic_key":"goal:g-42/procedural/drafts/repeated-success/one","content":"draft"},` +
			`{"topic_key":"project/other","content":"unrelated"},` +
			`{"topic_key":"procedural/candidates/","content":"prefix only"}]}`
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		return nil, nil, nil
	})
	source := Source{Executable: "engram", TempDir: temp, Runner: runner}
	got, err := source.ListCandidateObservations("demo")
	if err != nil {
		t.Fatalf("ListCandidateObservations: %v", err)
	}
	want := skills.CandidateObservations{
		Candidates: []skills.CandidateObservation{
			{TopicKey: "procedural/candidates/repeated-success/one", Content: "candidate"},
			{TopicKey: "procedural/candidates/", Content: "prefix only"},
		},
		Drafts: []skills.CandidateObservation{{TopicKey: "goal:g-42/procedural/drafts/repeated-success/one", Content: "draft"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observations = %+v; want %+v", got, want)
	}
	entries, err := os.ReadDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("temporary export was not removed: %v", entries)
	}
}

func TestSourceRefusesUnreadableOrMalformedExports(t *testing.T) {
	cases := []struct {
		name string
		run  runnerFunc
		want string
	}{
		{"command failure", func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
			return []byte("partial"), []byte("database unavailable"), errors.New("exit status 1")
		}, "database unavailable"},
		{"file absent", func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		}, "read export"},
		{"invalid json", func(_ context.Context, _ []string, _ string, args ...string) ([]byte, []byte, error) {
			return nil, nil, os.WriteFile(args[1], []byte("{"), 0o600)
		}, "decode export"},
		{"missing observations", func(_ context.Context, _ []string, _ string, args ...string) ([]byte, []byte, error) {
			return nil, nil, os.WriteFile(args[1], []byte(`{"other":[]}`), 0o600)
		}, "observations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := Source{Executable: "engram", TempDir: t.TempDir(), Runner: tc.run}
			if _, err := source.ListCandidateObservations("demo"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ListCandidateObservations error = %v; want text containing %q", err, tc.want)
			}
		})
	}
}

func TestSourceRejectsMissingConfigurationBeforeStartingCommand(t *testing.T) {
	if _, err := (Source{}).ListCandidateObservations("demo"); err == nil || !strings.Contains(err.Error(), "runner") {
		t.Fatalf("missing runner error = %v; want a runner refusal", err)
	}
	if _, err := (Source{Runner: runnerFunc(func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
		t.Fatal("runner called for empty project")
		return nil, nil, nil
	})}).ListCandidateObservations(""); err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("empty project error = %v; want a project refusal", err)
	}
}
