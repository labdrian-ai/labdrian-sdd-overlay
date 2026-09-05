package promote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// allocateScript is the vault-relative address allocator entrypoint (D7): a
// real shell entrypoint (shebang + exec bit), so Allocate execs it directly
// via Runner.Run, matching setup-retrieve.sh's convention (3a.4), never
// RunInterpreted.
const allocateScript = "scripts/allocate-address.sh"

// allocateTimeout bounds a single allocate-address.sh call (D8's
// convention for vault subprocess calls).
const allocateTimeout = 10 * time.Second

// manifestRelPath is .raw/.manifest.json's vault-relative location (D6/D7):
// the wiki-ingest-owned address and source manifest.
const manifestRelPath = ".raw/.manifest.json"

// Allocate returns the vault address for the Engram observation identified
// by (project, engramID) under vaultRoot (R-028). When a page already
// promoted under wiki/memory/ carries this engram_id and project in its
// frontmatter, that page's own address is reused: no subprocess is
// invoked and .raw/.manifest.json is not written again. Otherwise a fresh
// address is allocated via scripts/allocate-address.sh (flock-safe, via
// internal/vault.Runner) and recorded in .raw/.manifest.json's
// address_map, keyed by the page's address-derived path.
func Allocate(vaultRoot, project string, engramID int) (string, error) {
	if existing, ok, err := findPromotedPage(vaultRoot, project, engramID); err != nil {
		return "", err
	} else if ok {
		return existing.Address, nil
	}

	runner := &vault.Runner{Root: vaultRoot}
	ctx, cancel := context.WithTimeout(context.Background(), allocateTimeout)
	defer cancel()

	stdout, stderr, exitCode, err := runner.Run(ctx, allocateScript)
	if err != nil {
		return "", fmt.Errorf("promote: allocate address: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("promote: %s exited %d: %s", allocateScript, exitCode, strings.TrimSpace(string(stderr)))
	}
	address := strings.TrimSpace(string(stdout))
	if address == "" {
		return "", fmt.Errorf("promote: %s produced no address", allocateScript)
	}

	path := pagePathPrefix + "/" + address + ".md"
	if err := recordAddress(vaultRoot, path, address); err != nil {
		return "", err
	}
	return address, nil
}

// promotedPage is one already-promoted page's address and the Engram
// revision it was last promoted at (7a REFACTOR): Allocate's re-promotion
// address reuse (R-028) and Sync's unpromoted-or-revised gate (R-009)
// share this one wiki/memory/ scan instead of each re-implementing it.
type promotedPage struct {
	Address string
	// Revision is the engram_revision the page was last promoted at. It is
	// meaningful for the page findPromotedPage returns; findMovedPages
	// leaves it zero, since superseding a moved page patches status and
	// related only and never consults a revision.
	Revision int
	// Project is the project frontmatter value the page was promoted
	// under. It is carried rather than merely matched on because the
	// address lookup has three outcomes, not two: found under this
	// project, found under a DIFFERENT one (the observation moved between
	// Engram projects), and not promoted at all.
	Project string
}

// findPromotedPage reports the page whose frontmatter already carries this
// engram_id AND this project, with the engram_revision it was last promoted
// at (R-028 re-promotion reuse; R-009 Sync's unpromoted-or-revised gate).
// A page carrying this engram_id under a DIFFERENT project is deliberately
// NOT a match here -- see findMovedPages for that outcome and why the
// project stays part of this key.
//
// Corrupted promotion state fails closed, but only for the project that
// owns the page: a matched page with no usable address, or an unparseable
// engram_revision, errors here exactly as it always has. Widening that to
// every page sharing the engram_id was tried and reverted. This lookup is
// Sync's R-009 gate (sync.go) and Propagate's page lookup as well as
// Allocate's reuse, so one corrupted page under project A would have
// permanently failed project B's promotion of the one observation sharing
// that engram_id -- nothing writes an address into that page, so no run
// could ever clear it. Sync and Propagate record such an error per
// observation and continue, so the rest of project B's run is unaffected;
// the earlier wording here claimed the whole run wedged, which overstates
// it. One permanently unpromotable observation, on account of a page in a
// project it has nothing to do with, is reason enough. The page is a real
// defect; doctor is what names it, for the project it belongs to.
func findPromotedPage(vaultRoot, project string, engramID int) (promotedPage, bool, error) {
	matches, err := scanPromotedPages(vaultRoot, engramID)
	if err != nil {
		return promotedPage{}, false, err
	}
	for _, match := range matches {
		if match.Project != project {
			continue
		}
		page, err := match.resolve()
		if err != nil {
			return promotedPage{}, false, err
		}
		return page, true, nil
	}
	return promotedPage{}, false, nil
}

// findMovedPages returns every already-promoted page carrying this
// engram_id under a project OTHER than project: the pages an observation
// left behind when it moved between Engram projects (an ordinary Engram
// operation -- projects can be merged).
//
// This is the third outcome findPromotedPage deliberately does not have.
// The alternative -- dropping the project comparison from the lookup
// entirely and rewriting project: in place on the old page -- was rejected:
// the move is real history worth keeping, and the address would stop
// implying a stable project while Sync's R-009 gate shares this same
// lookup. So the page is recognised by engram_id, the observation is
// promoted to a fresh address under its new project, and the page it left
// is marked superseded pointing at that new address (Writer.supersedeMoved).
//
// The key is engram_id alone; project only decides which side of the split
// a matched page falls on. Two different observations that merely share a
// project never see each other here.
//
// A matched page under another project with no usable address is skipped
// rather than reported: there is no file for the successor's pointer to
// name, and failing here would let a corrupted page under project A wedge
// every promotion under project B -- the same widening findPromotedPage
// refuses. Revision is not read for a moved page (supersession patches
// status and related only), so an unparseable engram_revision on one is
// not consulted either; that corruption still fails closed for the project
// that owns the page.
func findMovedPages(vaultRoot, project string, engramID int) ([]promotedPage, error) {
	matches, err := scanPromotedPages(vaultRoot, engramID)
	if err != nil {
		return nil, err
	}
	var moved []promotedPage
	for _, match := range matches {
		if match.Project == project || match.Address == "" {
			continue
		}
		moved = append(moved, promotedPage{Address: match.Address, Project: match.Project})
	}
	return moved, nil
}

// promotedPageMatch is one wiki/memory/ page whose frontmatter carries the
// scanned engram_id, with its fields still unvalidated. Validation is
// deliberately NOT part of the scan: a page's corruption may only fail the
// lookup that actually selects it, and the two lookups select on project
// and need different fields (see findPromotedPage and findMovedPages).
type promotedPageMatch struct {
	// File is the page's base filename, named in every error so a human
	// knows which page to repair.
	File string
	// EngramID is the engram_id both the page and the scan carry, kept so
	// an error names what the page matched on.
	EngramID string
	// Project is the page's project frontmatter value, verbatim.
	Project string
	// Address is its address frontmatter value, trimmed; empty when the
	// page carries none.
	Address string
	// RawRevision is its engram_revision frontmatter value, trimmed and
	// unparsed; empty when the page carries none.
	RawRevision string
}

// resolve validates a match into a usable promotedPage. A matched page
// without an address is corrupted promotion state and errors -- the same
// discipline as the fresh path's "produced no address" guard -- never an
// empty-string reuse. A page whose engram_revision cannot be parsed as an
// integer errors the same way, rather than silently reading as revision 0
// and letting Sync skip content that is not actually current. A page with
// no engram_revision field at all is revision 0, which is "never recorded",
// not corruption.
func (m promotedPageMatch) resolve() (promotedPage, error) {
	if m.Address == "" {
		return promotedPage{}, fmt.Errorf("promote: promoted page %s matches engram_id %s but carries no address", m.File, m.EngramID)
	}
	revision := 0
	if m.RawRevision != "" {
		parsed, err := strconv.Atoi(m.RawRevision)
		if err != nil {
			return promotedPage{}, fmt.Errorf("promote: promoted page %s carries an unparseable engram_revision %q: %w", m.File, m.RawRevision, err)
		}
		revision = parsed
	}
	return promotedPage{Address: m.Address, Revision: revision, Project: m.Project}, nil
}

// scanPromotedPages walks wiki/memory/*.md once and returns every page
// whose frontmatter carries engramID, in directory order, whatever project
// each was promoted under. A missing wiki/memory/ directory means nothing
// has been promoted yet. Only I/O failures error here; a matched page's own
// corruption is the caller's to judge, because it may only fail the lookup
// that selects that page.
func scanPromotedPages(vaultRoot string, engramID int) ([]promotedPageMatch, error) {
	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("promote: list %s: %w", memoryDir, err)
	}

	wantID := strconv.Itoa(engramID)
	var found []promotedPageMatch
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memoryDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("promote: read %s: %w", entry.Name(), err)
		}
		block, ok := frontmatterBlock(string(data))
		if !ok {
			continue
		}
		fields := parseFrontmatterFields(block)
		if fields[engramIDField] != wantID {
			continue
		}
		found = append(found, promotedPageMatch{
			File:        entry.Name(),
			EngramID:    wantID,
			Project:     fields[projectField],
			Address:     strings.TrimSpace(fields["address"]),
			RawRevision: strings.TrimSpace(fields[engramRevisionField]),
		})
	}
	return found, nil
}

