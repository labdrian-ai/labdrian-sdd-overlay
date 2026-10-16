package goal

import (
	"strings"
	"testing"
)

// The canonical bytes of a Goal are the fixture files byte for byte (see
// TestMarshalCanonicalBytesMatchVersionOneFixture), so the digests below were made with sha256sum
// over the fixtures, not with this package.
const (
	versionOneFixtureDigest = "27bbcd46f04e9a1ee3bd7c556ae5d68f002e5aa862ed466c2515c85676da9153"
	versionTwoFixtureDigest = "60d9019df599aadc3bf856d0d8ad42d59a1a9dde378b4329c8956e5f23d41e46"
)

func TestDigestIsTheSHA256OfTheCanonicalBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		goal Goal
		want string
	}{
		{"version 1", sampleGoal(), versionOneFixtureDigest},
		{"version 2", sampleGoalV2(), versionTwoFixtureDigest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.goal.Digest()
			if err != nil {
				t.Fatalf("Digest() = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Digest() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The digest says whether the Goal changed, not whether the file it was read from did: two
// documents that parse to one Goal have one digest, however they were written.
func TestDigestIsAFunctionOfTheGoalNotOfTheBytesItWasParsedFrom(t *testing.T) {
	parsed, err := Parse([]byte(validGoalJSON))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	got, err := parsed.Digest()
	if err != nil {
		t.Fatalf("Digest() = %v, want nil", err)
	}
	if got != versionOneFixtureDigest {
		t.Errorf("Digest() of the compact document = %q, want the digest of the indented fixture %q", got, versionOneFixtureDigest)
	}
}

func TestDigestChangesWithEveryFieldOfTheGoal(t *testing.T) {
	base, err := sampleGoalV2().Digest()
	if err != nil {
		t.Fatalf("Digest() = %v, want nil", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Goal)
	}{
		{"project_id", func(g *Goal) { g.ProjectID += "x" }},
		{"goal_id", func(g *Goal) { g.GoalID += "x" }},
		{"objective", func(g *Goal) { g.Objective += "x" }},
		{"scope", func(g *Goal) { g.Scope += "x" }},
		{"constraints", func(g *Goal) { g.Constraints = append([]string{"x"}, g.Constraints...) }},
		{"non_goals", func(g *Goal) { g.NonGoals = append(g.NonGoals, "x") }},
		{"acceptance_criteria", func(g *Goal) { g.AcceptanceCriteria = append(g.AcceptanceCriteria, "x") }},
		{"memory_scope", func(g *Goal) { g.MemoryScope += "x" }},
		{"runtime_scope", func(g *Goal) { g.RuntimeScope += "x" }},
		{"delivery_boundary", func(g *Goal) { g.DeliveryBoundary += "x" }},
		{"a whitespace-only edit", func(g *Goal) { g.Objective += " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := sampleGoalV2()
			tc.mutate(&g)
			got, err := g.Digest()
			if err != nil {
				t.Fatalf("Digest() = %v, want nil", err)
			}
			if got == base {
				t.Errorf("Digest() after changing %s is still %q", tc.name, got)
			}
		})
	}
}

// A Goal that Marshal refuses has no canonical bytes, so it has no digest, and the error is
// Marshal's own.
func TestDigestRefusesAnInvalidGoal(t *testing.T) {
	g := sampleGoal()
	g.Version = 0
	got, err := g.Digest()
	if err == nil {
		t.Fatalf("Digest() of an invalid goal = (%q, nil), want an error", got)
	}
	if got != "" {
		t.Errorf("Digest() of an invalid goal = %q, want no digest", got)
	}
	if want := "marshal goal: unsupported version 0"; !strings.Contains(err.Error(), want) {
		t.Errorf("Digest() error = %q, want it to contain %q", err.Error(), want)
	}
}
