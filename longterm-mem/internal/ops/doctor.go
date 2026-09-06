package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// Check status values.
const (
	CheckPassed = "PASS"
	CheckFailed = "FAIL"
)

// Check names -- the five diagnostics R-011 requires.
const (
	CheckVaultConfigResolvable        = "vault-config-resolvable"
	CheckAddressMapIntegrity          = "address-map-integrity"
	CheckWikiRegistrationConsistency  = "wiki-registration-consistency"
	CheckPrecedenceSidecarConsistency = "precedence-sidecar-consistency"
	CheckRuntimePrerequisites         = "runtime-prerequisites"
)

// Check names -- the three additional embedding-index diagnostics R-064
// requires, read-only over internal/vecindex's own state.
const (
	CheckEmbeddingIndexPresent     = "embedding-index-present"
	CheckEmbeddingIndexFresh       = "embedding-index-fresh"
	CheckEmbeddingBackendReachable = "embedding-backend-reachable"
)

// requiredPrerequisite is the one external runtime dependency the vault's
// own index scripts need (D12): python3, the interpreter
// vault.Runner.RunInterpreted uses for scripts/contextual-prefix.py and
// scripts/bm25-index.py.
const requiredPrerequisite = "python3"

// promotedPagesDir is the vault-relative directory promoted pages live
// under, mirroring promote's own (unexported) pagePathPrefix constant
// (D7: wiki/memory/<address>.md).
const promotedPagesDir = "wiki/memory"

// logRelPath mirrors promote's own (unexported) logMdRelPath constant
// (register.go): the vault's append-only promotion log, the on-disk
// source the wiki-registration-consistency check's log half reads. The
// catalog half (wiki/index.md) needs no equivalent constant here -- it is
// checked entirely through promote.LintPage's reused inbound-index-link
// rule, which already knows that path.
const logRelPath = "wiki/log.md"

// precedenceSidecarRelPath mirrors promote's own (unexported)
// precedenceManifestRelPath constant (store.go): longterm-mem's
// last-written-by-us fingerprint file, named in this check's details so an
// operator reading a FAIL knows which file to look at. The file itself is
// read through promote.LoadPrecedenceStore, never parsed here.
const precedenceSidecarRelPath = ".raw/.longterm-mem-manifest.json"

// Check is one named diagnostic's result.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// DoctorDeps are Doctor's dependencies (function-seam convention matching
// StatusDeps): PrerequisitePresent is a seam so a test can prove the
// runtime-prerequisites check's own logic without depending on what is
// actually installed on the host running the test.
type DoctorDeps struct {
	// VaultRoot is the (possibly unresolvable) vault path Doctor inspects.
	VaultRoot string
	// PrerequisitePresent reports whether name is present as a runtime
	// prerequisite. Production wires vault.PrerequisitePresent (R-021: no
	// direct os/exec import outside internal/vault/runner.go). Required.
	PrerequisitePresent func(name string) bool

	// StateDir is the directory this module's own derived state (the
	// embedding index) lives under -- vecindex.Dir(StateDir, project)
	// resolves the project's index directory. Production wires the same
	// state directory `index --embeddings` writes to (cmd's
	// defaultStateDir). Required for the three embedding checks (R-064).
	StateDir string
	// LiveObservationIDs returns every live (non-soft-deleted) engram_id
	// for project, so embedding-index-fresh can name exactly how many of
	// them the index is missing. Production wires
	// engram.Store.ListObservations (already R-020-scoped), mapped to IDs.
	// Required.
	LiveObservationIDs func(project string) ([]int64, error)
	// EmbeddingBackendCheck probes the configured embedding backend and
	// reports nil when it answered with the required model present, or a
	// typed error -- *embed.BackendUnreachableError or
	// *embed.ModelMissingError -- otherwise. This is the seam that keeps
	// doctor.go itself out of net_allowlist_test.go's allowedNetImporters
	// (R-071): production wires embed.Client.Embed with a small probe
	// input, this package never imports net/http directly. Required.
	EmbeddingBackendCheck func(ctx context.Context) error
}

