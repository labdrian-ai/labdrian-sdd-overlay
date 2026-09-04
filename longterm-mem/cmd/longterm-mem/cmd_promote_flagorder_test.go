package main

import (
	"strings"
	"testing"
)

// TestCmdPromoteReconcile_FlagsAfterTheAddressAreNamedAsFlagOrder: Go's flag
// package stops parsing at the first positional, so
// `promote reconcile c-000701 --project P` leaves THREE positionals -- and
// the bulk-refusal branch then reported an attempted mass adoption ("expected
// exactly one address, got 3: reconcile adopts one page at a time on
// purpose") to an operator who named exactly one address.
//
// That is the wrong diagnosis of the wrong mistake: nothing about flag order
// is mentioned, and the reason given (bulk adoption is refused on purpose) is
// not why the invocation failed. The operator retypes the same command.
func TestCmdPromoteReconcile_FlagsAfterTheAddressAreNamedAsFlagOrder(t *testing.T) {
	const address = "c-000701"
	useVault(t, vaultWithUnrecordedPage(t, address))

	stderr := captureStderr(t, func() {
		if exit := run([]string{"promote", "reconcile", address, "--project", "reconcile-project"}); exit != exitUsage {
			t.Fatalf("flag-after-address reconcile exited %d, want %d (usage)", exit, exitUsage)
		}
	})
	if !strings.Contains(stderr, "--project") || !strings.Contains(stderr, "before") {
		t.Fatalf("flag-order refusal said %q, want it to say the flags must come before the address", stderr)
	}
	if strings.Contains(stderr, "one page at a time") {
		t.Fatalf("flag-order refusal said %q, which misdiagnoses a flag-ordering mistake as an attempted mass adoption", stderr)
	}
}
