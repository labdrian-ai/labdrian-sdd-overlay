package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var backtickTokenPattern = regexp.MustCompile("`([^`]+)`")

// extractSchemaNextRecommendationEnum reads the single owner of the token
// domain: the expected_native_next_recommendation enum in the entry-contract
// schema.
func extractSchemaNextRecommendationEnum(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(schemaPath(t))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties is not an object")
	}
	property, ok := properties[nextRecommendationField].(map[string]any)
	if !ok {
		t.Fatalf("schema is missing property %q", nextRecommendationField)
	}
	enumValues, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("schema property %q has no enum", nextRecommendationField)
	}
	tokens := make([]string, 0, len(enumValues))
	for _, value := range enumValues {
		token, ok := value.(string)
		if !ok {
			t.Fatalf("schema enum value %v is not a string", value)
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func inceptionPipelineSkillLines(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "skills", "inception-pipeline", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skills/inception-pipeline/SKILL.md: %v", err)
	}
	return strings.Split(string(data), "\n")
}

func findLineContaining(t *testing.T, lines []string, marker string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, marker) {
			return line
		}
	}
	t.Fatalf("skills/inception-pipeline/SKILL.md has no line containing %q", marker)
	return ""
}

func backtickTokens(text string) []string {
	matches := backtickTokenPattern.FindAllStringSubmatch(text, -1)
	tokens := make([]string, 0, len(matches))
	for _, match := range matches {
		tokens = append(tokens, match[1])
	}
	return tokens
}

func toTokenSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

// TestNextRecommendationTokenDomainMatchesInceptionPipelineProse pins the
// prose mirror at skills/inception-pipeline/SKILL.md against the single
// owner of the token domain: the expected_native_next_recommendation enum in
// skills/_shared/entry-contract.schema.json. The two must name exactly the
// same tokens, or the dispatcher gate documentation has drifted from what
// the validator actually accepts.
func TestNextRecommendationTokenDomainMatchesInceptionPipelineProse(t *testing.T) {
	schemaTokens := extractSchemaNextRecommendationEnum(t)
	proseLine := findLineContaining(t, inceptionPipelineSkillLines(t), "Preserve the native token exactly")
	proseTokens := backtickTokens(proseLine)

	schemaSet := toTokenSet(schemaTokens)
	proseSet := toTokenSet(proseTokens)

	var onlyInSchema, onlyInProse []string
	for token := range schemaSet {
		if _, ok := proseSet[token]; !ok {
			onlyInSchema = append(onlyInSchema, token)
		}
	}
	for token := range proseSet {
		if _, ok := schemaSet[token]; !ok {
			onlyInProse = append(onlyInProse, token)
		}
	}
	sort.Strings(onlyInSchema)
	sort.Strings(onlyInProse)

	if len(onlyInSchema) > 0 || len(onlyInProse) > 0 {
		t.Fatalf(
			"expected_native_next_recommendation token domain drifted between "+
				"skills/_shared/entry-contract.schema.json (single owner) and "+
				"skills/inception-pipeline/SKILL.md (prose mirror):\n"+
				"  present in schema enum but missing from SKILL.md prose: %v\n"+
				"  present in SKILL.md prose but missing from schema enum: %v",
			onlyInSchema, onlyInProse,
		)
	}
}

// TestColdStartExemptionTokensAreSubsetOfNextRecommendationEnum pins the
// control tokens named by the cold-start exemption clause (currently
// sdd-new and select-change) as a subset of the owner enum.
func TestColdStartExemptionTokensAreSubsetOfNextRecommendationEnum(t *testing.T) {
	schemaSet := toTokenSet(extractSchemaNextRecommendationEnum(t))
	coldStartLine := findLineContaining(t, inceptionPipelineSkillLines(t), "Cold-start exemption")

	const marker = "`nextRecommended` is "
	idx := strings.Index(coldStartLine, marker)
	if idx == -1 {
		t.Fatalf("cold-start exemption line does not contain %q: %q", marker, coldStartLine)
	}
	clause := coldStartLine[idx+len(marker):]
	if commaIdx := strings.Index(clause, ","); commaIdx != -1 {
		clause = clause[:commaIdx]
	}
	controlTokens := backtickTokens(clause)
	if len(controlTokens) == 0 {
		t.Fatalf("no control tokens found in cold-start exemption clause: %q", clause)
	}

	for _, token := range controlTokens {
		if _, ok := schemaSet[token]; !ok {
			t.Errorf("cold-start exemption control token %q is not present in the expected_native_next_recommendation enum", token)
		}
	}
}
