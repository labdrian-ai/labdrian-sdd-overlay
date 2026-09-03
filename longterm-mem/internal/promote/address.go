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
	Address  string
	Revision int
}

// findPromotedPage scans wiki/memory/*.md for a page whose frontmatter
// already carries this engram_id and project, returning its address and
// the engram_revision it was last promoted at (R-028 re-promotion reuse;
// R-009 Sync's unpromoted-or-revised gate). A missing wiki/memory/
// directory means nothing has been promoted yet. A matched page without a
// usable address is corrupted promotion state and errors -- the same
// discipline as the fresh path's "produced no address" guard -- never an
// empty-string reuse. A matched page whose engram_revision cannot be
// parsed as an integer errors the same way, rather than silently treating
// it as revision 0 and re-promoting content that may already be current.
func findPromotedPage(vaultRoot, project string, engramID int) (promotedPage, bool, error) {
	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return promotedPage{}, false, nil
		}
		return promotedPage{}, false, fmt.Errorf("promote: list %s: %w", memoryDir, err)
	}

	wantID := strconv.Itoa(engramID)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memoryDir, entry.Name()))
		if err != nil {
			return promotedPage{}, false, fmt.Errorf("promote: read %s: %w", entry.Name(), err)
		}
		block, ok := frontmatterBlock(string(data))
		if !ok {
			continue
		}
		fields := parseFrontmatterFields(block)
		if fields["engram_id"] != wantID || fields["project"] != project {
			continue
		}
		address := strings.TrimSpace(fields["address"])
		if address == "" {
			return promotedPage{}, false, fmt.Errorf("promote: promoted page %s matches engram_id %s but carries no address", entry.Name(), wantID)
		}
		revision := 0
		if raw := strings.TrimSpace(fields["engram_revision"]); raw != "" {
			revision, err = strconv.Atoi(raw)
			if err != nil {
				return promotedPage{}, false, fmt.Errorf("promote: promoted page %s carries an unparseable engram_revision %q: %w", entry.Name(), raw, err)
			}
		}
		return promotedPage{Address: address, Revision: revision}, true, nil
	}
	return promotedPage{}, false, nil
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
