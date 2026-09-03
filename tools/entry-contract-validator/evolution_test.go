package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realEntryContract names a real entry contract in the repository tree. These
// files are read-only inputs: archived contracts are an immutable historical
// record and must never be edited to satisfy the validator.
func realEntryContract(t *testing.T, elements ...string) string {
	t.Helper()
	return filepath.Join(append([]string{repoRoot(t), "openspec", "changes"}, elements...)...)
}

// runValidator runs the CLI entry point and returns its exit code plus the
// combined output, so exit-code contracts are asserted the way callers and CI
// observe them.
func runValidator(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

func decodeJSONInto(t *testing.T, filename string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode %s: %v", filename, err)
	}
	return document
}

func writeJSONTemp(t *testing.T, dir, name string, document any) string {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	target := filepath.Join(dir, name)
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return target
}

// TestValidatorAcceptsArchivedDeliveryTimeSizeException is the acceptance test
// for the delivery-time size-exception vocabulary. The archived longterm-mem
// contract records a real overrun that the planned line counts could not
// predict, and it is an immutable record: the validator must be taught the
// vocabulary rather than the file rewritten.
func TestValidatorAcceptsArchivedDeliveryTimeSizeException(t *testing.T) {
	instance := realEntryContract(t, "archive", "2026-09-02-longterm-mem", "entry.json")
	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance)
	if code != exitOK {
		t.Fatalf("archived longterm-mem contract exit code = %d, want %d; output=%q", code, exitOK, output)
	}
}

// TestValidatorReportsLegacyContractWithoutContractVersion pins the
// machine-readable distinction between a pre-v2 legacy contract (no
// contract_version at all) and a corrupt one. A legacy file must be reported as
// not-validated, not buried under a wall of schema-shape errors.
func TestValidatorReportsLegacyContractWithoutContractVersion(t *testing.T) {
	instance := realEntryContract(t, "archive", "2026-07-21-restore-skill-registry-scoped-blocks", "entry.json")
	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance)
	if code != exitLegacyContract {
		t.Fatalf("pre-v2 contract exit code = %d, want %d; output=%q", code, exitLegacyContract, output)
	}
	if !strings.Contains(output, "contract_version") {
		t.Errorf("legacy report %q does not name the missing contract_version field", output)
	}
}

// TestUnrecognisedContractVersionIsNotLegacy proves the legacy exit code is
// reserved for the absent-field case. A contract that declares a version the
// bundle does not know is a hard failure, never a skip.
func TestUnrecognisedContractVersionIsNotLegacy(t *testing.T) {
	contract := loadValidContract(t)
	contract["contract_version"] = "9.9.9"
	instance := writeJSONTemp(t, t.TempDir(), "entry.json", contract)

	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance)
	if code == exitOK || code == exitLegacyContract {
		t.Fatalf("unrecognised contract_version exit code = %d, want a hard failure; output=%q", code, output)
	}
}

// TestUsageDocumentsLegacyExitCode keeps the CLI's own exit-code table honest.
func TestUsageDocumentsLegacyExitCode(t *testing.T) {
	code, output := runValidator(t, "--help")
	if code != exitOK {
		t.Fatalf("help exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(output, "7  ") {
		t.Errorf("usage text does not document exit code 7:\n%s", output)
	}
}

// TestValidatorAcceptsCurrentBundleContractVersion proves the bundle version is
// no longer an exact-match lock that a version bump would break.
func TestValidatorAcceptsCurrentBundleContractVersion(t *testing.T) {
	contract := loadValidContract(t)
	contract["contract_version"] = expectedContractVersion
	instance := writeJSONTemp(t, t.TempDir(), "entry.json", contract)

	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance)
	if code != exitOK {
		t.Fatalf("contract_version %s exit code = %d, want %d; output=%q", expectedContractVersion, code, exitOK, output)
	}
}

