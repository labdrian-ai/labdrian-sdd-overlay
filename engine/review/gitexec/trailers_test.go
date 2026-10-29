package gitexec_test

// What the engine asks git to read the Risk trailer (own-rdd T6b1, S6 a): the raw message of a commit and the
// trailers git parses from a message. Both go through the real git on repositories made in temporary directories.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

func TestMessageIsTheRawMessageOfTheCommit(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "a\n")
	f.stage()
	head := f.snapshot("feat: x\n\nbody line\n\nRisk: none")

	got, err := f.adapter().Message(f.repo(), head)

	want := "feat: x\n\nbody line\n\nRisk: none\n"
	if err != nil || got != want {
		t.Errorf("Message = %q, %v, want %q, nil", got, err, want)
	}
}

func TestMessageRefusesACommitGitDoesNotHave(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "a\n")
	f.stage()
	f.snapshot("feat: x")

	if _, err := f.adapter().Message(f.repo(), strings.Repeat("a", 40)); err == nil {
		t.Error("Message of an unknown commit gave no error")
	}
}

func TestTrailersAreTheOnesGitParsesFromTheMessage(t *testing.T) {
	f := newFixture(t, "sha1")
	for _, tc := range []struct {
		name, message string
		want          []review.Trailer
	}{
		{"one", "feat: x\n\nbody\n\nRisk: none\n", []review.Trailer{{Key: "Risk", Value: "none"}}},
		{"several, in order", "feat: x\n\nSigned-off-by: A\nrisk: 3,5\n", []review.Trailer{{Key: "Signed-off-by", Value: "A"}, {Key: "risk", Value: "3,5"}}},
		{"a wrapped one comes back unfolded", "feat: x\n\nRisk: 3,\n 5\n", []review.Trailer{{Key: "Risk", Value: "3, 5"}}},
		{"none in a message without a final block", "feat: x\n\nRisk: none\n\nmore body\n", nil},
		{"none in a message of one line", "feat: x\n", nil},
	} {
		got, err := f.adapter().Trailers(f.repo(), tc.message)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: Trailers = %#v, %v, want %#v, nil", tc.name, got, err, tc.want)
		}
	}
}
