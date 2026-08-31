package promote

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Diagnostic is one LintPage finding.
type Diagnostic struct {
	Rule   string
	Detail string
}

var addressPattern = regexp.MustCompile(`^c-\d{6}$`)

// allowedStatuses is the vault's status enum extended with R-033's
// superseded/archived values (D7); LintPage tolerates both.
var allowedStatuses = map[string]bool{
	"seed": true, "developing": true, "mature": true, "evergreen": true,
	"superseded": true, "archived": true,
}

// requiredScalarFields are the universal frontmatter scalars every page
// must carry (WIKI.md; list fields are always emitted by EmitPage's
// writeListField and are not re-checked here, per the design's 6-rule
// line-budget mitigation for this slice).
var requiredScalarFields = []string{"type", "title", "address", "created", "updated", "status"}

// LintPage checks page against the vault's own contract, kept to exactly
// the 6 rules the design's line-budget mitigation names for this slice:
// required fields, the type/status enum, the address format, address_map
// consistency, wikilink resolvability, and an inbound index.md link.
// vaultRoot lets the last three rules inspect on-disk state; doctor
// (slice 8a) reuses LintPage's rules unchanged.
func LintPage(page Page, vaultRoot string) []Diagnostic {
	var diags []Diagnostic

	fields := parseFrontmatterFields(page.Frontmatter)
	for _, key := range requiredScalarFields {
		if fields[key] == "" {
			diags = append(diags, Diagnostic{Rule: "required-fields", Detail: "missing " + key})
		}
	}

	if fields["type"] != "" && fields["type"] != vaultType {
		diags = append(diags, Diagnostic{Rule: "enum", Detail: fmt.Sprintf("type %q is not %q", fields["type"], vaultType)})
	}
	if fields["status"] != "" && !allowedStatuses[fields["status"]] {
		diags = append(diags, Diagnostic{Rule: "enum", Detail: fmt.Sprintf("status %q is not a recognized value", fields["status"])})
	}

	if !addressPattern.MatchString(page.Address) {
		diags = append(diags, Diagnostic{Rule: "address-format", Detail: fmt.Sprintf("address %q does not match ^c-\\d{6}$", page.Address)})
	}

	if diag, ok := checkAddressMap(page, vaultRoot); !ok {
		diags = append(diags, diag)
	}
	diags = append(diags, checkWikilinksResolve(page, vaultRoot)...)
	if diag, ok := checkInboundIndexLink(page, vaultRoot); !ok {
		diags = append(diags, diag)
	}

	return diags
}

// parseFrontmatterFields extracts flat scalar `key: value` lines from
// rendered frontmatter; list-header lines (`tags:`) are skipped since only
// scalar universal fields are checked by the required-fields rule.
func parseFrontmatterFields(raw string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" || strings.HasPrefix(line, "- ") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || value == "[]" {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.Trim(value, `"`)
	}
	return fields
}

// checkAddressMap reports the address-map-consistency rule: page.Address
// must have an entry in .raw/.manifest.json's address_map pointing at
// page.Path. A missing manifest (address allocation is slice 5) passes --
// there is nothing yet to be inconsistent with.
func checkAddressMap(page Page, vaultRoot string) (Diagnostic, bool) {
	data, err := os.ReadFile(filepath.Join(vaultRoot, ".raw", ".manifest.json"))
	if err != nil {
		return Diagnostic{}, true
	}
	var manifest struct {
		AddressMap map[string]string `json:"address_map"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Diagnostic{Rule: "address-map", Detail: ".raw/.manifest.json is not valid JSON"}, false
	}
	for path, addr := range manifest.AddressMap {
		if addr != page.Address {
			continue
		}
		if path != page.Path {
			return Diagnostic{Rule: "address-map", Detail: fmt.Sprintf("address %s maps to %s, not %s", page.Address, path, page.Path)}, false
		}
		return Diagnostic{}, true
	}
	return Diagnostic{Rule: "address-map", Detail: fmt.Sprintf("address %s has no address_map entry", page.Address)}, false
}

// checkWikilinksResolve reports the wikilink-resolvability rule: every
// `[[c-NNNNNN|...]]` in the rendered page must resolve to an existing
// wiki/memory/<address>.md file.
func checkWikilinksResolve(page Page, vaultRoot string) []Diagnostic {
	var diags []Diagnostic
	for _, m := range wikilinkPattern.FindAllStringSubmatch(page.Frontmatter+page.Body, -1) {
		target := filepath.Join(vaultRoot, pagePathPrefix, m[1]+".md")
		if _, err := os.Stat(target); err != nil {
			diags = append(diags, Diagnostic{Rule: "wikilink-resolvability", Detail: fmt.Sprintf("%s does not resolve to an existing file", m[0])})
		}
	}
	return diags
}

// checkInboundIndexLink reports the inbound-index.md-link rule: wiki/
// index.md must exist and contain a wikilink to page.Address.
func checkInboundIndexLink(page Page, vaultRoot string) (Diagnostic, bool) {
	data, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		return Diagnostic{Rule: "inbound-index-link", Detail: "wiki/index.md is missing"}, false
	}
	if !strings.Contains(string(data), "[["+page.Address) {
		return Diagnostic{Rule: "inbound-index-link", Detail: fmt.Sprintf("wiki/index.md has no link to %s", page.Address)}, false
	}
	return Diagnostic{}, true
}