// DoctorReport is Doctor's R-011 output: each of the five named checks'
// individual result.
type DoctorReport struct {
	Project string  `json:"project"`
	Checks  []Check `json:"checks"`
}

// Doctor runs R-011's five read-only diagnostic checks independently: one
// check's own failure -- an unresolvable path, a missing directory, a
// corrupted manifest -- must never prevent the other three from running or
// reporting (slice 7's review finding: a per-item failure must never abort
// a whole run). Each check function below therefore never returns early
// out of this assembly; it reports FAIL with a detail instead of erroring.
// Doctor itself only returns a non-nil error for a condition outside any
// single check's own reporting (there is none today; every failure mode
// this slice defines is expressed as a Check, not a Doctor-level error).
func Doctor(ctx context.Context, deps DoctorDeps, project string) (DoctorReport, error) {
	return DoctorReport{
		Project: project,
		Checks: []Check{
			checkVaultConfigResolvable(deps.VaultRoot),
			checkAddressMapIntegrity(deps.VaultRoot),
			checkWikiRegistrationConsistency(deps.VaultRoot),
			checkPrecedenceSidecarConsistency(deps.VaultRoot),
			checkRuntimePrerequisites(deps),
			checkEmbeddingIndexPresent(deps, project),
			checkEmbeddingIndexFresh(deps, project),
			checkEmbeddingBackendReachable(ctx, deps),
		},
	}, nil
}

// checkVaultConfigResolvable reports whether vaultRoot resolves to an
// existing directory on disk (R-011 scenario: "Unresolvable vault config
// is named").
func checkVaultConfigResolvable(vaultRoot string) Check {
	if vaultRoot == "" {
		return Check{Name: CheckVaultConfigResolvable, Status: CheckFailed, Detail: "vault root is not configured"}
	}
	info, err := os.Stat(vaultRoot)
	if err != nil {
		return Check{Name: CheckVaultConfigResolvable, Status: CheckFailed, Detail: fmt.Sprintf("%s does not resolve to an existing directory: %v", vaultRoot, err)}
	}
	if !info.IsDir() {
		return Check{Name: CheckVaultConfigResolvable, Status: CheckFailed, Detail: fmt.Sprintf("%s is not a directory", vaultRoot)}
	}
	return Check{Name: CheckVaultConfigResolvable, Status: CheckPassed}
}

// checkAddressMapIntegrity reports every promoted page's address-map
// inconsistency (R-011 scenario: "Corrupted address-map entry is named"),
// reusing promote.LintPage's own address-map-consistency rule (lint.go's
// checkAddressMap) rather than re-implementing it (task 8a description).
// A vault root that cannot be scanned for promoted pages at all (missing
// directory) reports PASS -- there is nothing yet to be inconsistent with,
// matching checkAddressMap's own graceful handling of a missing manifest.
func checkAddressMapIntegrity(vaultRoot string) Check {
	pages, unreadable, err := loadPromotedPages(vaultRoot)
	if err != nil {
		return Check{Name: CheckAddressMapIntegrity, Status: CheckFailed, Detail: err.Error()}
	}

	details := append([]string(nil), unreadable...)
	for _, page := range pages {
		for _, diag := range promote.LintPage(page, vaultRoot) {
			if diag.Rule == "address-map" {
				details = append(details, diag.Detail)
			}
		}
	}
	if len(details) > 0 {
		return Check{Name: CheckAddressMapIntegrity, Status: CheckFailed, Detail: strings.Join(details, "; ")}
	}
	return Check{Name: CheckAddressMapIntegrity, Status: CheckPassed}
}

