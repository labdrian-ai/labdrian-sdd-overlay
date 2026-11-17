package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a OpenCodeAdapter) openCodeJSONPath() string { return filepath.Join(a.root, openCodeJSONFile) }

// writeOpenCodeJSON makes opencode.json hold the owned keys and entries and says what it did. Only
// the missing members are spliced into the owner's bytes; every other byte stays. The file is created
// when absent. A file that is not an object the adapter can edit is refused and left as it is. A file
// that changes first gets a copy of what it held, once.
func (a OpenCodeAdapter) writeOpenCodeJSON() (message string, err error) {
	path := a.openCodeJSONPath()
	file, err := readBlockFile(path)
	if err != nil {
		return "", err
	}
	root, err := parseOpenCodeJSON(file.content)
	if err != nil {
		return "", fmt.Errorf("%s cannot be edited: %w", path, err)
	}
	notices := foreignNotices(inspectOpenCodeJSON(root))
	raw := []byte(file.content)
	if strings.TrimSpace(file.content) == "" {
		raw = []byte("{}\n")
	}
	changed := false
	if _, present := root[openCodeDefaultAgentKey]; !present {
		if raw, err = spliceOpenCodeMember(raw, nil, openCodeDefaultAgentKey, openCodeDefaultAgent); err != nil {
			return "", fmt.Errorf("%s cannot be edited: %w", path, err)
		}
		changed = true
	}
	for _, server := range openCodeMCPServers {
		if _, present := serversOf(root)[server.name]; present {
			continue
		}
		if raw, err = spliceOpenCodeMember(raw, []string{"mcp", "servers"}, server.name, server.entry); err != nil {
			return "", fmt.Errorf("%s cannot be edited: %w", path, err)
		}
		changed = true
	}
	if !changed {
		return "opencode.json already current in " + path + notices, nil
	}
	if file.existed {
		if err := keepOnce(path+openCodeJSONBackupSuffix, file); err != nil {
			return "", err
		}
	}
	if err := writeBlockFile(path, string(raw), file); err != nil {
		return "", err
	}
	return "opencode.json written to " + path + notices, nil
}

// removeFromOpenCodeJSON cuts the owned keys and entries out of the owner's bytes. The containers it
// leaves empty are cut too, and a file left with nothing in it is removed: it was created for them.
// An absent file is left alone.
func (a OpenCodeAdapter) removeFromOpenCodeJSON() (removed bool, err error) {
	path := a.openCodeJSONPath()
	file, err := readBlockFile(path)
	if err != nil || !file.existed {
		return false, err
	}
	root, err := parseOpenCodeJSON(file.content)
	if err != nil {
		return false, fmt.Errorf("%s cannot be edited: %w", path, err)
	}
	raw := []byte(file.content)
	if root[openCodeDefaultAgentKey] == openCodeDefaultAgent {
		raw, _, err = removeOpenCodeMember(raw, nil, openCodeDefaultAgentKey)
		if err != nil {
			return false, fmt.Errorf("%s cannot be edited: %w", path, err)
		}
		removed = true
	}
	removedServer := false
	for _, server := range openCodeMCPServers {
		if value, present := serversOf(root)[server.name]; !present || canonical(value) != canonical(server.entry) {
			continue
		}
		if raw, _, err = removeOpenCodeMember(raw, []string{"mcp", "servers"}, server.name); err != nil {
			return false, fmt.Errorf("%s cannot be edited: %w", path, err)
		}
		removed, removedServer = true, true
	}
	if !removed {
		return false, nil
	}
	if removedServer {
		for _, container := range [][]string{{"mcp", "servers"}, {"mcp"}} {
			if raw, err = pruneEmptyObject(raw, container); err != nil {
				return false, fmt.Errorf("%s cannot be edited: %w", path, err)
			}
		}
	}
	if object, err := rootObject(raw); err == nil && len(object.members) == 0 {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return true, removeBlockFile(path, file)
		}
	}
	return true, writeBlockFile(path, string(raw), file)
}
