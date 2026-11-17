package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a OpenCodeAdapter) openCodeJSONPath() string { return filepath.Join(a.root, openCodeJSONFile) }

// serversToWrite are the MCP servers with the absolute path of their program when the adapter's
// lookup finds one, and a sentence for each program it did not find (the bare name is written then).
func (a OpenCodeAdapter) serversToWrite() (servers []openCodeMCPServer, notes []string) {
	for _, server := range openCodeMCPServers {
		if a.options.PathLookup != nil {
			if program, err := a.options.PathLookup(server.command[0]); err == nil && filepath.IsAbs(program) {
				servers = append(servers, server.resolved(program))
				continue
			}
			notes = append(notes, server.command[0]+" not found on PATH, so its bare name is written and OpenCode must find it on its own PATH")
		}
		servers = append(servers, server)
	}
	return servers, notes
}

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
	if isBlankJSON(file.content) {
		raw = []byte("{}\n")
	}
	changed := false
	if _, present := root[openCodeDefaultAgentKey]; !present {
		if raw, err = spliceOpenCodeMember(raw, nil, openCodeDefaultAgentKey, openCodeDefaultAgent); err != nil {
			return "", fmt.Errorf("%s cannot be edited: %w", path, err)
		}
		changed = true
	}
	servers, notes := a.serversToWrite()
	for _, server := range servers {
		if current, present := serversOf(root)[server.name]; present {
			if !bareEntryToUpgrade(current, server) {
				continue
			}
			if raw, err = replaceOpenCodeMember(raw, []string{"mcp", "servers"}, server.name, server.entry); err != nil {
				return "", fmt.Errorf("%s cannot be edited: %w", path, err)
			}
			changed = true
			notices += "; " + server.name + " upgraded to " + server.command[0]
			continue
		}
		if raw, err = spliceOpenCodeMember(raw, []string{"mcp", "servers"}, server.name, server.entry); err != nil {
			return "", fmt.Errorf("%s cannot be edited: %w", path, err)
		}
		changed = true
		if note := noteFor(notes, server.name); note != "" {
			notices += "; " + note
		}
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

func noteFor(notes []string, name string) string {
	for _, note := range notes {
		if strings.HasPrefix(note, name+" ") {
			return note
		}
	}
	return ""
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
		if value, present := serversOf(root)[server.name]; !present || !server.owns(value) {
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

// bareEntryToUpgrade says whether current is our entry written with the bare program name while the
// server to write has found the program: it is then replaced by the absolute path. An absolute entry of
// ours, even for another path, stays: it is the owner's choice of where the program is.
func bareEntryToUpgrade(current any, server openCodeMCPServer) bool {
	if server.command[0] == server.name {
		return false
	}
	for _, bare := range openCodeMCPServers {
		if bare.name == server.name && !bare.owns(current) {
			return false
		}
	}
	entry, _ := current.(map[string]any)
	command, _ := entry["command"].([]any)
	return len(command) > 0 && command[0] == server.name
}