// checkWikiRegistrationConsistency reports every promoted page absent from
// the vault's master catalog (wiki/index.md) and/or its append-only
// promotion log (wiki/log.md) (R-011 scenario: "Unregistered promoted page
// is named"). The catalog half reuses promote.LintPage's own
// inbound-index-link rule (lint.go's checkInboundIndexLink) -- the same
// on-disk marker block RegisterIndex writes -- rather than
// re-implementing it; LintPage has no equivalent log.md rule, so the log
// half is this check's own, small and self-contained.
func checkWikiRegistrationConsistency(vaultRoot string) Check {
	pages, unreadable, err := loadPromotedPages(vaultRoot)
	if err != nil {
		return Check{Name: CheckWikiRegistrationConsistency, Status: CheckFailed, Detail: err.Error()}
	}

	logData, logErr := os.ReadFile(filepath.Join(vaultRoot, logRelPath))

	details := append([]string(nil), unreadable...)
	for _, page := range pages {
		for _, diag := range promote.LintPage(page, vaultRoot) {
			if diag.Rule == "inbound-index-link" {
				details = append(details, diag.Detail)
			}
		}
		if logErr != nil || !strings.Contains(string(logData), "[["+page.Address) {
			details = append(details, fmt.Sprintf("wiki/log.md has no entry for %s", page.Address))
		}
	}
	if len(details) > 0 {
		return Check{Name: CheckWikiRegistrationConsistency, Status: CheckFailed, Detail: strings.Join(details, "; ")}
	}
	return Check{Name: CheckWikiRegistrationConsistency, Status: CheckPassed}
}

// checkPrecedenceSidecarConsistency reports every promoted page the
// precedence sidecar (.raw/.longterm-mem-manifest.json) has no entry for at
// all -- a page longterm-mem published without recording that it did.
//
// This is the other half of the failure wiki-registration-consistency
// already half-diagnosed. A promotion writes the page, the sidecar entry,
// the catalog and the log as separate durable steps, and a run killed
// between them used to leave a page nothing recorded -- which every later
// promotion then refused as unknown provenance, suppressing the very Save
// and registration that would have repaired it. Nothing else in doctor
// reads this file, so a vault whose catalog and log had been repaired by
// hand reported entirely healthy while still being permanently wedged.
//
// It reports two states, and deliberately not a third.
//
// A MISSING entry is the first: a page longterm-mem published without
// recording that it did.
//
// A STALE entry that records NO USABLE PROMOTED REVISION is the second, and
// it is the wedged one. Promotion can only attribute a diverged page to one
// of its own interrupted writes when the entry names the revision it
// fingerprinted; an entry that names none carries no such evidence, so the
// page is refused -- and the refusal is a skip, which suppresses the very
// store write that would have given the entry a revision. Every later run
// repeats it. A vault permanently refusing its own page must not report
// entirely healthy: that is the exact failure mode this check exists to
// end, and reporting it is what makes the residue the promotion path
// deliberately leaves behind visible to an operator. "No usable revision"
// is spelled here exactly as promotion's own guard spells it
// (revisionsAllowAdoption: PromotedRevision <= 0), so a sidecar recording a
// negative revision -- equally unadoptable -- is reported rather than left
// wedged-but-silent.
//
// A stale entry that DOES record a positive revision is the third, and is
// not reported. It is an ordinary local edit, which R-030 makes a supported
// and separately reported state: promotion refuses that page, says so with
// its own local-edit-precedence diagnostic, and preserving that edit is the
// point. Note what this deliberately does NOT claim: a later revision does
// not reconcile it. Adoption needs the page's engram_revision to stand
// strictly ABOVE the entry's, and a human's edit leaves it level, so every
// later revision refuses the page too. That is R-030 working, not a defect
// -- the page is held, not lost, and the operator is told on every run --
// but it is a standing refusal, and this check stays quiet about it
// precisely because flagging it would report every page a human has ever
// touched as broken.
func checkPrecedenceSidecarConsistency(vaultRoot string) Check {
	pages, unreadable, err := loadPromotedPages(vaultRoot)
	if err != nil {
		return Check{Name: CheckPrecedenceSidecarConsistency, Status: CheckFailed, Detail: err.Error()}
	}

	store, storeErr := promote.LoadPrecedenceStore(vaultRoot)

	details := append([]string(nil), unreadable...)
	for _, page := range pages {
		if storeErr != nil {
			details = append(details, fmt.Sprintf("%s could not be read, so %s has no provable provenance: %v", precedenceSidecarRelPath, page.Address, storeErr))
			continue
		}
		entry, tracked := store.Get(page.Address)
		switch {
		case !tracked:
			details = append(details, fmt.Sprintf("%s has no entry for %s, so longterm-mem cannot prove it wrote that page", precedenceSidecarRelPath, page.Address))
		case entry.PromotedRevision <= 0 && !entry.MatchesPage(page.Frontmatter):
			details = append(details, fmt.Sprintf("%s records no usable promoted revision (%d) for %s and no longer matches that page, so every promotion of it is refused and nothing in the promotion path can repair the entry", precedenceSidecarRelPath, entry.PromotedRevision, page.Address))
		}
	}
	if len(details) > 0 {
		return Check{Name: CheckPrecedenceSidecarConsistency, Status: CheckFailed, Detail: strings.Join(details, "; ")}
	}
	return Check{Name: CheckPrecedenceSidecarConsistency, Status: CheckPassed}
}

