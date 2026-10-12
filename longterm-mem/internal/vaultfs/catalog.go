package vaultfs

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// ReadIndex returns the whole content of the vault's master catalog, byte for byte, reading through a link to
// it. A catalog that is not there is the file system's error for it (errors.Is(err, fs.ErrNotExist)), and any
// other failure is the file system's own: both name the file, and the adapter adds no word of its own to either,
// because the callers word the failure around the file they were reading.
func (v *Vault) ReadIndex() (string, error) { return v.readFile(vaultlayout.IndexFile) }

// IndexLocation is the path of the catalog in this vault, as an operator would open it.
func (v *Vault) IndexLocation() string { return v.layout.Path(vaultlayout.IndexFile) }

// WriteIndex replaces the catalog with content, durably and atomically, creating the wiki directory when the
// vault has none: through a link to the catalog, keeping the mode of the catalog that is there, and creating a
// new one owner-only. Its errors name what failed and carry no consumer's name.
func (v *Vault) WriteIndex(content string) error { return v.writeFile(vaultlayout.IndexFile, content) }

// ReadLog returns the whole content of the vault's promotion log, and fails as ReadIndex does.
func (v *Vault) ReadLog() (string, error) { return v.readFile(vaultlayout.LogFile) }

// LogLocation is the path of the log in this vault, as an operator would open it.
func (v *Vault) LogLocation() string { return v.layout.Path(vaultlayout.LogFile) }

// WriteLog replaces the log with content, on the terms of WriteIndex.
func (v *Vault) WriteLog(content string) error { return v.writeFile(vaultlayout.LogFile, content) }

// readFile is the content of the vault-relative file rel, whole, or the file system's own error.
func (v *Vault) readFile(rel string) (string, error) {
	data, err := os.ReadFile(v.layout.Path(rel))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// writeFile replaces the vault-relative file rel with content the way the module replaces every file of the vault.
func (v *Vault) writeFile(rel, content string) error {
	if err := writeFileAtomic(v.layout.Path(rel), []byte(content)); err != nil {
		return err
	}
	return nil
}
