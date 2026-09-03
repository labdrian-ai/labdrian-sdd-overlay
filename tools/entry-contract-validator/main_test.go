package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// expectedContractVersion is the version the current bundle produces. Contracts
// written by an earlier supported bundle stay valid; see
// expectedSupersededContractVersions.
const expectedContractVersion = "2.1.0"

// expectedSupersededContractVersions are the earlier bundle versions this
// bundle still accepts. The schema's accepted-version set is the union.
var expectedSupersededContractVersions = []string{"2.0.0"}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func schemaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "skills", "_shared", "entry-contract.schema.json")
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func loadValidContract(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, "valid-entry-contract.json"))
	if err != nil {
		t.Fatalf("read valid fixture: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var contract map[string]any
	if err := decoder.Decode(&contract); err != nil {
		t.Fatalf("decode valid fixture: %v", err)
	}
	return contract
}

func validateMutation(t *testing.T, mutate func(map[string]any)) error {
	t.Helper()
	contract := loadValidContract(t)
	mutate(contract)
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal mutated contract: %v", err)
	}
	instancePath := filepath.Join(t.TempDir(), "entry.json")
	if err := os.WriteFile(instancePath, data, 0o644); err != nil {
		t.Fatalf("write mutated contract: %v", err)
	}
	return validateFiles(schemaPath(t), instancePath, validationOptions{})
}

func artifactReferenceMap(t *testing.T, contract map[string]any, name string) map[string]any {
	t.Helper()
	refs := contract["artifact_refs"].(map[string]any)
	return refs[name].(map[string]any)
}

func reviewSlices(contract map[string]any) []any {
	return contract["review_slices"].([]any)
}

func TestValidateFilesAcceptsV2Fixture(t *testing.T) {
	if err := validateFiles(schemaPath(t), fixturePath(t, "valid-entry-contract.json"), validationOptions{}); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
}

func TestValidateFilesAcceptsNativeDispatcherContractValues(t *testing.T) {
	for _, token := range []string{
		"sdd-new",
		"select-change",
		"propose",
		"spec",
		"design",
		"tasks",
		"apply",
		"verify",
		"remediate",
		"review",
		"resolve-review",
		"archive",
	} {
		t.Run("next recommendation "+token, func(t *testing.T) {
			if err := validateMutation(t, func(contract map[string]any) {
				contract["expected_native_next_recommendation"] = token
			}); err != nil {
				t.Fatalf("native dispatcher token %q rejected: %v", token, err)
			}
		})
	}

	t.Run("critical complexity", func(t *testing.T) {
		if err := validateMutation(t, func(contract map[string]any) {
			estimate := contract["estimate"].(map[string]any)
			estimate["complexity"] = "critical"
		}); err != nil {
			t.Fatalf("normalized critical complexity rejected: %v", err)
		}
	})
}

func TestValidateFilesRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantDetail string
	}{
		{
			name: "unknown property",
			mutate: func(contract map[string]any) {
				contract["unknown_property"] = true
			},
		},
		{
			name: "mismatched contract version",
			mutate: func(contract map[string]any) {
				contract["contract_version"] = "1.0.0"
			},
		},
		{
			name: "stale prefixed dispatcher phase",
			mutate: func(contract map[string]any) {
				contract["expected_native_next_recommendation"] = "sdd-propose"
			},
			wantDetail: "may be legitimate and newer than the overlay's mirror",
		},
		{
			name: "bad estimate range",
			mutate: func(contract map[string]any) {
				estimate := contract["estimate"].(map[string]any)
				hours := estimate["planned_range_hours"].(map[string]any)
				hours["low"] = json.Number("41")
				hours["high"] = json.Number("40")
			},
			wantDetail: "planned_range_hours.low",
		},
		{
			name: "duplicate slice order",
			mutate: func(contract map[string]any) {
				slices := reviewSlices(contract)
				slices[1].(map[string]any)["order"] = json.Number("1")
			},
			wantDetail: "order",
		},
		{
			name: "out of order slices",
			mutate: func(contract map[string]any) {
				slices := reviewSlices(contract)
				slices[0], slices[1] = slices[1], slices[0]
			},
			wantDetail: "ordered",
		},
		{
			name: "duplicate slice id",
			mutate: func(contract map[string]any) {
				slices := reviewSlices(contract)
				slices[1].(map[string]any)["id"] = slices[0].(map[string]any)["id"]
			},
			wantDetail: "duplicate",
		},
		{
			name: "forward dependency",
			mutate: func(contract map[string]any) {
				slices := reviewSlices(contract)
				slices[1].(map[string]any)["dependencies"] = []any{"release-readiness"}
			},
			wantDetail: "prior slice",
		},
		{
			name: "inconsistent chain strategy",
			mutate: func(contract map[string]any) {
				contract["delivery_strategy"] = "single-pr"
				contract["chaining_required"] = false
			},
			wantDetail: "force-chained",
		},
		{
			name: "missing chain topology",
			mutate: func(contract map[string]any) {
				contract["chain_strategy"] = "none"
			},
			wantDetail: "chain_strategy",
		},
		{
			name: "missing strict TDD broad commands",
			mutate: func(contract map[string]any) {
				strictTDD := contract["strict_tdd"].(map[string]any)
				commands := strictTDD["test_commands"].(map[string]any)
				delete(commands, "broad")
			},
		},
		{
			name: "size exception inconsistency",
			mutate: func(contract map[string]any) {
				slices := reviewSlices(contract)
				lines := slices[0].(map[string]any)["estimated_changed_lines"].(map[string]any)
				lines["high"] = json.Number("401")
			},
			wantDetail: "size exception",
		},
		{
			name: "unstable topic reference",
			mutate: func(contract map[string]any) {
				artifactReferenceMap(t, contract, "requirements")["topic_key"] = "project/other/requirements/wrong-change"
			},
			wantDetail: "requirements topic_key",
		},
		{
			name: "unsafe hybrid path",
			mutate: func(contract map[string]any) {
				artifactReferenceMap(t, contract, "requirements")["openspec_path"] = "openspec/../outside.json"
			},
			wantDetail: "normalized relative path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMutation(t, tt.mutate)
			if err == nil {
				t.Fatal("invalid contract accepted")
			}
			if tt.wantDetail != "" && !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("error %q does not contain %q", err, tt.wantDetail)
			}
		})
	}
}

// TestSelfDiagnosingMessageForUnknownDispatcherToken proves that a rejected
// expected_native_next_recommendation token produces a message that blames
// the likeliest real cause (a stale overlay mirror of the native
// dispatcher's token domain) rather than a generic schema-validation
// failure. The exit code must stay exitSchemaValidation: this is a message
// change, not a contract change.
func TestSelfDiagnosingMessageForUnknownDispatcherToken(t *testing.T) {
	const rejectedToken = "sdd-propose"
	err := validateMutation(t, func(contract map[string]any) {
		contract["expected_native_next_recommendation"] = rejectedToken
	})
	if err == nil {
		t.Fatal("invalid contract accepted")
	}
	classified, ok := err.(*contractError)
	if !ok {
		t.Fatalf("error %v is not a *contractError", err)
	}
	if classified.code != exitSchemaValidation {
		t.Fatalf("exit code = %d, want %d (message-only change must keep the exit code)", classified.code, exitSchemaValidation)
	}

	message := err.Error()
	for _, want := range []string{
		rejectedToken,
		"may be legitimate and newer than the overlay's mirror",
		"skills/_shared/entry-contract.schema.json",
		"skills/inception-pipeline/SKILL.md",
		"gentle-ai sdd-status",
		"nextRecommended",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("self-diagnosing message %q does not contain %q", message, want)
		}
	}
}

