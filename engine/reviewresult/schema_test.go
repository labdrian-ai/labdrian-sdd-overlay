package reviewresult

// The JSON Schema of the reply (phase9-c26-review-contract S2 a) is held to the members and the constants of the
// package, so the schema a reviewer is told to follow and the parser that reads its reply cannot drift apart.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// node is the part of a JSON Schema object these tests read.
type node struct {
	Type                 string          `json:"type"`
	AdditionalProperties *bool           `json:"additionalProperties"`
	Required             []string        `json:"required"`
	Properties           map[string]node `json:"properties"`
	Enum                 []string        `json:"enum"`
	Items                *node           `json:"items"`
	Pattern              string          `json:"pattern"`
	MinItems             int             `json:"minItems"`
	MinLength            int             `json:"minLength"`
}

func loadSchema(t *testing.T) node {
	t.Helper()
	var root node
	if err := json.Unmarshal([]byte(Schema()), &root); err != nil {
		t.Fatalf("Schema() is not JSON: %v", err)
	}
	return root
}

func sorted(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func strs[T ~string](in []T) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

func keys(m map[string]node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return sorted(out)
}

func TestSchemaIsCompactJSON(t *testing.T) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(Schema())); err != nil {
		t.Fatalf("Schema() = %q: %v; want one JSON value", Schema(), err)
	}
	if compact.String() != Schema() {
		t.Errorf("Schema() = %q; want the compact form %q, as json.dumps(separators=(',', ':')) prints it", Schema(), compact.String())
	}
}

func TestSchemaRequiresEveryMemberOfEveryObjectOfTheContract(t *testing.T) {
	root := loadSchema(t)
	finding := root.Properties["findings"].Items
	if finding == nil {
		t.Fatalf("the schema has no items for findings")
	}
	for _, c := range []struct {
		name string
		obj  node
		want []string
	}{
		{"the reply", root, resultMembers},
		{"candidate", root.Properties["candidate"], candidateMembers},
		{"inspection", root.Properties["inspection"], inspectionMembers},
		{"a finding", *finding, findingMembers},
	} {
		if !reflect.DeepEqual(sorted(c.obj.Required), sorted(c.want)) {
			t.Errorf("%s: required = %v; want the members %v", c.name, c.obj.Required, c.want)
		}
		if !reflect.DeepEqual(keys(c.obj.Properties), sorted(c.want)) {
			t.Errorf("%s: properties = %v; want the members %v", c.name, keys(c.obj.Properties), c.want)
		}
		if c.obj.AdditionalProperties == nil || *c.obj.AdditionalProperties {
			t.Errorf("%s: additionalProperties is not false, but the contract refuses a member it does not name", c.name)
		}
	}
}

func TestSchemaEnumerationsAreTheConstantsOfThePackage(t *testing.T) {
	root := loadSchema(t)
	finding := root.Properties["findings"].Items
	if finding == nil {
		t.Fatalf("the schema has no items for findings")
	}
	for _, c := range []struct {
		name string
		got  []string
		want []string
	}{
		{"inspection.status", root.Properties["inspection"].Properties["status"].Enum, []string{string(InspectionCompleted), "incomplete"}},
		{"severity", finding.Properties["severity"].Enum, strs(severities)},
		{"evidence_class", finding.Properties["evidence_class"].Enum, strs(evidenceClasses)},
		{"causal_disposition", finding.Properties["causal_disposition"].Enum, strs(dispositions)},
	} {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s: enum = %v; want %v, in the order of the constants", c.name, c.got, c.want)
		}
	}
}

// A reply that fills every member the schema names with a value it allows passes Parse and CheckLocations.
func TestAReplyOfLegalValuesOfTheSchemaPassesParseAndCheckLocations(t *testing.T) {
	root := loadSchema(t)
	finding := root.Properties["findings"].Items
	if finding == nil {
		t.Fatalf("the schema has no items for findings")
	}
	b := Binding{
		Base:       strings.Repeat("1", 40),
		Head:       strings.Repeat("2", 40),
		DiffSHA256: strings.Repeat("3", 64),
		Paths:      []string{"engine/a.go", "b.md"},
	}
	var findings []map[string]any
	for i, severity := range finding.Properties["severity"].Enum {
		class := finding.Properties["evidence_class"].Enum
		disposition := finding.Properties["causal_disposition"].Enum
		findings = append(findings, map[string]any{
			"id":                 "F" + string(rune('1'+i)),
			"severity":           severity,
			"location":           "engine/a.go:" + string(rune('1'+i)) + "-9",
			"claim":              "a claim",
			"evidence_class":     class[i%len(class)],
			"causal_disposition": disposition[i%len(disposition)],
			"proof_refs":         []string{"engine/a.go:1 is a changed line"},
		})
	}
	data, err := json.Marshal(map[string]any{
		"candidate":  map[string]any{"base": b.Base, "head": b.Head, "diff_sha256": b.DiffSHA256},
		"inspection": map[string]any{"status": root.Properties["inspection"].Properties["status"].Enum[0], "paths": b.Paths, "reason": ""},
		"findings":   findings,
		"evidence":   []string{"read the diff"},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := Parse(data, b)
	if err != nil {
		t.Fatalf("Parse refused a reply of legal values of the schema: %v", err)
	}
	if err := CheckLocations(result); err != nil {
		t.Errorf("CheckLocations refused a reply of legal values of the schema: %v", err)
	}
	if len(result.Findings) != len(severities) {
		t.Errorf("Parse kept %d findings; want one per severity, %d", len(result.Findings), len(severities))
	}
}

func TestSchemaPatternsAreTheOnesTheBenchSent(t *testing.T) {
	finding := loadSchema(t).Properties["findings"].Items
	if finding == nil {
		t.Fatalf("the schema has no items for findings")
	}
	if got := finding.Properties["id"].Pattern; got != `^F[1-9][0-9]*$` {
		t.Errorf("id pattern = %q", got)
	}
	if got := finding.Properties["location"].Pattern; got != `^[^:\s]+:[1-9][0-9]*(-[1-9][0-9]*)?$` {
		t.Errorf("location pattern = %q", got)
	}
}
