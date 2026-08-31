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
	if existing, ok, err := findPromotedAddress(vaultRoot, project, engramID); err != nil {
		return "", err
	} else if ok {
		return existing, nil
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

// findPromotedAddress scans wiki/memory/*.md for a page whose frontmatter
// already carries this engram_id and project, returning its address
// (R-028 re-promotion reuse). A missing wiki/memory/ directory means
// nothing has been promoted yet. A matched page without a usable address
// is corrupted promotion state and errors -- the same discipline as the
// fresh path's "produced no address" guard -- never an empty-string reuse.
func findPromotedAddress(vaultRoot, project string, engramID int) (string, bool, error) {
	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("promote: list %s: %w", memoryDir, err)
	}

	wantID := strconv.Itoa(engramID)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memoryDir, entry.Name()))
		if err != nil {
			return "", false, fmt.Errorf("promote: read %s: %w", entry.Name(), err)
		}
		block, ok := frontmatterBlock(string(data))
		if !ok {
			continue
		}
		fields := parseFrontmatterFields(block)
		if fields["engram_id"] == wantID && fields["project"] == project {
			address := strings.TrimSpace(fields["address"])
			if address == "" {
				return "", false, fmt.Errorf("promote: promoted page %s matches engram_id %s but carries no address", entry.Name(), wantID)
			}
			return address, true, nil
		}
	}
	return "", false, nil
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

// writeFileAtomic writes data to path via tmp+fsync+rename, MkdirAll'ing
// the parent directory first -- vaultreg.writeJSONAtomic's pattern (D6),
// copied here since vaultreg's helper is unexported and address.go/
// register.go live in a different package. Shared by both.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("promote: create directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("promote: create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("promote: write temp file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("promote: fsync temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("promote: close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("promote: rename temp file into %s: %w", path, err)
	}
	return nil
}