// TestValidatorStillAcceptsSupersededContractVersions is the backwards
// compatibility pin: bumping the bundle version must never invalidate a
// contract produced by a previous supported bundle.
func TestValidatorStillAcceptsSupersededContractVersions(t *testing.T) {
	instances := []string{
		fixturePath(t, "valid-entry-contract.json"),
		realEntryContract(t, "overlay-versioned-releases", "entry.json"),
		realEntryContract(t, "archive", "2026-08-05-skills-validate-ondisk-gate", "entry.json"),
	}
	for _, instance := range instances {
		t.Run(filepath.Base(filepath.Dir(instance)), func(t *testing.T) {
			declared := decodeJSONInto(t, instance)["contract_version"]
			if declared != "2.0.0" {
				t.Fatalf("fixture %s no longer declares the superseded version 2.0.0 (got %v); this pin must keep testing a superseded version", instance, declared)
			}
			code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance)
			if code != exitOK {
				t.Fatalf("superseded 2.0.0 contract %s exit code = %d, want %d; output=%q", instance, code, exitOK, output)
			}
		})
	}
}

// TestSchemaMustAcceptTheValidatorsCurrentVersion proves the schema/validator
// compatibility handshake is a set-membership check, not const equality: a
// schema whose accepted-version set omits the validator's current version is a
// bundle mismatch and must be rejected before any instance is read.
func TestSchemaMustAcceptTheValidatorsCurrentVersion(t *testing.T) {
	schema := decodeJSONInto(t, schemaPath(t))
	properties := schema["properties"].(map[string]any)
	properties["contract_version"] = map[string]any{
		"enum":        []any{"1.0.0"},
		"description": "deliberately omits the validator's current version",
	}
	staleSchema := writeJSONTemp(t, t.TempDir(), "stale-schema.json", schema)

	code, output := runValidator(t, "--schema", staleSchema, "--instance", fixturePath(t, "valid-entry-contract.json"))
	if code != exitSchema {
		t.Fatalf("stale schema exit code = %d, want %d; output=%q", code, exitSchema, output)
	}
	if !strings.Contains(output, expectedContractVersion) {
		t.Errorf("bundle-mismatch report %q does not name the validator version %s", output, expectedContractVersion)
	}
}

// TestSemanticLayerRejectsUnsupportedVersionNamingTheSupportedSet proves the
// semantic layer independently enforces the supported set even when the schema
// it was handed is more permissive, and that its message tells the reader which
// versions this bundle actually supports.
func TestSemanticLayerRejectsUnsupportedVersionNamingTheSupportedSet(t *testing.T) {
	dir := t.TempDir()
	schema := decodeJSONInto(t, schemaPath(t))
	properties := schema["properties"].(map[string]any)
	versionProperty := properties["contract_version"].(map[string]any)
	versionProperty["enum"] = []any{"2.0.0", expectedContractVersion, "9.9.9"}
	delete(versionProperty, "const")
	permissiveSchema := writeJSONTemp(t, dir, "permissive-schema.json", schema)

	contract := loadValidContract(t)
	contract["contract_version"] = "9.9.9"
	instance := writeJSONTemp(t, dir, "entry.json", contract)

	code, output := runValidator(t, "--schema", permissiveSchema, "--instance", instance)
	if code != exitSemanticValidation {
		t.Fatalf("unsupported version exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
	for _, want := range []string{"2.0.0", expectedContractVersion} {
		if !strings.Contains(output, want) {
			t.Errorf("message %q does not name supported version %s", output, want)
		}
	}
}

func setSizeException(contract map[string]any, exception map[string]any) {
	budget := contract["review_budget"].(map[string]any)
	budget["size_exception"] = exception
}

func makeFirstSliceOverBudget(contract map[string]any) {
	slices := reviewSlices(contract)
	lines := slices[0].(map[string]any)["estimated_changed_lines"].(map[string]any)
	lines["high"] = json.Number("401")
}

// TestDeliveryTimeSizeExceptionIsLegalWithinPlannedBudget is the core of the
// delivery-time vocabulary: a realized overrun is a fact the planner could not
// predict, so it is legal precisely when every planned slice was within budget.
func TestDeliveryTimeSizeExceptionIsLegalWithinPlannedBudget(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		setSizeException(contract, map[string]any{
			"state":         deliveryTimeSizeExceptionState,
			"scope":         "openspec/changes/example/verify-report.md",
			"changed_lines": json.Number("547"),
			"authorized_by": "maintainer",
			"reason":        "The verification report is a single indivisible artifact.",
		})
	})
	if err != nil {
		t.Fatalf("delivery-time size exception rejected within planned budget: %v", err)
	}
}