// TestSelfDiagnosingMessageSurfacesSiblingViolations proves that when an
// instance carries the unknown-dispatcher-token violation AND an unrelated
// violation at the same time (the root schema's additionalProperties: false
// rejecting an unknown top-level property), the self-diagnosing message
// still surfaces the second violation instead of silently discarding it.
func TestSelfDiagnosingMessageSurfacesSiblingViolations(t *testing.T) {
	const rejectedToken = "sdd-propose"
	const unknownProperty = "unknown_property"
	err := validateMutation(t, func(contract map[string]any) {
		contract["expected_native_next_recommendation"] = rejectedToken
		contract[unknownProperty] = true
	})
	if err == nil {
		t.Fatal("invalid contract accepted")
	}
	classified, ok := err.(*contractError)
	if !ok {
		t.Fatalf("error %v is not a *contractError", err)
	}
	if classified.code != exitSchemaValidation {
		t.Fatalf("exit code = %d, want %d", classified.code, exitSchemaValidation)
	}

	message := err.Error()
	if !strings.Contains(message, "may be legitimate and newer than the overlay's mirror") {
		t.Errorf("message %q does not contain the self-diagnosing sentence", message)
	}
	if !strings.Contains(message, unknownProperty) {
		t.Errorf("message %q does not surface the sibling additionalProperties violation naming %q", message, unknownProperty)
	}
}

// TestSelfDiagnosingMessageWrapsUnderlyingValidationError proves that the
// self-diagnosing message still wraps the original *jsonschema.ValidationError
// with %w rather than stringifying it away, so callers using errors.As can
// still reach the full validation detail.
func TestSelfDiagnosingMessageWrapsUnderlyingValidationError(t *testing.T) {
	err := validateMutation(t, func(contract map[string]any) {
		contract["expected_native_next_recommendation"] = "sdd-propose"
	})
	if err == nil {
		t.Fatal("invalid contract accepted")
	}
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("errors.As could not reach *jsonschema.ValidationError from %v", err)
	}
}

func TestRunExitCodesAndHelp(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--help"}, &stdout, &stderr); code != exitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitOK, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Exit codes:") {
			t.Fatalf("help output does not document exit codes: %q", stdout.String())
		}
	})

	t.Run("version", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--version"}, &stdout, &stderr); code != exitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitOK, stderr.String())
		}
		if !strings.Contains(stdout.String(), expectedContractVersion) {
			t.Fatalf("version output %q does not contain %q", stdout.String(), expectedContractVersion)
		}
	})

	t.Run("valid", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--schema", schemaPath(t), "--instance", fixturePath(t, "valid-entry-contract.json")}, &stdout, &stderr)
		if code != exitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitOK, stderr.String())
		}
	})

	t.Run("usage", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(nil, &stdout, &stderr); code != exitUsage {
			t.Fatalf("exit code = %d, want %d", code, exitUsage)
		}
	})

	t.Run("schema unavailable", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--schema", filepath.Join(t.TempDir(), "missing.json"), "--instance", fixturePath(t, "valid-entry-contract.json")}, &stdout, &stderr)
		if code != exitSchema {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitSchema, stderr.String())
		}
	})

	t.Run("instance unavailable", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--schema", schemaPath(t), "--instance", filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr)
		if code != exitInstance {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitInstance, stderr.String())
		}
	})

	t.Run("schema validation", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--schema", schemaPath(t), "--instance", fixturePath(t, "invalid-entry-contract.json")}, &stdout, &stderr)
		if code != exitSchemaValidation {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitSchemaValidation, stderr.String())
		}
	})

	t.Run("semantic validation", func(t *testing.T) {
		contract := loadValidContract(t)
		estimate := contract["estimate"].(map[string]any)
		hours := estimate["planned_range_hours"].(map[string]any)
		hours["low"] = json.Number("50")
		data, err := json.Marshal(contract)
		if err != nil {
			t.Fatal(err)
		}
		instancePath := filepath.Join(t.TempDir(), "bad-range.json")
		if err := os.WriteFile(instancePath, data, 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		code := run([]string{"--schema", schemaPath(t), "--instance", instancePath}, &stdout, &stderr)
		if code != exitSemanticValidation {
			t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitSemanticValidation, stderr.String())
		}
	})
}