// checkRuntimePrerequisites reports whether requiredPrerequisite (python3,
// D12) is present, rather than letting a later vault subprocess call fail
// with a generic error (R-011 scenario: "Missing runtime prerequisite is
// named").
func checkRuntimePrerequisites(deps DoctorDeps) Check {
	if !deps.PrerequisitePresent(requiredPrerequisite) {
		return Check{Name: CheckRuntimePrerequisites, Status: CheckFailed, Detail: fmt.Sprintf("%s is not present on PATH", requiredPrerequisite)}
	}
	return Check{Name: CheckRuntimePrerequisites, Status: CheckPassed}
}

// checkEmbeddingIndexPresent reports whether project has an embedding index
// at all (R-064 scenario: "Missing index is named, not silently skipped").
// It reports the same failure -- naming the directory doctor looked in --
// whether no index has ever been built (vecindex.ErrNoIndex) or an index
// exists but cannot be trusted (vecindex.ErrCorrupted): a corrupted index is
// exactly as absent from a caller's point of view as no index at all, and
// this is doctor's own read-only diagnostic, never a place that repairs or
// rebuilds one (doctor is read-only by contract).
func checkEmbeddingIndexPresent(deps DoctorDeps, project string) Check {
	dir := vecindex.Dir(deps.StateDir, project)
	if _, err := vecindex.Load(dir); err != nil {
		if errors.Is(err, vecindex.ErrNoIndex) {
			return Check{Name: CheckEmbeddingIndexPresent, Status: CheckFailed, Detail: fmt.Sprintf("no embedding index has been built at %s; run `longterm-mem index --embeddings`", dir)}
		}
		return Check{Name: CheckEmbeddingIndexPresent, Status: CheckFailed, Detail: fmt.Sprintf("embedding index at %s could not be loaded: %v", dir, err)}
	}
	return Check{Name: CheckEmbeddingIndexPresent, Status: CheckPassed}
}