// frontmatterBlock isolates the leading `---\n...\n---\n` frontmatter block
// from a full page's raw content, so parseFrontmatterFields (lint.go)
// never sees body prose that could collide with a field name.
func frontmatterBlock(raw string) (string, bool) {
	const delim = "---\n"
	parts := strings.SplitN(raw, delim, 3)
	if len(parts) < 3 {
		return "", false
	}
	return delim + parts[1] + delim, true
}

// recordAddress decodes .raw/.manifest.json as an open key set -- the
// file is wiki-ingest-owned (D7): fields this package does not know must
// survive, and keys absent from the live file must never be fabricated --
// mutates only address_map[path] = address, and re-encodes at 2-space
// indent atomically. Only a wholly absent file starts from the minimal
// seed manifest.
func recordAddress(vaultRoot, path, address string) error {
	full := filepath.Join(vaultRoot, manifestRelPath)
	m := map[string]json.RawMessage{}

	if data, err := os.ReadFile(full); err == nil {
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("promote: parse %s: %w", full, err)
		}
	} else if os.IsNotExist(err) {
		m["version"] = json.RawMessage("1")
		m["created"] = json.RawMessage(strconv.Quote(nowFunc().Format("2006-01-02")))
		m["sources"] = json.RawMessage("{}")
	} else {
		return fmt.Errorf("promote: read %s: %w", full, err)
	}

	addressMap := map[string]string{}
	if raw, ok := m["address_map"]; ok {
		if err := json.Unmarshal(raw, &addressMap); err != nil {
			return fmt.Errorf("promote: parse %s address_map: %w", full, err)
		}
	}
	addressMap[path] = address
	encoded, err := json.Marshal(addressMap)
	if err != nil {
		return fmt.Errorf("promote: marshal %s address_map: %w", full, err)
	}
	m["address_map"] = encoded

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("promote: marshal %s: %w", full, err)
	}
	return writeFileAtomic(full, append(data, '\n'))
}

