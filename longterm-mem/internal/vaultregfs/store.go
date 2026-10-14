// Package vaultregfs is the adapter that keeps the registry of vaults (internal/vaultreg) in a JSON file on a file
// system, ~/.labdrian-overlay/vaults.json by default. It answers the Store port vaultreg owns, in vaultreg's own
// types, and the composition root hands it to the use case that finds a project's vault; nothing else names it.
//
// The file is the module's own state: a document with a schema number and one row per project. A reader tolerates
// what it does not know (other fields, another schema), because the rows are all the registry means; the writer
// writes the schema it knows, through durable.WriteJSON.
//
// The words of its errors begin with the name of the registry's package, "vaultreg:", as they did when the file was
// read there: they are what an operator reads when the registry is not what it should be.
package vaultregfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// schemaVersion is the schema version of the vaults.json the store writes (D5).
const schemaVersion = 1

// createPerm is the mode the registry gets when the store creates it: per-user state, owner-only. It applies to
// creation only; a registry that exists keeps the mode its owner gave it.
const createPerm = 0o600

// Store is the registry kept in the file at Path.
type Store struct {
	// Path is the registry file.
	Path string
}

var _ vaultreg.Store = Store{}

// document is the registry as the file holds it.
type document struct {
	Schema int            `json:"schema"`
	Vaults map[string]row `json:"vaults"`
}

// row is one project's row as the file holds it.
type row struct {
	Path   string `json:"path"`
	Seeded bool   `json:"seeded,omitempty"`
}

// Exists says whether there is a file at Path. A file that cannot be looked at for any reason but that nothing is
// there is a failure, not an absence: taking it for one would have the registry seeded over whatever is in the way.
func (s Store) Exists() (bool, error) {
	_, err := os.Stat(s.Path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("vaultreg: stat %s: %w", s.Path, err)
	}
}

// Load reads the registry at Path. A registry that is not there is an error that wraps fs.ErrNotExist; a file that
// is not a registry is one that names the file and says it could not be parsed. A registry with no rows is an empty
// one.
func (s Store) Load() (vaultreg.Registry, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return vaultreg.Registry{}, fmt.Errorf("vaultreg: read %s: %w", s.Path, err)
	}

	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return vaultreg.Registry{}, fmt.Errorf("vaultreg: parse %s: %w", s.Path, err)
	}
	reg := vaultreg.Registry{Vaults: make(map[string]vaultreg.Entry, len(doc.Vaults)), Origin: s.Path}
	for project, r := range doc.Vaults {
		reg.Vaults[project] = vaultreg.Entry{Path: r.Path, Seeded: r.Seeded}
	}
	return reg, nil
}

// Save writes the registry to Path, replacing what is there atomically (durable.WriteJSON), so that a reader never
// observes a partially written one. Where the registry came from (its Origin) is not part of it.
func (s Store) Save(reg vaultreg.Registry) error {
	doc := document{Schema: schemaVersion, Vaults: make(map[string]row, len(reg.Vaults))}
	for project, entry := range reg.Vaults {
		doc.Vaults[project] = row{Path: entry.Path, Seeded: entry.Seeded}
	}
	if err := durable.WriteJSON(s.Path, doc, createPerm); err != nil {
		return fmt.Errorf("vaultreg: %w", err)
	}
	return nil
}