// checkEmbeddingIndexFresh reports how many of project's live rows the
// embedding index does not yet know about (R-064 scenario: "A stale index
// is named with its coverage gap"). It compares deps.LiveObservationIDs
// against the loaded manifest's own entries -- comparing ID sets, not
// counts, so a corpus that both grew and shrank by the same amount is still
// caught, unlike a bare length comparison.
//
// When the index cannot be loaded at all, freshness is reported as its own
// failure naming that it cannot be determined, rather than fabricating
// either a pass or a specific missing-row count doctor has no evidence for
// -- "if it cannot determine freshness, it says so."
func checkEmbeddingIndexFresh(deps DoctorDeps, project string) Check {
	dir := vecindex.Dir(deps.StateDir, project)
	idx, err := vecindex.Load(dir)
	if err != nil {
		return Check{Name: CheckEmbeddingIndexFresh, Status: CheckFailed, Detail: fmt.Sprintf("freshness cannot be determined: embedding index at %s could not be loaded: %v", dir, err)}
	}

	liveIDs, err := deps.LiveObservationIDs(project)
	if err != nil {
		return Check{Name: CheckEmbeddingIndexFresh, Status: CheckFailed, Detail: fmt.Sprintf("freshness cannot be determined: could not list live rows for %s: %v", project, err)}
	}

	indexed := make(map[int64]bool, len(idx.Manifest.Entries))
	for _, e := range idx.Manifest.Entries {
		indexed[e.EngramID] = true
	}
	missing := 0
	for _, id := range liveIDs {
		if !indexed[id] {
			missing++
		}
	}
	if missing > 0 {
		return Check{Name: CheckEmbeddingIndexFresh, Status: CheckFailed, Detail: fmt.Sprintf("embedding index is missing %d live row(s) for %s; run `longterm-mem index --embeddings`", missing, project)}
	}
	return Check{Name: CheckEmbeddingIndexFresh, Status: CheckPassed}
}

// checkEmbeddingBackendReachable reports whether the configured embedding
// backend actually answers with the required model present, distinguishing
// unreachable from model-missing by deps.EmbeddingBackendCheck's typed
// error (R-064 scenario: "An unreachable backend is distinguished from a
// missing model"; R-070's own BackendUnreachableError/ModelMissingError
// split). A check that could not fail would be worse than no check: this
// one fails on either condition, with wording that never collapses the two.
func checkEmbeddingBackendReachable(ctx context.Context, deps DoctorDeps) Check {
	err := deps.EmbeddingBackendCheck(ctx)
	if err == nil {
		return Check{Name: CheckEmbeddingBackendReachable, Status: CheckPassed}
	}

	var modelMissing *embed.ModelMissingError
	if errors.As(err, &modelMissing) {
		return Check{Name: CheckEmbeddingBackendReachable, Status: CheckFailed, Detail: fmt.Sprintf("backend answered but required model is missing: %v", err)}
	}
	return Check{Name: CheckEmbeddingBackendReachable, Status: CheckFailed, Detail: fmt.Sprintf("backend is unreachable: %v", err)}
}

// loadPromotedPages scans vaultRoot's promoted-pages directory
// (wiki/memory) and builds a promote.Page per entry, address and path
// derived from the filename (D7: never from title, so a later retitle
// cannot rename it) and Frontmatter carrying the raw file content --
// sufficient for the address-map and inbound-index-link rules, which only
// read Page.Address and Page.Path. A missing directory (nothing promoted
// yet) returns an empty slice, not an error -- mirroring checkAddressMap's
// own "nothing yet to be inconsistent with" handling of a missing
// manifest.
//
// A single unreadable entry is returned as its own detail line, never as
// an error that ends the scan: aborting here would make every calling
// check report only that one I/O message and discard every other page's
// result, hiding a genuinely unregistered page behind one broken symlink
// on every run. That is the per-item-failure-aborts-the-run shape this
// package's own contract forbids. Only a directory that cannot be listed
// at all -- where there is no per-page result to salvage -- is an error.
func loadPromotedPages(vaultRoot string) (pages []promote.Page, unreadable []string, err error) {
	dir := filepath.Join(vaultRoot, promotedPagesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("ops: list promoted pages in %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		address := strings.TrimSuffix(entry.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			unreadable = append(unreadable, fmt.Sprintf("promoted page %s could not be read: %v", entry.Name(), err))
			continue
		}
		pages = append(pages, promote.Page{
			Address:     address,
			Path:        promotedPagesDir + "/" + entry.Name(),
			Frontmatter: string(data),
		})
	}
	return pages, unreadable, nil
}