// pageCreatePerm is the mode a page or sidecar gets when promote is the one
// creating it. Promoted pages carry memory content, so a file this module
// brings into existence starts owner-only. It applies to creation only: a
// page that already exists keeps whatever mode its owner gave it.
const pageCreatePerm = 0o600

// writeFileAtomic durably replaces path with data, MkdirAll'ing the parent
// directory first.
//
// Everything it touches lives inside the USER'S Obsidian vault --
// wiki/memory pages, index.md, log.md, .raw/.manifest.json -- not inside
// longterm-mem's own state directory, and several callers rewrite a page
// that already exists (PatchStatusFields patches frontmatter in place, the
// update path republishes). So this is not a writer of module-owned files
// that can pick its own permissions: a vault page is routinely 0o644,
// routinely tracked by git, and in a synced or dotfiles-style vault
// routinely reached through a symlink.
//
// It used to hand-roll tmp+fsync+rename, which made every page it touched
// come back as a fresh os.CreateTemp inode: 0o644 silently became 0o600,
// and a symlinked page was replaced by a regular file, leaving the user's
// real note unedited while promote reported success. durable.WriteFile
// resolves the link and carries the mode across; see its doc comment for
// the one identity property (hardlinks) deliberately traded for atomicity.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("promote: create directory for %s: %w", path, err)
	}
	if err := durable.WriteFile(path, data, pageCreatePerm); err != nil {
		return fmt.Errorf("promote: %w", err)
	}
	return nil
}
