package capability_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// validDeclaration returns a declaration that satisfies every rule:
// installation is supported with one test reference, and every other
// capability is unsupported with a stated limit. Each table case below breaks
// exactly one rule on a fresh copy, so a failure names the rule under test.
func validDeclaration() capability.Declaration {
	claims := make([]capability.Claim, 0, len(capability.Capabilities()))
	for _, c := range capability.Capabilities() {
		if c == capability.Installation {
			claims = append(claims, capability.Claim{
				Capability: c,
				Status:     capability.Supported,
				Tests:      []string{"runtime:TestExample"},
			})
			continue
		}
		claims = append(claims, capability.Claim{
			Capability: c,
			Status:     capability.Unsupported,
			Detail:     "not implemented",
		})
	}
	return capability.Declaration{Target: capability.TargetClaude, Claims: claims}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d *capability.Declaration)
		wantErr string // empty means the declaration must be valid
	}{
		// Declarations that must be accepted.
		{
			name:   "a declaration with one supported and seven unsupported claims is valid",
			mutate: func(d *capability.Declaration) {},
		},
		{
			name: "a partial claim with sorted tests and a limit is valid",
			mutate: func(d *capability.Declaration) {
				d.Claims[1] = capability.Claim{Capability: capability.Projection, Status: capability.Partial, Tests: []string{"a:TestOne", "b:TestTwo"}, Detail: "only the first stage is projected"}
			},
		},
		{
			name: "a supported claim may also carry a detail",
			mutate: func(d *capability.Declaration) {
				d.Claims[0].Detail = "install, update, uninstall, and status of the hook families"
			},
		},
		{
			name: "a detail exactly at the byte bound is valid",
			mutate: func(d *capability.Declaration) {
				d.Claims[1].Detail = strings.Repeat("a", capability.MaxTextBytes)
			},
		},
		{
			name: "an untested reason is valid on any target",
			mutate: func(d *capability.Declaration) {
				d.Untested = "cannot be exercised here"
			},
		},
		{
			name: "references compare as whole strings, so runtime/sub sorts before runtime:",
			mutate: func(d *capability.Declaration) {
				d.Claims[0].Tests = []string{"cmd:TestB", "runtime/sub:TestC", "runtime:TestA"}
			},
		},

		// Target.
		{
			name:    "an unknown target is refused",
			mutate:  func(d *capability.Declaration) { d.Target = "cursor" },
			wantErr: `target "cursor" is not one of`,
		},
		{
			name:    "an empty target is refused",
			mutate:  func(d *capability.Declaration) { d.Target = "" },
			wantErr: `target "" is not one of`,
		},
		{
			name:    "targets are case sensitive",
			mutate:  func(d *capability.Declaration) { d.Target = "Claude" },
			wantErr: `target "Claude" is not one of`,
		},

		// Untested.
		{
			name:    "an untested reason over the byte bound is refused",
			mutate:  func(d *capability.Declaration) { d.Untested = strings.Repeat("a", capability.MaxTextBytes+1) },
			wantErr: "untested exceeds the maximum of 512 bytes",
		},
		{
			name:    "an untested reason with a control character is refused",
			mutate:  func(d *capability.Declaration) { d.Untested = "line one\nline two" },
			wantErr: "untested contains a non-printable character",
		},

		// The capability set.
		{
			name:    "a missing capability is refused",
			mutate:  func(d *capability.Declaration) { d.Claims = d.Claims[:len(d.Claims)-1] },
			wantErr: `missing capability "memory-enforcement"`,
		},
		{
			name:    "a declaration without claims is refused",
			mutate:  func(d *capability.Declaration) { d.Claims = nil },
			wantErr: `missing capability "installation"`,
		},
		{
			name: "a duplicated capability is refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[3] = d.Claims[2]
			},
			wantErr: `claims[3]: capability "dispatch" appears more than once`,
		},
		{
			name: "a capability outside the closed set is refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[2].Capability = "teleport"
			},
			wantErr: `claims[2]: capability "teleport" is not in the closed set`,
		},
		{
			name: "claims out of the closed set's order are refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[1], d.Claims[2] = d.Claims[2], d.Claims[1]
			},
			wantErr: `claims[1]: capability "dispatch" is out of order, want "projection"`,
		},
		{
			name: "an extra claim beyond the closed set is refused as a duplicate",
			mutate: func(d *capability.Declaration) {
				d.Claims = append(d.Claims, d.Claims[0])
			},
			wantErr: `claims[8]: capability "installation" appears more than once`,
		},

		// Status.
		{
			name:    "a status outside the closed set is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Status = "maybe" },
			wantErr: `claims[1] (projection): status "maybe" is not in the closed set`,
		},
		{
			name:    "an empty status is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Status = "" },
			wantErr: `claims[1] (projection): status "" is not in the closed set`,
		},
		{
			name:    "restart_required is a lifecycle result, not a claim status",
			mutate:  func(d *capability.Declaration) { d.Claims[5].Status = "restart_required" },
			wantErr: `claims[5] (restart): status "restart_required" is not in the closed set`,
		},

		// Evidence requirements.
		{
			name:    "a supported claim without tests is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = nil },
			wantErr: `claims[0] (installation): status "supported" requires at least one test reference`,
		},
		{
			name: "a partial claim without tests is refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[1] = capability.Claim{Capability: capability.Projection, Status: capability.Partial, Detail: "limited"}
			},
			wantErr: `claims[1] (projection): status "partial" requires at least one test reference`,
		},
		{
			name: "an unsupported claim that names a test is refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[1].Tests = []string{"runtime:TestExample"}
			},
			wantErr: `claims[1] (projection): status "unsupported" must not name tests`,
		},

		// Limit requirements.
		{
			name: "a partial claim without a detail is refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[1] = capability.Claim{Capability: capability.Projection, Status: capability.Partial, Tests: []string{"a:TestOne"}}
			},
			wantErr: `claims[1] (projection): status "partial" requires a detail stating the limit`,
		},
		{
			name: "a partial claim whose detail is only whitespace is refused",
			mutate: func(d *capability.Declaration) {
				d.Claims[1] = capability.Claim{Capability: capability.Projection, Status: capability.Partial, Tests: []string{"a:TestOne"}, Detail: "  "}
			},
			wantErr: `claims[1] (projection): status "partial" requires a detail stating the limit`,
		},
		{
			name:    "an unsupported claim without a detail is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Detail = "" },
			wantErr: `claims[1] (projection): status "unsupported" requires a detail stating the limit`,
		},

		// Detail text.
		{
			name:    "a detail one byte over the bound is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Detail = strings.Repeat("a", capability.MaxTextBytes+1) },
			wantErr: "claims[1] (projection): detail exceeds the maximum of 512 bytes",
		},
		{
			name:    "a detail with a newline is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Detail = "first line\nsecond line" },
			wantErr: "claims[1] (projection): detail contains a non-printable character U+000A",
		},
		{
			name:    "a detail with a zero-width format character is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Detail = "looks" + string(rune(0x200B)) + "fine" },
			wantErr: "claims[1] (projection): detail contains a non-printable character U+200B",
		},
		{
			name:    "a detail with a non-breaking space is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Detail = "no" + string(rune(0x00A0)) + "break" },
			wantErr: "claims[1] (projection): detail contains a non-printable character U+00A0",
		},
		{
			name:    "a detail that is not valid UTF-8 is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[1].Detail = "bad \xff byte" },
			wantErr: "claims[1] (projection): detail is not valid UTF-8",
		},

		// Test reference format.
		{
			name:    "a reference without a colon is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtimeTestExample"} },
			wantErr: `test reference "runtimeTestExample" must match`,
		},
		{
			name:    "a reference whose directory has an uppercase letter is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"Runtime:TestExample"} },
			wantErr: `test reference "Runtime:TestExample" must match`,
		},
		{
			name:    "a reference whose name does not start with Test is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:BenchmarkExample"} },
			wantErr: `test reference "runtime:BenchmarkExample" must match`,
		},
		{
			name:    "a reference with an empty test name is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:Test"} },
			wantErr: `test reference "runtime:Test" must match`,
		},
		{
			name:    "a reference whose name has a hyphen is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:Test-Example"} },
			wantErr: `test reference "runtime:Test-Example" must match`,
		},
		{
			name:    "a reference that climbs out of the engine root is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"../etc:TestExample"} },
			wantErr: `test reference "../etc:TestExample" must match`,
		},
		{
			name:    "a reference with an absolute directory is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"/runtime:TestExample"} },
			wantErr: `test reference "/runtime:TestExample" must match`,
		},
		{
			name:    "a reference with an empty path element is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime//sub:TestExample"} },
			wantErr: `test reference "runtime//sub:TestExample" directory "runtime//sub" must be a clean relative path`,
		},
		{
			name:    "a reference whose directory ends in a slash is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime/:TestExample"} },
			wantErr: `test reference "runtime/:TestExample" directory "runtime/" must be a clean relative path`,
		},
		{
			name:    "a reference with two colons is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:TestA:TestB"} },
			wantErr: `test reference "runtime:TestA:TestB" must match`,
		},

		// Test reference set.
		{
			name:    "a duplicated reference is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:TestA", "runtime:TestA"} },
			wantErr: `claims[0] (installation): test reference "runtime:TestA" is listed more than once`,
		},
		{
			name:    "references that are not sorted are refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:TestB", "runtime:TestA"} },
			wantErr: `claims[0] (installation): test references must be sorted: "runtime:TestA" comes after "runtime:TestB"`,
		},
		{
			name:    "sorting compares whole references, so runtime: before runtime/sub is refused",
			mutate:  func(d *capability.Declaration) { d.Claims[0].Tests = []string{"runtime:TestA", "runtime/sub:TestC"} },
			wantErr: `claims[0] (installation): test references must be sorted: "runtime/sub:TestC" comes after "runtime:TestA"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validDeclaration()
			tt.mutate(&d)
			err := capability.Validate(d)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidateAcceptsEveryTarget pins that the target rule is exactly the
// closed set Targets() returns, so adding a target to one place and not the
// other cannot go unnoticed.
func TestValidateAcceptsEveryTarget(t *testing.T) {
	for _, target := range capability.Targets() {
		d := validDeclaration()
		d.Target = target
		if err := capability.Validate(d); err != nil {
			t.Errorf("Validate(target %q) = %v, want nil", target, err)
		}
	}
}
