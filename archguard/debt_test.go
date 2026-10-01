package archguard

import (
	"reflect"
	"strings"
	"testing"
)

func edgesOf(vs []violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.edge())
	}
	return out
}

func TestDebtLinesAreSortedByPackageThenTarget(t *testing.T) {
	debt := Debt{
		"shaper": {"os": "H10", "gitprov": "H4"},
		"roles":  {"syscall": "H7", "os": "H7"},
	}
	want := []debtLine{
		{from: "roles", target: "os", unit: "H7"},
		{from: "roles", target: "syscall", unit: "H7"},
		{from: "shaper", target: "gitprov", unit: "H4"},
		{from: "shaper", target: "os", unit: "H10"},
	}
	if got := debt.lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %v, want %v", got, want)
	}
}

func TestReconcileSplitsViolationsIntoNewAndKnown(t *testing.T) {
	found := []violation{
		{from: "a", target: "os"},
		{from: "a", target: "syscall"},
		{from: "b", target: "a"},
	}
	lines := []debtLine{
		{from: "a", target: "os", unit: "H1"},
		{from: "gone", target: "os", unit: "H2"},
	}
	unlisted, stale := reconcile(found, lines)
	if want := []string{"a -> syscall", "b -> a"}; !reflect.DeepEqual(edgesOf(unlisted), want) {
		t.Errorf("unlisted = %q, want %q", edgesOf(unlisted), want)
	}
	if want := []debtLine{{from: "gone", target: "os", unit: "H2"}}; !reflect.DeepEqual(stale, want) {
		t.Errorf("stale = %v, want %v (a debt whose violation is gone must be deleted)", stale, want)
	}
}

func TestValidateDebtsRejectsUnusableLines(t *testing.T) {
	rings := map[string]Ring{"dom": Domain, "app": Application, "ad": Adapter}
	tests := []struct {
		name string
		line debtLine
		want string
	}{
		{"a unit id is required", debtLine{from: "dom", target: "os"}, "unit"},
		{"a unit id names a work unit", debtLine{from: "dom", target: "os", unit: "later"}, "unit"},
		{"the package must be declared", debtLine{from: "nope", target: "os", unit: "H1"}, "declared"},
		{"only domain and application packages can owe debt", debtLine{from: "ad", target: "os", unit: "H1"}, "domain or application"},
		{"a target is required", debtLine{from: "dom", unit: "H1"}, "target"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := validateDebts([]debtLine{tc.line}, rings)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Errorf("problems = %q, want one mentioning %q", problems, tc.want)
			}
		})
	}

	wellFormed := []debtLine{
		{from: "dom", target: "os", unit: "H12"},
		{from: "dom", target: "syscall", unit: "L3"},
		{from: "app", target: "ad", unit: "T1"},
		{from: "app", target: "time.Now", unit: "B2"},
	}
	if problems := validateDebts(wellFormed, rings); len(problems) != 0 {
		t.Errorf("well-formed debts rejected: %q", problems)
	}
}
