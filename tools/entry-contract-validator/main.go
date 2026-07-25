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
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	contractVersion = "2.0.0"

	exitOK                 = 0
	exitUsage              = 2
	exitSchema             = 3
	exitInstance           = 4
	exitSchemaValidation   = 5
	exitSemanticValidation = 6
)

const usageText = `Usage: entry-contract-validator --schema PATH --instance PATH

Validates a pre-SDD entry contract with the version-matched Draft 2020-12
schema, then checks deterministic cross-field invariants.

Options:
  --schema PATH    Entry-contract schema file
  --instance PATH  Entry-contract JSON instance
  --version        Print validator/contract version
  --help           Show this help

Exit codes:
  0  valid contract, help, or version
  2  invalid command-line usage
  3  schema unavailable, malformed, incompatible, or uncompilable
  4  instance unavailable or malformed
  5  JSON Schema validation failed
  6  semantic invariant validation failed
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
			State string `json:"state"`
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
		fmt.Fprintf(stdout, "entry-contract-validator %s\n", contractVersion)
		return exitOK
	}
	if flags.NArg() != 0 || *schemaFile == "" || *instanceFile == "" {
		fmt.Fprintln(stderr, "error: --schema and --instance are required")
		return exitUsage
	}

	if err := validateFiles(*schemaFile, *instanceFile); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		var classified *contractError
		if errors.As(err, &classified) {
			return classified.code
		}
		return exitSemanticValidation
	}

	fmt.Fprintf(stdout, "entry contract valid (version %s)\n", contractVersion)
	return exitOK
}

func validateFiles(schemaFile, instanceFile string) error {
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
	if err := compiled.Validate(instance); err != nil {
		return &contractError{code: exitSchemaValidation, err: fmt.Errorf("schema validation: %w", err)}
	}
	if err := validateSemantics(instance); err != nil {
		return &contractError{code: exitSemanticValidation, err: fmt.Errorf("semantic validation: %w", err)}
	}
	return nil
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
	if got, _ := versionProperty["const"].(string); got != contractVersion {
		return fmt.Errorf("contract_version const %q does not match validator %q", got, contractVersion)
	}
	return nil
}

func validateSemantics(instance any) error {
	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}
	var contract entryContract
	if err := json.Unmarshal(data, &contract); err != nil {
		return err
	}

	if contract.ContractVersion != contractVersion {
		return fmt.Errorf("contract_version %q does not match validator %q", contract.ContractVersion, contractVersion)
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
	if overBudget && contract.ReviewBudget.SizeException.State != "approved" {
		return fmt.Errorf("a review slice exceeds the per-slice budget but no size exception is approved")
	}
	if !overBudget && contract.ReviewBudget.SizeException.State != "not-needed" {
		return fmt.Errorf("size exception must be not-needed when every review slice is within budget")
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
