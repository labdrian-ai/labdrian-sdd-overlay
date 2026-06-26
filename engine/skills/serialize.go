package skills

import (
	"fmt"
	"strings"
)

// forbiddenChars are characters the tokenizer rejects even inside quotes.
// A value containing any of them can never round-trip through ParseRegistry.
const forbiddenChars = "{}[]&*!\t"

// representable reports whether v can be safely emitted in the strict YAML subset.
// Returns false for values containing tokenizer-forbidden characters (ADR-7).
func representable(v string) bool {
	return !strings.ContainsAny(v, forbiddenChars)
}

// needsQuote reports whether v must be wrapped in double quotes for safe round-trip
// through ParseRegistry. ADR-7 quoting rules.
func needsQuote(v string) bool {
	if v == "" {
		return true
	}
	if v != strings.TrimSpace(v) {
		return true
	}
	// YAML indicator characters that change meaning when first byte.
	const indicators = `"'|>#-?:, `
	if strings.ContainsRune(indicators, rune(v[0])) {
		return true
	}
	if strings.Contains(v, ": ") {
		return true
	}
	if strings.HasSuffix(v, ":") {
		return true
	}
	if strings.Contains(v, " # ") {
		return true
	}
	return false
}

// scalar emits v as a quoted or bare scalar.
func scalar(v string) string {
	if needsQuote(v) {
		return `"` + v + `"`
	}
	return v
}

// Serialize emits reg as a strict-subset YAML byte slice — the exact inverse of
// ParseRegistry. Returns a non-nil error (and nil bytes) if any scalar value
// contains characters unrepresentable in the subset (ADR-7). The output is
// deterministic: identical Registry values always produce identical bytes.
// minimal: forced — ADR-6 (full re-emit, zero-dep invariant)
func Serialize(reg Registry) ([]byte, error) {
	var b strings.Builder

	// Check representability of all scalar values before writing any output.
	if err := checkRepresentable(reg); err != nil {
		return nil, err
	}

	// version: "1"  — always double-quoted (R-053 special rule)
	b.WriteString(`version: "`)
	b.WriteString(reg.Version)
	b.WriteString("\"\n")

	b.WriteString("skills:\n")

	for _, e := range reg.Skills {
		// Entry: dash at indent 2, first field inline.
		b.WriteString("  - id: ")
		b.WriteString(scalar(e.ID))
		b.WriteByte('\n')

		if e.Path != "" {
			b.WriteString("    path: ")
			b.WriteString(scalar(e.Path))
			b.WriteByte('\n')
		}

		// source block (field order: type → upstream → repo → ref; ADR-12)
		b.WriteString("    source:\n")
		b.WriteString("      type: ")
		b.WriteString(scalar(e.Source.Type))
		b.WriteByte('\n')
		if e.Source.Upstream != nil {
			b.WriteString("      upstream:\n")
			b.WriteString("        owner: ")
			b.WriteString(scalar(e.Source.Upstream.Owner))
			b.WriteByte('\n')
		}
		if e.Source.Repo != "" {
			b.WriteString("      repo: ")
			b.WriteString(scalar(e.Source.Repo))
			b.WriteByte('\n')
		}
		if e.Source.Ref != "" {
			b.WriteString("      ref: ")
			b.WriteString(scalar(e.Source.Ref))
			b.WriteByte('\n')
		}

		// install block
		b.WriteString("    install:\n")
		b.WriteString("      defaultScope: ")
		b.WriteString(scalar(e.Install.DefaultScope))
		b.WriteByte('\n')
		b.WriteString("      targets:\n")
		for _, t := range e.Install.Targets {
			b.WriteString("        - ")
			b.WriteString(scalar(t))
			b.WriteByte('\n')
		}
		if len(e.Install.AllowedProjects) > 0 {
			b.WriteString("      allowedProjects:\n")
			for _, p := range e.Install.AllowedProjects {
				b.WriteString("        - ")
				b.WriteString(scalar(p))
				b.WriteByte('\n')
			}
		}

		// lifecycle block
		b.WriteString("    lifecycle:\n")
		b.WriteString("      updateStrategy: ")
		b.WriteString(scalar(e.Lifecycle.UpdateStrategy))
		b.WriteByte('\n')
	}

	return []byte(b.String()), nil
}

// checkRepresentable walks every scalar in reg and returns an error for the first
// value that contains tokenizer-forbidden characters.
func checkRepresentable(reg Registry) error {
	if !representable(reg.Version) {
		return fmt.Errorf("serialize: version %q contains unrepresentable characters", reg.Version)
	}
	for _, e := range reg.Skills {
		for _, check := range []struct {
			field string
			value string
		}{
			{"id", e.ID},
			{"path", e.Path},
			{"source.type", e.Source.Type},
			{"install.defaultScope", e.Install.DefaultScope},
			{"lifecycle.updateStrategy", e.Lifecycle.UpdateStrategy},
		} {
			if !representable(check.value) {
				return fmt.Errorf("serialize: entry %q: field %s value %q contains unrepresentable characters",
					e.ID, check.field, check.value)
			}
		}
		if e.Source.Upstream != nil {
			if !representable(e.Source.Upstream.Owner) {
				return fmt.Errorf("serialize: entry %q: source.upstream.owner %q contains unrepresentable characters",
					e.ID, e.Source.Upstream.Owner)
			}
		}
		if !representable(e.Source.Repo) {
			return fmt.Errorf("serialize: entry %q: source.repo %q contains unrepresentable characters",
				e.ID, e.Source.Repo)
		}
		if !representable(e.Source.Ref) {
			return fmt.Errorf("serialize: entry %q: source.ref %q contains unrepresentable characters",
				e.ID, e.Source.Ref)
		}
		for _, t := range e.Install.Targets {
			if !representable(t) {
				return fmt.Errorf("serialize: entry %q: install.targets value %q contains unrepresentable characters",
					e.ID, t)
			}
		}
		for _, p := range e.Install.AllowedProjects {
			if !representable(p) {
				return fmt.Errorf("serialize: entry %q: install.allowedProjects value %q contains unrepresentable characters",
					e.ID, p)
			}
		}
	}
	return nil
}