// TestDeliveryTimeSizeExceptionIsLegalOverPlannedBudget proves a slice planned
// over budget that also overran at delivery is still honestly recordable.
func TestDeliveryTimeSizeExceptionIsLegalOverPlannedBudget(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		makeFirstSliceOverBudget(contract)
		setSizeException(contract, map[string]any{
			"state":         deliveryTimeSizeExceptionState,
			"scope":         "openspec/changes/example/verify-report.md",
			"changed_lines": json.Number("547"),
			"authorized_by": "maintainer",
			"reason":        "Planned over budget and overran at delivery.",
		})
	})
	if err != nil {
		t.Fatalf("delivery-time size exception rejected over planned budget: %v", err)
	}
}

// TestDeliveryTimeSizeExceptionRequiresItsCompanionFields keeps the new state
// from becoming an unaccountable escape hatch.
func TestDeliveryTimeSizeExceptionRequiresItsCompanionFields(t *testing.T) {
	complete := map[string]any{
		"state":         deliveryTimeSizeExceptionState,
		"scope":         "openspec/changes/example/verify-report.md",
		"changed_lines": json.Number("547"),
		"authorized_by": "maintainer",
		"reason":        "The verification report is a single indivisible artifact.",
	}
	for _, missing := range []string{"scope", "changed_lines", "authorized_by", "reason"} {
		t.Run("missing "+missing, func(t *testing.T) {
			err := validateMutation(t, func(contract map[string]any) {
				exception := map[string]any{}
				for key, value := range complete {
					if key != missing {
						exception[key] = value
					}
				}
				setSizeException(contract, exception)
			})
			if err == nil {
				t.Fatalf("delivery-time size exception accepted without %s", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("error %q does not name the missing field %q", err, missing)
			}
		})
	}
}

// TestDeliveryTimeSizeExceptionUsesExactlyOneToken pins the delivery-time
// state to a single spelling. An alias would have to be accepted forever,
// because the archived contract that carries it is immutable and CI validates
// it on every run; a synonym that can never be removed is not a deprecation,
// it is a second vocabulary, and the reader has no way to tell which spelling
// is real.
func TestDeliveryTimeSizeExceptionUsesExactlyOneToken(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		setSizeException(contract, map[string]any{
			"state":         "granted-at-delivery",
			"scope":         "openspec/changes/example/verify-report.md",
			"changed_lines": json.Number("547"),
			"authorized_by": "maintainer",
			"reason":        "A second spelling of the delivery-time state.",
		})
	})
	if err == nil {
		t.Fatalf("size_exception accepted a second spelling of %q", deliveryTimeSizeExceptionState)
	}
	if !strings.Contains(err.Error(), deliveryTimeSizeExceptionState) {
		t.Fatalf("error %q does not name the one canonical delivery-time token %q", err, deliveryTimeSizeExceptionState)
	}
}

// TestSizeExceptionRejectsUnknownFields proves relaxing the object for the
// delivery-time companions did not make it open-ended.
func TestSizeExceptionRejectsUnknownFields(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		setSizeException(contract, map[string]any{
			"state":            "not-needed",
			"unknown_property": "smuggled",
		})
	})
	if err == nil {
		t.Fatal("size_exception accepted an unknown property")
	}
	if !strings.Contains(err.Error(), "unknown_property") {
		t.Fatalf("error %q does not name the rejected property", err)
	}
}

