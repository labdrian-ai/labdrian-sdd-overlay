package memorycardcli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestResolverRunsTheResolveVerbForEngramWithEveryKey(t *testing.T) {
	var gotEnv []string
	var gotBin string
	var gotArgs []string
	runner := runnerFunc(func(_ context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
		gotEnv, gotBin, gotArgs = env, bin, args
		return []byte(`{"source":"engram","ids":[{"key":"100","memory_id":7},{"key":"3","memory_id":null}]}`), nil, nil
	})
	got, err := (Resolver{Runner: runner}).ResolveLegacyOccurrences([]string{"100", "3"})
	if err != nil {
		t.Fatalf("ResolveLegacyOccurrences: %v", err)
	}
	wantArgs := []string{"resolve", "--source", "engram", "--key", "100", "--key", "3", "--json"}
	if gotEnv != nil || gotBin != "memorycard" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command = env %v, %q %q; want inherited env memorycard %q", gotEnv, gotBin, gotArgs, wantArgs)
	}
	if want := map[string]int{"100": 7}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mapped = %v; want %v (a null memory_id is left out)", got, want)
	}
	if _, err := (Resolver{Executable: "/opt/mc", Runner: runner}).ResolveLegacyOccurrences([]string{"100", "3"}); err != nil || gotBin != "/opt/mc" {
		t.Fatalf("Executable not honoured: bin = %q, err = %v", gotBin, err)
	}
}

func TestResolverRefusesWhatItCannotTrust(t *testing.T) {
	cases := []struct {
		name string
		run  runnerFunc
		want string
	}{
		{"command failure relays stderr", func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
			return nil, []byte(`unknown command "resolve"`), errors.New("exit status 2")
		}, `unknown command "resolve"`},
		{"invalid json", answering("{"), "decode"},
		{"missing ids", answering(`{"source":"engram"}`), "ids"},
		{"null ids", answering(`{"source":"engram","ids":null}`), "ids"},
		{"fewer keys than asked", answering(`{"source":"engram","ids":[{"key":"1","memory_id":2}]}`), "keys"},
		{"other key than asked", answering(`{"source":"engram","ids":[{"key":"1","memory_id":2},{"key":"9","memory_id":3}]}`), "keys"},
		{"other order than asked", answering(`{"source":"engram","ids":[{"key":"2","memory_id":3},{"key":"1","memory_id":2}]}`), "keys"},
		{"other source", answering(`{"source":"other","ids":[{"key":"1","memory_id":2},{"key":"2","memory_id":3}]}`), "source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (Resolver{Runner: tc.run}).ResolveLegacyOccurrences([]string{"1", "2"}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want text containing %q", err, tc.want)
			}
		})
	}
}

func TestResolverRejectsMissingConfigurationBeforeStartingCommand(t *testing.T) {
	if _, err := (Resolver{}).ResolveLegacyOccurrences([]string{"1"}); err == nil || !strings.Contains(err.Error(), "runner") {
		t.Fatalf("missing runner error = %v; want a runner refusal", err)
	}
	called := runnerFunc(func(context.Context, []string, string, ...string) ([]byte, []byte, error) {
		t.Fatal("runner called without a key")
		return nil, nil, nil
	})
	if _, err := (Resolver{Runner: called}).ResolveLegacyOccurrences(nil); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("no key error = %v; want a key refusal", err)
	}
}
