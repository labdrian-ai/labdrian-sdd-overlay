package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

const (
	// currentContractVersion is the bundle version this validator produces and
	// pins the schema against.
	currentContractVersion = "2.1.0"

	exitOK                 = 0
	exitUsage              = 2
	exitSchema             = 3
	exitInstance           = 4
	exitSchemaValidation   = 5
	exitSemanticValidation = 6
	exitLegacyContract     = 7
)

// supportedContractVersions lists every bundle version whose contracts this
// validator still accepts, oldest first with currentContractVersion last.
//
// The version is a compatibility set, not an exact-match lock. A lock cannot
// express a backwards-compatible extension: bumping it would invalidate every
// contract an earlier bundle already wrote, including archived ones that are an
// immutable historical record. Adding a member here, adding it to the schema's
// contract_version enum, and extending the schema's vocabulary is how such an
// extension ships. Removing a member is a breaking change.
var supportedContractVersions = []string{"2.0.0", currentContractVersion}

func isSupportedContractVersion(version string) bool {
	for _, supported := range supportedContractVersions {
		if version == supported {
			return true
		}
	}
	return false
}

// deliveryTimeSizeExceptionState records an overrun discovered at delivery
// rather than predicted at planning time.
//
// There is exactly one spelling, and it is the one the archived contract
// already carries. A second, more self-describing token would have to be
// accepted forever — the archived contract is immutable and CI validates it on
// every run — so it could never actually be retired. A synonym that cannot be
// removed is not a deprecation; it is a second vocabulary, and this repository
// already pays for undetected vocabulary drift. The name does not self-describe,
// so the schema's state description carries that meaning instead.
const deliveryTimeSizeExceptionState = "granted"

func isDeliveryTimeSizeException(state string) bool {
	return state == deliveryTimeSizeExceptionState
}

// nextRecommendationField is the schema property whose enum is the single
// owner of the native dispatcher's token domain (see
// skills/_shared/entry-contract.schema.json and
// skills/inception-pipeline/SKILL.md, which mirrors it in prose).
const nextRecommendationField = "expected_native_next_recommendation"

const usageText = `Usage: entry-contract-validator --schema PATH --instance PATH

Validates a pre-SDD entry contract with a compatible Draft 2020-12 schema, then
checks deterministic cross-field invariants.

The schema validates the UNION of every supported version's vocabulary, and
contract_version records which bundle produced the contract rather than gating
which vocabulary applies. Per-version feature gating is deliberately NOT
enforced: the schema carries no conditional keywords by design, so every
version-sensitive and cross-field invariant is checked here instead.

Options:
  --schema PATH       Entry-contract schema file
  --instance PATH     Entry-contract JSON instance
  --exists-root PATH  Opt-in: resolve every artifact_refs openspec_path against
                      PATH and fail if it is missing. Off by default. It is an
                      inception-time check for a live change directory only:
                      an archived contract points at a change directory that
                      archiving consumed, so enabling it against history would
                      make every archived contract permanently invalid.
  --version           Print validator/contract version
  --help              Show this help

Exit codes:
  0  valid contract, help, or version
  2  invalid command-line usage
  3  schema unavailable, malformed, incompatible, or uncompilable
  4  instance unavailable or malformed
  5  JSON Schema validation failed
  6  semantic invariant validation failed
  7  pre-v2 legacy contract: no contract_version declared, not validated.
     A contract declaring an unrecognised version is not legacy; it fails hard.
`

type contractError struct {
	code int
	err  error
}

func (e *contractError) Error() string { return e.err.Error() }
func (e *contractError) Unwrap() error { return e.err }

type artifactRef struct {
	TopicKey     string `json:"topic_key"`
	OpenSpecPath string `json:"openspec_path"`
}

type manifestRefs struct {
	Context artifactRef `json:"context"`
	Mission artifactRef `json:"mission"`
	Rules   artifactRef `json:"rules"`
}

type artifactRefs struct {
	Entry         artifactRef  `json:"entry"`
	Requirements  artifactRef  `json:"requirements"`
	Manifest      manifestRefs `json:"manifest"`
	Architecture  artifactRef  `json:"architecture"`
	Roadmap       artifactRef  `json:"roadmap"`
	Estimate      artifactRef  `json:"estimate"`
	SDDInit       artifactRef  `json:"sdd_init"`
	PipelineState artifactRef  `json:"pipeline_state"`
	Delivery      artifactRef  `json:"delivery"`
}