// TestApprovedSizeExceptionRequiresReasonAndApprover pins accountability on the
// planned-exception path: an approval with no approver and no reason records
// nothing a reviewer can act on.
func TestApprovedSizeExceptionRequiresReasonAndApprover(t *testing.T) {
	for _, tt := range []struct {
		name      string
		exception map[string]any
		wantField string
	}{
		{
			name:      "no approver and no reason",
			exception: map[string]any{"state": "approved"},
			wantField: "approved_by",
		},
		{
			name:      "reason without approver",
			exception: map[string]any{"state": "approved", "reason": "one indivisible artifact"},
			wantField: "approved_by",
		},
		{
			name:      "approver without reason",
			exception: map[string]any{"state": "approved", "approved_by": "maintainer"},
			wantField: "reason",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMutation(t, func(contract map[string]any) {
				makeFirstSliceOverBudget(contract)
				setSizeException(contract, tt.exception)
			})
			if err == nil {
				t.Fatal("unaccountable approved size exception accepted")
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Fatalf("error %q does not name the required field %q", err, tt.wantField)
			}
		})
	}
}

// TestApprovedSizeExceptionWithReasonAndApproverIsAccepted keeps the
// accountability rule from rejecting a properly recorded approval.
func TestApprovedSizeExceptionWithReasonAndApproverIsAccepted(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		makeFirstSliceOverBudget(contract)
		setSizeException(contract, map[string]any{
			"state":       "approved",
			"reason":      "One indivisible artifact cannot be split across slices.",
			"approved_by": "maintainer",
		})
	})
	if err != nil {
		t.Fatalf("accountable approved size exception rejected: %v", err)
	}
}

// TestChainStrategyAcceptsStackedToMain aligns the schema token with its only
// consumers: the SDD phase skills branch on stacked-to-main.
func TestChainStrategyAcceptsStackedToMain(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		contract["chain_strategy"] = "stacked-to-main"
	})
	if err != nil {
		t.Fatalf("chain_strategy stacked-to-main rejected: %v", err)
	}
}

// TestChainStrategyRejectsStackedPrChain proves the orphan token is gone: a
// contract carrying it would fall through every consumer branch unguarded.
func TestChainStrategyRejectsStackedPrChain(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		contract["chain_strategy"] = "stacked-pr-chain"
	})
	if err == nil {
		t.Fatal("chain_strategy stacked-pr-chain accepted, but no consumer implements it")
	}
	if !strings.Contains(err.Error(), "chain_strategy") {
		t.Fatalf("error %q does not name chain_strategy", err)
	}
}

// TestChainStrategyRejectsUnknownValue is the general out-of-enum guard the
// token domain previously lacked.
func TestChainStrategyRejectsUnknownValue(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		contract["chain_strategy"] = "invented-topology"
	})
	if err == nil {
		t.Fatal("unknown chain_strategy accepted")
	}
}

// declaredOpenSpecPaths collects every artifact_refs openspec_path declared by
// a contract, at any nesting depth.
func declaredOpenSpecPaths(refs any) []string {
	node, ok := refs.(map[string]any)
	if !ok {
		return nil
	}
	var paths []string
	if value, ok := node["openspec_path"].(string); ok && value != "" {
		paths = append(paths, value)
	}
	for key, child := range node {
		if key == "openspec_path" || key == "topic_key" {
			continue
		}
		paths = append(paths, declaredOpenSpecPaths(child)...)
	}
	return paths
}

