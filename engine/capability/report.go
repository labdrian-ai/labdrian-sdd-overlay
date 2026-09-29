package capability

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ReportVersion is the only Report wire version this package emits.
const ReportVersion = 1

// Report is the printable form of one or more declarations: what
// `runtime capabilities` prints. A report for a single target holds one
// declaration.
type Report struct {
	Version      int           `json:"version"`
	Declarations []Declaration `json:"declarations"`
}

// Marshal returns the indented, human-readable JSON representation of a
// valid report: fields in contract order, two-space indentation, and exactly
// one trailing newline. Two calls on equal reports return equal bytes.
//
// Marshal refuses an invalid report instead of printing it: the output is a
// public statement about what a runtime supports, so a declaration that
// breaks a rule (for example a supported claim without evidence) must never
// reach a reader.
func (r Report) Marshal() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, fmt.Errorf("marshal capability report: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal capability report: %w", err)
	}
	return append(data, '\n'), nil
}

// validate checks the pinned version, that at least one declaration is
// present, that every declaration is valid, and that no target is listed
// twice. It is first-error-wins.
func (r Report) validate() error {
	if r.Version != ReportVersion {
		return fmt.Errorf("version must be %d, got %d", ReportVersion, r.Version)
	}
	if len(r.Declarations) == 0 {
		return errors.New("at least one declaration is required")
	}
	seen := make(map[string]bool, len(r.Declarations))
	for i, d := range r.Declarations {
		if err := Validate(d); err != nil {
			return fmt.Errorf("declarations[%d]: %w", i, err)
		}
		if seen[d.Target] {
			return fmt.Errorf("declarations[%d]: target %q appears more than once", i, d.Target)
		}
		seen[d.Target] = true
	}
	return nil
}