func TestBundleAssetsAreTrackedAndVersionMatched(t *testing.T) {
	root := repoRoot(t)
	manifest, err := os.ReadFile(filepath.Join(root, "overlay.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{
		"inception-pipeline/SKILL.md custom",
		"_shared/pre-sdd-contracts.md custom",
		"_shared/entry-contract.schema.json custom",
		"_shared/actuals-record.schema.json custom",
	} {
		if !strings.Contains(string(manifest), row) {
			t.Errorf("overlay.manifest does not track %q", row)
		}
	}

	skill, err := os.ReadFile(filepath.Join(root, "skills", "inception-pipeline", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), `version: "`+expectedContractVersion+`"`) {
		t.Errorf("inception-pipeline metadata is not version %s", expectedContractVersion)
	}
	const dispatcherCommand = "gentle-ai sdd-status {change} --cwd {project-root} --json --instructions"
	if !strings.Contains(string(skill), dispatcherCommand) {
		t.Errorf("inception-pipeline does not use the authoritative dispatcher command %q", dispatcherCommand)
	}

	contracts, err := os.ReadFile(filepath.Join(root, "skills", "_shared", "pre-sdd-contracts.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contracts), "Contract bundle version: `"+expectedContractVersion+"`") {
		t.Errorf("pre-sdd-contracts.md does not declare bundle version %s", expectedContractVersion)
	}

	schemaData, err := os.ReadFile(schemaPath(t))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	versionProperty := properties["contract_version"].(map[string]any)
	if _, locked := versionProperty["const"]; locked {
		t.Error("schema contract_version must be an enum of supported versions, not a const: a const cannot express a backwards-compatible extension")
	}
	acceptedVersions, ok := versionProperty["enum"].([]any)
	if !ok {
		t.Fatalf("schema contract_version has no enum: %v", versionProperty)
	}
	accepted := make(map[string]struct{}, len(acceptedVersions))
	for _, value := range acceptedVersions {
		accepted[value.(string)] = struct{}{}
	}
	for _, want := range append([]string{expectedContractVersion}, expectedSupersededContractVersions...) {
		if _, ok := accepted[want]; !ok {
			t.Errorf("schema accepted-version set %v does not contain %s", acceptedVersions, want)
		}
	}
	if id, _ := schema["$id"].(string); !strings.HasSuffix(id, "/"+expectedContractVersion) {
		t.Errorf("schema $id = %q, want a %s path", id, expectedContractVersion)
	}

	engineMod, err := os.ReadFile(filepath.Join(root, "engine", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(engineMod), "jsonschema") {
		t.Error("engine/go.mod must remain independent of the entry-contract validator")
	}

	actuals, err := os.ReadFile(filepath.Join(root, "skills", "_shared", "actuals-record.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(actuals), "/actuals-record/"+expectedContractVersion) {
		t.Errorf("actuals-record schema does not declare bundle version %s", expectedContractVersion)
	}

	validatorMod, err := os.ReadFile(filepath.Join(root, "tools", "entry-contract-validator", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(validatorMod), "github.com/santhosh-tekuri/jsonschema/v6 v6.0.2") {
		t.Error("validator module does not pin jsonschema/v6 v6.0.2")
	}
}

func TestOverlayCLIResolvesValidationPathsFromCaller(t *testing.T) {
	root := repoRoot(t)
	tempDir := t.TempDir()
	command := func(instance string) *exec.Cmd {
		cmd := exec.Command(
			"bash",
			filepath.Join(root, "bin", "labdrian-overlay"),
			"validate-entry-contract",
			"--schema", "skills/_shared/entry-contract.schema.json",
			"--instance", instance,
		)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "OVERLAY_DIR="+root, "TMPDIR="+tempDir)
		return cmd
	}

	cmd := command("tools/entry-contract-validator/testdata/valid-entry-contract.json")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("overlay validator wrapper rejected caller-relative paths: %v\n%s", err, output)
	}

	cmd = command("tools/entry-contract-validator/testdata/invalid-entry-contract.json")
	output, err := cmd.CombinedOutput()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("overlay validator wrapper accepted invalid fixture: err=%v\n%s", err, output)
	}
	if exitError.ExitCode() != exitSchemaValidation {
		t.Fatalf("wrapper exit code = %d, want %d\n%s", exitError.ExitCode(), exitSchemaValidation, output)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("wrapper left validator build artifacts in TMPDIR: %v", entries)
	}
}