func materializeDeclaredPaths(t *testing.T, root string, contract map[string]any) []string {
	t.Helper()
	paths := declaredOpenSpecPaths(contract["artifact_refs"])
	if len(paths) == 0 {
		t.Fatal("fixture declares no openspec_path values; this test would prove nothing")
	}
	for _, relative := range paths {
		target := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("create %s: %v", target, err)
		}
	}
	return paths
}

// TestExistsRootAcceptsPresentArtifactPaths covers the opt-in inception-time
// check against a live change directory where the declared artifacts exist.
func TestExistsRootAcceptsPresentArtifactPaths(t *testing.T) {
	root := t.TempDir()
	contract := loadValidContract(t)
	materializeDeclaredPaths(t, root, contract)
	instance := writeJSONTemp(t, t.TempDir(), "entry.json", contract)

	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance, "--exists-root", root)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
}

// TestExistsRootRejectsMissingArtifactPath proves the opt-in check actually
// stats, and names the path that is missing.
func TestExistsRootRejectsMissingArtifactPath(t *testing.T) {
	root := t.TempDir()
	contract := loadValidContract(t)
	paths := materializeDeclaredPaths(t, root, contract)
	removed := filepath.Join(root, filepath.FromSlash(paths[0]))
	if err := os.Remove(removed); err != nil {
		t.Fatalf("remove %s: %v", removed, err)
	}
	instance := writeJSONTemp(t, t.TempDir(), "entry.json", contract)

	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance, "--exists-root", root)
	if code != exitSemanticValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
	if !strings.Contains(output, paths[0]) {
		t.Errorf("failure %q does not name the missing path %q", output, paths[0])
	}
}

// TestOverlayCLIResolvesExistsRootFromCaller proves the overlay wrapper
// normalizes --exists-root the same way it normalizes --schema and --instance.
// The validator does not run in the caller's working directory, so a relative
// root that is passed through unnormalized would silently resolve against the
// overlay root and stat the wrong tree.
func TestOverlayCLIResolvesExistsRootFromCaller(t *testing.T) {
	root := repoRoot(t)
	callerDir := t.TempDir()
	contract := loadValidContract(t)
	materializeDeclaredPaths(t, filepath.Join(callerDir, "live-change"), contract)
	writeJSONTemp(t, callerDir, "entry.json", contract)

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "bin", "labdrian-overlay"),
		"validate-entry-contract",
		"--schema", filepath.Join(root, "skills", "_shared", "entry-contract.schema.json"),
		"--instance", "entry.json",
		"--exists-root", "live-change",
	)
	cmd.Dir = callerDir
	cmd.Env = append(os.Environ(), "OVERLAY_DIR="+root, "TMPDIR="+t.TempDir())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wrapper did not resolve --exists-root from the caller's directory: %v\n%s", err, output)
	}
}

// TestExistsRootDefaultsOff is the load-bearing default: the shipped fixtures
// and every archived contract declare paths that no longer exist on disk, so
// enabling the check by default would break CI and invalidate history.
func TestExistsRootDefaultsOff(t *testing.T) {
	instances := []string{
		fixturePath(t, "valid-entry-contract.json"),
		realEntryContract(t, "archive", "2026-09-02-longterm-mem", "entry.json"),
	}
	for _, instance := range instances {
		t.Run(filepath.Base(filepath.Dir(instance)), func(t *testing.T) {
			var missing []string
			for _, relative := range declaredOpenSpecPaths(decodeJSONInto(t, instance)["artifact_refs"]) {
				if _, err := os.Stat(filepath.Join(repoRoot(t), filepath.FromSlash(relative))); err != nil {
					missing = append(missing, relative)
				}
			}
			if len(missing) == 0 {
				t.Fatal("every declared path still exists on disk; this pin needs a contract whose declared paths are gone")
			}
			code, output := runValidator(t, "--schema", schemaPath(t), "--instance", instance)
			if code != exitOK {
				t.Fatalf("exit code = %d, want %d without --exists-root; output=%q", code, exitOK, output)
			}
		})
	}
}
