package capability

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxTextBytes bounds the free text of a declaration: a claim's Detail and a
// declaration's Untested reason. Both are a sentence or two of human
// explanation, so 512 bytes is generous for a stated limit. The bound keeps a
// declaration reviewable at a glance and the printed report small (four
// targets, eight claims each), and it stops a declaration from growing
// without limit.
const MaxTextBytes = 512

// testRefPattern is the format of a test reference: a directory relative to
// the engine root, a colon, and the name of a test function, for example
// "runtime:TestPiAdapter_UninstallUsesRemoveNotUninstall". The directory
// alphabet excludes dots, so a reference cannot climb out of the engine root
// with "..", and excludes uppercase letters, which no engine package
// directory uses.
var testRefPattern = regexp.MustCompile(`^[a-z][a-z0-9/_-]*:Test[A-Za-z0-9_]+$`)

// Validate checks every rule of a declaration and returns the first
// violation:
//
//   - Target is one of Targets().
//   - Every capability of the closed set appears exactly once, and the claims
//     are stored in the closed set's fixed order.
//   - Every Status is in the closed set.
//   - A supported or partial claim names at least one test; an unsupported
//     claim names none.
//   - A partial or unsupported claim has a non-blank Detail stating the
//     limit.
//   - Detail and Untested are at most MaxTextBytes of valid UTF-8 made only
//     of printable characters (no control, format, or non-ASCII space
//     characters).
//   - Each test reference matches <dir>:<TestName>, names a clean relative
//     directory, and the references of a claim are unique and sorted as
//     whole strings.
//
// Validate reads nothing outside d. Whether a named test exists is the job of
// capabilitytest.CheckEvidence.
func Validate(d Declaration) error {
	if !isTarget(d.Target) {
		return fmt.Errorf("target %q is not one of %s", d.Target, strings.Join(Targets(), ", "))
	}
	if err := validateText("untested", d.Untested); err != nil {
		return err
	}
	if err := validateClaimSet(d.Claims); err != nil {
		return err
	}
	for i, c := range d.Claims {
		if err := validateClaim(c); err != nil {
			return fmt.Errorf("claims[%d] (%s): %w", i, c.Capability, err)
		}
	}
	return nil
}

func isTarget(target string) bool {
	for _, t := range targetOrder {
		if t == target {
			return true
		}
	}
	return false
}

func knownCapability(c Capability) bool {
	for _, known := range capabilityOrder {
		if known == c {
			return true
		}
	}
	return false
}

// validateClaimSet checks that claims hold every capability of the closed set
// exactly once, in the closed set's order. The checks run from the most
// specific failure to the least, so the message names what is actually wrong
// (an unknown or repeated capability, then a missing one, then a misplaced
// one) instead of a bare "wrong order".
func validateClaimSet(claims []Claim) error {
	seen := make(map[Capability]bool, len(capabilityOrder))
	for i, c := range claims {
		if !knownCapability(c.Capability) {
			return fmt.Errorf("claims[%d]: capability %q is not in the closed set", i, c.Capability)
		}
		if seen[c.Capability] {
			return fmt.Errorf("claims[%d]: capability %q appears more than once", i, c.Capability)
		}
		seen[c.Capability] = true
	}
	for _, c := range capabilityOrder {
		if !seen[c] {
			return fmt.Errorf("missing capability %q", c)
		}
	}
	// Every claim is known and unique, and none is missing, so len(claims) is
	// exactly len(capabilityOrder) and indexing capabilityOrder is safe.
	for i, c := range claims {
		if c.Capability != capabilityOrder[i] {
			return fmt.Errorf("claims[%d]: capability %q is out of order, want %q at this position", i, c.Capability, capabilityOrder[i])
		}
	}
	return nil
}

func validateClaim(c Claim) error {
	switch c.Status {
	case Supported, Partial:
		if len(c.Tests) == 0 {
			return fmt.Errorf("status %q requires at least one test reference", c.Status)
		}
	case Unsupported:
		if len(c.Tests) != 0 {
			return fmt.Errorf("status %q must not name tests, got %d", c.Status, len(c.Tests))
		}
	default:
		return fmt.Errorf("status %q is not in the closed set (%s, %s, %s)", c.Status, Supported, Partial, Unsupported)
	}
	if c.Status != Supported && strings.TrimSpace(c.Detail) == "" {
		return fmt.Errorf("status %q requires a detail stating the limit", c.Status)
	}
	if err := validateText("detail", c.Detail); err != nil {
		return err
	}
	return validateTestRefs(c.Tests)
}

// validateText bounds a free-text field and restricts it to printable text.
// unicode.IsPrint admits letters, marks, numbers, punctuation, symbols, and
// the ASCII space; it rejects control characters, format characters such as a
// zero-width space or a bidirectional override, and every other space, so the
// text renders the same everywhere it is copied.
func validateText(field, s string) error {
	if len(s) > MaxTextBytes {
		return fmt.Errorf("%s exceeds the maximum of %d bytes", field, MaxTextBytes)
	}
	// Checked before the rune loop: ranging over invalid UTF-8 yields
	// U+FFFD, which is printable and would slip through.
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not valid UTF-8", field)
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return fmt.Errorf("%s contains a non-printable character %U", field, r)
		}
	}
	return nil
}

func validateTestRefs(refs []string) error {
	for i, ref := range refs {
		if _, _, err := ParseTestRef(ref); err != nil {
			return err
		}
		if i == 0 {
			continue
		}
		switch prev := refs[i-1]; {
		case ref == prev:
			return fmt.Errorf("test reference %q is listed more than once", ref)
		case ref < prev:
			return fmt.Errorf("test references must be sorted: %q comes after %q", ref, prev)
		}
	}
	return nil
}

// ParseTestRef splits a test reference into its directory (relative to the
// engine root, with forward slashes) and its test function name. It is the
// single definition of a well-formed reference: Validate uses it for the
// format rule and capabilitytest.CheckEvidence uses it before touching the
// filesystem, so a reference that could name a path outside the engine root
// never reaches it. It is pure.
func ParseTestRef(ref string) (dir, name string, err error) {
	if !testRefPattern.MatchString(ref) {
		return "", "", fmt.Errorf("test reference %q must match <dir>:<TestName> (%s)", ref, testRefPattern)
	}
	dir, name, _ = strings.Cut(ref, ":")
	// The alphabet already excludes dots, so only empty path elements (a
	// doubled or trailing slash) remain to be rejected here.
	if path.Clean(dir) != dir {
		return "", "", fmt.Errorf("test reference %q directory %q must be a clean relative path", ref, dir)
	}
	return dir, name, nil
}
