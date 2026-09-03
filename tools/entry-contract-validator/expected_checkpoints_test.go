package main

import (
	"strings"
	"testing"
)

// The entry contract's `estimate` object is additionalProperties:false, so a
// property it does not declare is not merely ignored — it is REJECTED with exit
// 5. Before this change the object declared no checkpoint field, which meant a
// producer carrying the one number sdd-time-estimation calls "the real calendar
// time driver for agent-orchestrated delivery" had its whole contract refused.
//
// That is not a thin cache; it is a cache that actively refuses the plan side of
// a pair whose actual side (`checkpoint_count`) the actuals record already
// carries. Without both sides recorded, the checkpoint variance the estimator is
// told to report has nowhere to come from.

func TestEstimateAcceptsExpectedCheckpoints(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		estimate := contract["estimate"].(map[string]any)
		estimate["expected_checkpoints"] = 4
	})
	if err != nil {
		t.Fatalf("estimate.expected_checkpoints rejected: %v", err)
	}
}

func TestEstimateExpectedCheckpointsStaysOptional(t *testing.T) {
	// Contracts written before the field existed must keep validating: the
	// archived ones are an immutable historical record and CI validates them on
	// every run.
	err := validateMutation(t, func(contract map[string]any) {
		estimate := contract["estimate"].(map[string]any)
		delete(estimate, "expected_checkpoints")
	})
	if err != nil {
		t.Fatalf("a contract omitting expected_checkpoints must stay valid: %v", err)
	}
}

func TestEstimateRejectsNegativeExpectedCheckpoints(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		estimate := contract["estimate"].(map[string]any)
		estimate["expected_checkpoints"] = -1
	})
	if err == nil {
		t.Fatal("a negative expected_checkpoints was accepted")
	}
}

func TestEstimateRejectsNonIntegerExpectedCheckpoints(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		estimate := contract["estimate"].(map[string]any)
		estimate["expected_checkpoints"] = 2.5
	})
	if err == nil {
		t.Fatal("a fractional expected_checkpoints was accepted — a human round-trip is a countable event")
	}
}

// TestExpectedCheckpointsZeroFailsTheSemanticFloor is the cross-field rule the
// schema cannot express. Zero is a well-formed integer and a false prediction:
// the tiering go-ahead checkpoint is a durable floor that fires on every change,
// interaction_mode auto included — auto suppresses SDD product questions, not
// authorizations. An estimate predicting zero round-trips predicts that a
// checkpoint which always happens will not.
func TestExpectedCheckpointsZeroFailsTheSemanticFloor(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		estimate := contract["estimate"].(map[string]any)
		estimate["expected_checkpoints"] = 0
	})
	if err == nil {
		t.Fatal("expected_checkpoints 0 was accepted")
	}
	if !strings.Contains(err.Error(), "expected_checkpoints") {
		t.Errorf("error %q does not name expected_checkpoints", err)
	}
}
