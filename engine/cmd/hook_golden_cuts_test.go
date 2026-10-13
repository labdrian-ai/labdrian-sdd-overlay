package main

import (
	"reflect"
	"testing"
)

// A declared cut covers the detail it introduces up to the first "..." and no further: text after that, even
// when it holds another "...", is left as it was written.
func TestCutAt_StopsAtTheFirstCut(t *testing.T) {
	w := &hookWorld{}
	w.cutAt("remove ", "<STATE>/x")

	got, unmatched := w.applyCuts("failed (remove /tmp/very/long/pa...tail...), and then (see /another/pa...) done")

	if want := "failed (remove <STATE>/x...tail...), and then (see /another/pa...) done"; got != want {
		t.Errorf("applyCuts = %q, want %q", got, want)
	}
	if len(unmatched) != 0 {
		t.Errorf("unmatched = %v, want none", unmatched)
	}
}

// A declared cut that matches nothing is reported: the program stopped cutting there (or the lead changed), and
// a mask that matches nothing would otherwise hide that for good.
func TestCutAt_ReportsADeclarationThatMatchesNothing(t *testing.T) {
	w := &hookWorld{}
	w.cutAt("remove ", "<STATE>/x")
	w.cutAt("never said ", "<Y>")

	_, unmatched := w.applyCuts("failed (remove /tmp/pa...)")

	if want := []string{"never said "}; !reflect.DeepEqual(unmatched, want) {
		t.Errorf("unmatched = %v, want %v", unmatched, want)
	}
}