type numberRange struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

type entryContract struct {
	ContractVersion   string       `json:"contract_version"`
	ChangeName        string       `json:"change_name"`
	Project           string       `json:"project"`
	ArtifactStoreMode string       `json:"artifact_store_mode"`
	ArtifactRefs      artifactRefs `json:"artifact_refs"`
	Estimate          struct {
		PlannedRangeHours numberRange `json:"planned_range_hours"`
	} `json:"estimate"`
	RequestedPRStrategy string `json:"requested_pr_strategy"`
	DeliveryStrategy    string `json:"delivery_strategy"`
	ChainingRequired    bool   `json:"chaining_required"`
	ChainStrategy       string `json:"chain_strategy"`
	ReviewBudget        struct {
		MaxChangedLinesPerSlice int `json:"max_changed_lines_per_slice"`
		SizeException           struct {
			State        string `json:"state"`
			Reason       string `json:"reason"`
			ApprovedBy   string `json:"approved_by"`
			Scope        string `json:"scope"`
			ChangedLines int    `json:"changed_lines"`
			AuthorizedBy string `json:"authorized_by"`
		} `json:"size_exception"`
	} `json:"review_budget"`
	ReviewSlices []struct {
		Order                 int         `json:"order"`
		ID                    string      `json:"id"`
		Dependencies          []string    `json:"dependencies"`
		EstimatedChangedLines numberRange `json:"estimated_changed_lines"`
	} `json:"review_slices"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("entry-contract-validator", flag.ContinueOnError)
	flags.SetOutput(stderr)
	schemaFile := flags.String("schema", "", "entry-contract schema file")
	instanceFile := flags.String("instance", "", "entry-contract JSON instance")
	existsRoot := flags.String("exists-root", "", "opt-in: stat every declared openspec_path under this root")
	help := flags.Bool("help", false, "show help")
	shortHelp := flags.Bool("h", false, "show help")
	version := flags.Bool("version", false, "show version")
	flags.Usage = func() { fmt.Fprint(stdout, usageText) }

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if *help || *shortHelp {
		flags.Usage()
		return exitOK
	}
	if *version {
		fmt.Fprintf(stdout, "entry-contract-validator %s\n", currentContractVersion)
		return exitOK
	}
	if flags.NArg() != 0 || *schemaFile == "" || *instanceFile == "" {
		fmt.Fprintln(stderr, "error: --schema and --instance are required")
		return exitUsage
	}

	if err := validateFiles(*schemaFile, *instanceFile, validationOptions{existsRoot: *existsRoot}); err != nil {
		var classified *contractError
		if errors.As(err, &classified) {
			// A legacy contract is a skip, not a defect: it predates the
			// versioned contract and there is nothing to repair in it.
			if classified.code == exitLegacyContract {
				fmt.Fprintf(stderr, "skipped: %v\n", err)
			} else {
				fmt.Fprintf(stderr, "error: %v\n", err)
			}
			return classified.code
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitSemanticValidation
	}

	fmt.Fprintf(stdout, "entry contract valid (validator version %s)\n", currentContractVersion)
	return exitOK
}

// validationOptions carries opt-in checks that are deliberately off by default.
type validationOptions struct {
	// existsRoot, when non-empty, is the directory every declared
	// artifact_refs openspec_path is resolved against and stat'd.
	//
	// It must never default on. Archived contracts point at change
	// directories that archiving consumed, and the shipped fixtures declare
	// paths for an illustrative change that does not exist, so an on-by-default
	// stat would fail CI immediately and make history permanently invalid.
	// This is an inception-time check for a live change directory.
	existsRoot string
}

func validateFiles(schemaFile, instanceFile string, options validationOptions) error {
	schemaDocument, err := decodeJSONFile(schemaFile)
	if err != nil {
		return &contractError{code: exitSchema, err: fmt.Errorf("schema: %w", err)}
	}
	if err := validateSchemaVersion(schemaDocument); err != nil {
		return &contractError{code: exitSchema, err: fmt.Errorf("schema: %w", err)}
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const schemaResource = "urn:labdrian:pre-sdd-entry-contract"
	if err := compiler.AddResource(schemaResource, schemaDocument); err != nil {
		return &contractError{code: exitSchema, err: fmt.Errorf("add schema resource: %w", err)}
	}
	compiled, err := compiler.Compile(schemaResource)
	if err != nil {
		return &contractError{code: exitSchema, err: fmt.Errorf("compile Draft 2020-12 schema: %w", err)}
	}

	instance, err := decodeJSONFile(instanceFile)
	if err != nil {
		return &contractError{code: exitInstance, err: fmt.Errorf("instance: %w", err)}
	}
	// Classify a pre-v2 legacy contract before schema validation. A v1 file
	// shares almost no vocabulary with v2, so validating it would bury the one
	// fact a caller needs — this contract predates the versioned schema — under
	// a wall of shape errors indistinguishable from real corruption.
	if err := classifyLegacyContract(instance); err != nil {
		return err
	}
	if err := compiled.Validate(instance); err != nil {
		if diagnostic, ok := diagnoseNextRecommendationRejection(err); ok {
			return &contractError{code: exitSchemaValidation, err: fmt.Errorf("schema validation: %s: %w", diagnostic, err)}
		}
		return &contractError{code: exitSchemaValidation, err: fmt.Errorf("schema validation: %w", err)}
	}
	if err := validateSemantics(instance, options); err != nil {
		return &contractError{code: exitSemanticValidation, err: fmt.Errorf("semantic validation: %w", err)}
	}
	return nil
}

// classifyLegacyContract separates a pre-v2 legacy contract from a corrupt one
// so the difference is machine-readable. Only the absent-field case is legacy:
// an instance declaring a contract_version this bundle does not recognise is a
// hard failure, because it claims a vocabulary the validator cannot honour.
func classifyLegacyContract(instance any) error {
	document, ok := instance.(map[string]any)
	if !ok {
		return nil
	}
	if _, declared := document["contract_version"]; declared {
		return nil
	}
	return &contractError{code: exitLegacyContract, err: fmt.Errorf(
		"legacy contract: no contract_version declared, so this predates the versioned entry contract and was not validated (supported versions: %s)",
		strings.Join(supportedContractVersions, ", "),
	)}
}

// diagnoseNextRecommendationRejection inspects a JSON Schema validation
// failure for the specific case of an unknown expected_native_next_recommendation
// token. That failure is usually not a broken contract: the native
// gentle-ai dispatcher owns the real token domain, and the overlay's two
// mirrors (the schema enum and the inception-pipeline prose) can go stale
// when it adds or renames a token. When this specific case is detected, it
// returns a message that names the likely real cause instead of a generic
// schema-validation failure. It returns ok=false for every other failure,
// leaving the original error untouched.
func diagnoseNextRecommendationRejection(err error) (message string, ok bool) {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return "", false
	}
	token, found := findEnumRejection(validationErr, nextRecommendationField)
	if !found {
		return "", false
	}
	return fmt.Sprintf(
		"%s %q is not in the schema enum. "+
			"This token may be legitimate and newer than the overlay's mirror of the native dispatcher's token domain. "+
			"Update both mirrors together: skills/_shared/entry-contract.schema.json (the enum) and "+
			"skills/inception-pipeline/SKILL.md (the prose token list). "+
			"Confirm the real domain by running `gentle-ai sdd-status <change> --cwd <repo> --json --instructions` "+
			"and reading nextRecommended",
		nextRecommendationField, token,
	), true
}

// findEnumRejection walks the jsonschema validation error tree looking for
// an 'enum' keyword failure whose instance location is exactly the given
// top-level field. It returns the rejected value when found.
func findEnumRejection(validationErr *jsonschema.ValidationError, field string) (string, bool) {
	if len(validationErr.InstanceLocation) == 1 && validationErr.InstanceLocation[0] == field {
		if enumErr, ok := validationErr.ErrorKind.(*kind.Enum); ok {
			if token, ok := enumErr.Got.(string); ok {
				return token, true
			}
		}
	}
	for _, cause := range validationErr.Causes {
		if token, found := findEnumRejection(cause, field); found {
			return token, true
		}
	}
	return "", false
}

func decodeJSONFile(filename string) (any, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return document, nil
}

func validateSchemaVersion(document any) error {
	schema, ok := document.(map[string]any)
	if !ok {
		return fmt.Errorf("root must be an object")
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return fmt.Errorf("$schema must select Draft 2020-12")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("properties must be an object")
	}
	versionProperty, ok := properties["contract_version"].(map[string]any)
	if !ok {
		return fmt.Errorf("contract_version property is unavailable")
	}
	// The handshake is set membership, not equality: the schema may accept
	// versions this validator has retired, and it may accept versions a newer
	// validator will add. All that matters is that it accepts the version this
	// validator writes, so the two halves of the bundle agree.
	accepted, ok := versionProperty["enum"].([]any)
	if !ok {
		return fmt.Errorf("contract_version must declare an enum of accepted versions containing %q", currentContractVersion)
	}
	for _, value := range accepted {
		if version, _ := value.(string); version == currentContractVersion {
			return nil
		}
	}
	return fmt.Errorf("contract_version accepted-version set %v does not contain the validator's current version %q", accepted, currentContractVersion)
}

func validateSemantics(instance any, options validationOptions) error {
	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}
	var contract entryContract
	if err := json.Unmarshal(data, &contract); err != nil {
		return err
	}

	if !isSupportedContractVersion(contract.ContractVersion) {
		return fmt.Errorf("contract_version %q is not supported by this bundle (supported: %s)",
			contract.ContractVersion, strings.Join(supportedContractVersions, ", "))
	}
	if contract.Estimate.PlannedRangeHours.Low > contract.Estimate.PlannedRangeHours.High {
		return fmt.Errorf("estimate.planned_range_hours.low must not exceed high")
	}

	if contract.RequestedPRStrategy == "force-chained" &&
		(contract.DeliveryStrategy != "auto-chain" || !contract.ChainingRequired) {
		return fmt.Errorf("requested force-chained must normalize to delivery_strategy auto-chain with chaining_required=true")
	}
	if contract.RequestedPRStrategy == "force-single" &&
		(contract.DeliveryStrategy != "single-pr" && contract.DeliveryStrategy != "exception-ok") {
		return fmt.Errorf("requested force-single must normalize to single-pr or exception-ok")
	}
	if contract.ChainingRequired && contract.DeliveryStrategy != "auto-chain" {
		return fmt.Errorf("chaining_required=true requires delivery_strategy auto-chain")
	}
	if contract.DeliveryStrategy == "auto-chain" && !contract.ChainingRequired {
		return fmt.Errorf("delivery_strategy auto-chain requires chaining_required=true")
	}
	if (contract.DeliveryStrategy == "single-pr" || contract.DeliveryStrategy == "exception-ok") && contract.ChainingRequired {
		return fmt.Errorf("delivery_strategy %s requires chaining_required=false", contract.DeliveryStrategy)
	}
	if contract.ChainingRequired && contract.ChainStrategy == "none" {
		return fmt.Errorf("chaining_required=true requires a concrete chain_strategy")
	}
	if !contract.ChainingRequired && contract.ChainStrategy != "none" {
		return fmt.Errorf("chaining_required=false requires chain_strategy none")
	}

	seenIDs := make(map[string]struct{}, len(contract.ReviewSlices))
	overBudget := false
	for index, slice := range contract.ReviewSlices {
		wantOrder := index + 1
		if slice.Order != wantOrder {
			return fmt.Errorf("review_slices must be ordered contiguously: index %d has order %d, want %d", index, slice.Order, wantOrder)
		}
		if _, exists := seenIDs[slice.ID]; exists {
			return fmt.Errorf("review_slices contains duplicate id %q", slice.ID)
		}
		if slice.EstimatedChangedLines.Low > slice.EstimatedChangedLines.High {
			return fmt.Errorf("review slice %q estimated_changed_lines.low must not exceed high", slice.ID)
		}
		for _, dependency := range slice.Dependencies {
			if _, exists := seenIDs[dependency]; !exists {
				return fmt.Errorf("review slice %q dependency %q must name a prior slice", slice.ID, dependency)
			}
		}
		seenIDs[slice.ID] = struct{}{}
		if slice.EstimatedChangedLines.High > float64(contract.ReviewBudget.MaxChangedLinesPerSlice) {
			overBudget = true
		}
	}
	exception := contract.ReviewBudget.SizeException
	deliveryTime := isDeliveryTimeSizeException(exception.State)
	if overBudget && exception.State != "approved" && !deliveryTime {
		return fmt.Errorf("a review slice exceeds the per-slice budget but no size exception is approved")
	}
	// A delivery-time exception is legal precisely when the plan was within
	// budget: a realized overrun is a fact about what landed, and planned line
	// counts cannot predict it. Rejecting it here would force the record to
	// either lie about the plan or omit the overrun.
	if !overBudget && exception.State != "not-needed" && !deliveryTime {
		return fmt.Errorf("size exception must be %q when every review slice is within budget, or %q to record an overrun discovered at delivery",
			"not-needed", deliveryTimeSizeExceptionState)
	}
	if exception.State == "approved" && (exception.Reason == "" || exception.ApprovedBy == "") {
		return fmt.Errorf("size exception state approved requires both reason and approved_by: an approval that names neither an approver nor a justification records nothing a reviewer can act on")
	}
	if deliveryTime {
		for _, companion := range []struct {
			name    string
			missing bool
		}{
			{"scope", exception.Scope == ""},
			{"changed_lines", exception.ChangedLines == 0},
			{"authorized_by", exception.AuthorizedBy == ""},
			{"reason", exception.Reason == ""},
		} {
			if companion.missing {
				return fmt.Errorf("size exception state %q requires %s", exception.State, companion.name)
			}
		}
	}
	if contract.DeliveryStrategy == "exception-ok" && contract.ReviewBudget.SizeException.State != "approved" {
		return fmt.Errorf("delivery_strategy exception-ok requires an approved size exception")
	}

	expectedTopics := []struct {
		name string
		ref  artifactRef
		want string
	}{
		{"entry", contract.ArtifactRefs.Entry, "sdd/" + contract.ChangeName + "/entry"},
		{"requirements", contract.ArtifactRefs.Requirements, "project/" + contract.Project + "/requirements/" + contract.ChangeName},
		{"manifest.context", contract.ArtifactRefs.Manifest.Context, "project/" + contract.Project + "/manifest/context"},
		{"manifest.mission", contract.ArtifactRefs.Manifest.Mission, "project/" + contract.Project + "/manifest/mission"},
		{"manifest.rules", contract.ArtifactRefs.Manifest.Rules, "project/" + contract.Project + "/manifest/rules"},
		{"architecture", contract.ArtifactRefs.Architecture, "project/" + contract.Project + "/architect/final"},
		{"roadmap", contract.ArtifactRefs.Roadmap, "project/" + contract.Project + "/roadmap"},
		{"estimate", contract.ArtifactRefs.Estimate, "sdd/" + contract.ChangeName + "/estimate"},
		{"sdd_init", contract.ArtifactRefs.SDDInit, "sdd-init/" + contract.Project},
		{"pipeline_state", contract.ArtifactRefs.PipelineState, "sdd/" + contract.ChangeName + "/pipeline-state"},
		{"delivery", contract.ArtifactRefs.Delivery, "delivery/" + contract.ChangeName},
	}
	seenTopics := make(map[string]struct{}, len(expectedTopics))
	seenPaths := make(map[string]struct{}, len(expectedTopics))
	for _, item := range expectedTopics {
		if item.ref.TopicKey != item.want {
			return fmt.Errorf("%s topic_key %q must equal %q", item.name, item.ref.TopicKey, item.want)
		}
		if _, exists := seenTopics[item.ref.TopicKey]; exists {
			return fmt.Errorf("duplicate artifact topic_key %q", item.ref.TopicKey)
		}
		seenTopics[item.ref.TopicKey] = struct{}{}
		if item.ref.OpenSpecPath == "" {
			continue
		}
		if contract.ArtifactStoreMode == "engram" || contract.ArtifactStoreMode == "none" {
			return fmt.Errorf("%s openspec_path is not allowed in artifact_store_mode %s", item.name, contract.ArtifactStoreMode)
		}
		if err := validateOpenSpecPath(item.ref.OpenSpecPath); err != nil {
			return fmt.Errorf("%s openspec_path: %w", item.name, err)
		}
		if options.existsRoot != "" {
			target := filepath.Join(options.existsRoot, filepath.FromSlash(item.ref.OpenSpecPath))
			if _, err := os.Stat(target); err != nil {
				return fmt.Errorf("%s openspec_path %q does not exist under --exists-root %s", item.name, item.ref.OpenSpecPath, options.existsRoot)
			}
		}
		if _, exists := seenPaths[item.ref.OpenSpecPath]; exists {
			return fmt.Errorf("duplicate openspec_path %q", item.ref.OpenSpecPath)
		}
		seenPaths[item.ref.OpenSpecPath] = struct{}{}
	}

	return nil
}

func validateOpenSpecPath(value string) error {
	if strings.Contains(value, "\\") || path.IsAbs(value) || path.Clean(value) != value ||
		!strings.HasPrefix(value, "openspec/") || strings.Contains(value, "{") || strings.Contains(value, "}") {
		return fmt.Errorf("%q must be a normalized relative path below openspec/", value)
	}
	return nil
}
