package capability_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

func reportOf(targets ...string) capability.Report {
	r := capability.Report{Version: capability.ReportVersion}
	for _, target := range targets {
		d := validDeclaration()
		d.Target = target
		r.Declarations = append(r.Declarations, d)
	}
	return r
}

func TestReportMarshalIsDeterministicIndentedJSON(t *testing.T) {
	report := reportOf(capability.TargetClaude, capability.TargetCodex)

	first, err := report.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	second, err := report.Marshal()
	if err != nil {
		t.Fatalf("Marshal (second call): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Marshal is not deterministic: two calls on one report differ")
	}

	if !bytes.HasSuffix(first, []byte("}\n")) || bytes.HasSuffix(first, []byte("\n\n")) {
		t.Errorf("output must end with exactly one trailing newline, ends %q", first[len(first)-3:])
	}
	if !json.Valid(first) {
		t.Fatalf("output is not valid JSON:\n%s", first)
	}
	if bytes.Contains(first, []byte("null")) {
		t.Errorf("output must never contain null:\n%s", first)
	}
	wantPrefix := "{\n" +
		"  \"version\": 1,\n" +
		"  \"declarations\": [\n" +
		"    {\n" +
		"      \"target\": \"claude\",\n" +
		"      \"claims\": [\n" +
		"        {\n" +
		"          \"capability\": \"installation\",\n" +
		"          \"status\": \"supported\",\n" +
		"          \"tests\": [\n" +
		"            \"runtime:TestExample\"\n" +
		"          ]\n" +
		"        },\n" +
		"        {\n" +
		"          \"capability\": \"projection\",\n" +
		"          \"status\": \"unsupported\",\n" +
		"          \"tests\": [],\n" +
		"          \"detail\": \"not implemented\"\n" +
		"        },\n"
	if !strings.HasPrefix(string(first), wantPrefix) {
		t.Errorf("output does not start with the expected two-space indented layout:\n%s", first)
	}
}

func TestReportMarshalRoundTripsToTheSameBytes(t *testing.T) {
	report := reportOf(capability.TargetClaude, capability.TargetOpenCode)
	report.Declarations[1].Untested = "cannot be exercised here"

	first, err := report.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded capability.Report
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	again, err := decoded.Marshal()
	if err != nil {
		t.Fatalf("Marshal of the decoded report: %v", err)
	}
	if !bytes.Equal(first, again) {
		t.Fatalf("decode then encode changed the bytes:\nfirst:\n%s\nagain:\n%s", first, again)
	}
	if !strings.Contains(string(first), `"untested": "cannot be exercised here"`) {
		t.Errorf("untested reason missing from output:\n%s", first)
	}
}

func TestReportMarshalRefusesAnInvalidReport(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(r *capability.Report)
		wantErr string
	}{
		{
			name:    "a version other than the pinned one is refused",
			mutate:  func(r *capability.Report) { r.Version = 2 },
			wantErr: "version must be 1, got 2",
		},
		{
			name:    "a zero version is refused",
			mutate:  func(r *capability.Report) { r.Version = 0 },
			wantErr: "version must be 1, got 0",
		},
		{
			name:    "a report without declarations is refused",
			mutate:  func(r *capability.Report) { r.Declarations = nil },
			wantErr: "at least one declaration is required",
		},
		{
			name: "an invalid declaration is refused and named by position",
			mutate: func(r *capability.Report) {
				r.Declarations[1].Claims[1].Detail = ""
			},
			wantErr: `declarations[1]: claims[1] (projection): status "unsupported" requires a detail stating the limit`,
		},
		{
			name: "a target listed twice is refused",
			mutate: func(r *capability.Report) {
				r.Declarations[1].Target = capability.TargetClaude
			},
			wantErr: `declarations[1]: target "claude" appears more than once`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := reportOf(capability.TargetClaude, capability.TargetCodex)
			tt.mutate(&report)
			out, err := report.Marshal()
			if err == nil {
				t.Fatalf("Marshal() = %s, want an error containing %q", out, tt.wantErr)
			}
			if out != nil {
				t.Errorf("Marshal() returned bytes alongside an error: %s", out)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Marshal() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
