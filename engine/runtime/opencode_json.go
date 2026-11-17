package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// What the OpenCode adapter keeps in <config>/opencode.json (S12), in OpenCode v2's own shape
// (measured in L10 T4, schema read from opencode 2.2.7): the default primary agent, and the MCP
// servers memorycard and codegraph under mcp.servers with Code Mode left on. Every other key of the
// file is the owner's. The plugin is not named in the file: OpenCode v2 loads
// <config>/plugins/<id>/index.js without it. The provider is not either: the ChatGPT subscription
// is the built-in openai provider, logged in with `opencode auth login` (the credentials are never in
// this file), and each agent file names its own model.

const (
	openCodeJSONFile = "opencode.json"
	// openCodeJSONBackupSuffix names the copy of the file as it was before the adapter first changed it.
	openCodeJSONBackupSuffix = ".labdrian.bak"
	// openCodeDefaultAgentKey and openCodeDefaultAgent: the key is owned only while it holds exactly
	// this value, the agent file the overlay installs (ownership by value, as settings.json's agent key).
	openCodeDefaultAgentKey = "default_agent"
	openCodeDefaultAgent    = "foreman"
)

// openCodeMCPServer is one server entry the adapter owns, owned only while it holds exactly these bytes.
type openCodeMCPServer struct {
	name  string
	entry map[string]any
}

func openCodeLocalServer(name string, command ...string) openCodeMCPServer {
	argv := make([]any, len(command))
	for i, word := range command {
		argv[i] = word
	}
	return openCodeMCPServer{name: name, entry: map[string]any{"type": "local", "command": argv, "codemode": true}}
}

// openCodeMCPServers are the servers of the labdrian flow. The commands are bare names, resolved on
// the PATH OpenCode runs with.
var openCodeMCPServers = []openCodeMCPServer{
	openCodeLocalServer("memorycard", "memorycard", "mcp"),
	openCodeLocalServer("codegraph", "codegraph", "serve", "--mcp"),
}

func (s openCodeMCPServer) path() string { return "mcp.servers." + s.name }

// canonical is the value as JSON with sorted keys, to compare entries by content.
func canonical(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

// openCodeJSONState is what the file holds against what the adapter owns.
type openCodeJSONState struct {
	// missing are the owned keys and entries that are absent, by path.
	missing []string
	// foreign are the owned keys and entries that hold another value, as a sentence each.
	foreign []string
}

// errOpenCodeJSONShape is the error of a file that is JSON but not a config the adapter can edit.
var errOpenCodeJSONShape = errors.New("it is not an object, or its mcp.servers is not the v2 container of servers")

// parseOpenCodeJSON decodes the file. An empty file is an empty config; anything that is not one
// JSON object is refused, and the file left as it is.
func parseOpenCodeJSON(content string) (map[string]any, error) {
	if strings.TrimSpace(content) == "" {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err == nil {
		return nil, errors.New("it holds more than one JSON value")
	}
	if root == nil {
		return nil, errOpenCodeJSONShape
	}
	if mcp, present := root["mcp"]; present {
		container, ok := mcp.(map[string]any)
		if !ok {
			return nil, errOpenCodeJSONShape
		}
		if servers, present := container["servers"]; present && !isServersContainer(servers) {
			return nil, errOpenCodeJSONShape
		}
	}
	return root, nil
}

// isServersContainer says whether value is the v2 container of servers, which holds only servers
// (objects), and not an OpenCode 1.x server that is called "servers": one with a `type` or `command`
// that is not an object.
func isServersContainer(value any) bool {
	servers, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"type", "command"} {
		if member, present := servers[key]; present {
			if _, isObject := member.(map[string]any); !isObject {
				return false
			}
		}
	}
	return true
}

// serversOf is root.mcp.servers, or nil when the file has none yet.
func serversOf(root map[string]any) map[string]any {
	mcp, _ := root["mcp"].(map[string]any)
	servers, _ := mcp["servers"].(map[string]any)
	return servers
}

func inspectOpenCodeJSON(root map[string]any) openCodeJSONState {
	var state openCodeJSONState
	switch value, present := root[openCodeDefaultAgentKey]; {
	case !present:
		state.missing = append(state.missing, openCodeDefaultAgentKey)
	case value != openCodeDefaultAgent:
		state.foreign = append(state.foreign, fmt.Sprintf("%s holds %s, so the foreman is not the default agent (set it to %q or remove the key)", openCodeDefaultAgentKey, canonical(value), openCodeDefaultAgent))
	}
	servers := serversOf(root)
	for _, server := range openCodeMCPServers {
		switch value, present := servers[server.name]; {
		case !present:
			state.missing = append(state.missing, server.path())
		case canonical(value) != canonical(server.entry):
			state.foreign = append(state.foreign, fmt.Sprintf("%s holds %s, which is not the entry labdrian writes", server.path(), canonical(value)))
		}
	}
	return state
}

// mergeOpenCodeJSON adds every owned key and entry that is absent and reports whether it changed
// root. A foreign value is never overwritten.
func mergeOpenCodeJSON(root map[string]any) bool {
	changed := false
	if _, present := root[openCodeDefaultAgentKey]; !present {
		root[openCodeDefaultAgentKey] = openCodeDefaultAgent
		changed = true
	}
	for _, server := range openCodeMCPServers {
		if _, present := serversOf(root)[server.name]; present {
			continue
		}
		mcp, _ := root["mcp"].(map[string]any)
		if mcp == nil {
			mcp = map[string]any{}
			root["mcp"] = mcp
		}
		servers, _ := mcp["servers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
			mcp["servers"] = servers
		}
		servers[server.name] = server.entry
		changed = true
	}
	return changed
}

// removeOpenCodeJSON deletes the owned keys and entries that hold exactly what the adapter writes, and
// the containers that are left empty by it, and reports whether root changed.
func removeOpenCodeJSON(root map[string]any) bool {
	changed := false
	if root[openCodeDefaultAgentKey] == openCodeDefaultAgent {
		delete(root, openCodeDefaultAgentKey)
		changed = true
	}
	servers := serversOf(root)
	for _, server := range openCodeMCPServers {
		if value, present := servers[server.name]; present && canonical(value) == canonical(server.entry) {
			delete(servers, server.name)
			changed = true
		}
	}
	if changed && servers != nil && len(servers) == 0 {
		mcp := root["mcp"].(map[string]any)
		delete(mcp, "servers")
		if len(mcp) == 0 {
			delete(root, "mcp")
		}
	}
	return changed
}

// renderOpenCodeJSON is the file as written: keys sorted, two spaces, a final newline, no HTML escaping.
func renderOpenCodeJSON(root map[string]any) (string, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(root); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (a OpenCodeAdapter) openCodeJSONPath() string { return filepath.Join(a.root, openCodeJSONFile) }

// writeOpenCodeJSON makes opencode.json hold the owned keys and entries and says what it did. The
// file is created when absent. A file that is not an object the adapter can edit is refused and left
// as it is. A file that changes first gets a copy of what it held, once.
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
	if !mergeOpenCodeJSON(root) {
		return "opencode.json already current in " + path + notices, nil
	}
	text, err := renderOpenCodeJSON(root)
	if err != nil {
		return "", err
	}
	if file.existed {
		if err := keepOnce(path+openCodeJSONBackupSuffix, file); err != nil {
			return "", err
		}
	}
	if err := writeBlockFile(path, text, file); err != nil {
		return "", err
	}
	return "opencode.json written to " + path + notices, nil
}

// foreignNotices are the sentences about the owned keys that hold another value, ready to append.
func foreignNotices(state openCodeJSONState) string {
	var out strings.Builder
	for _, sentence := range state.foreign {
		out.WriteString("; ")
		out.WriteString(sentence)
	}
	return out.String()
}

// keepOnce writes the file as it was read to path unless path exists: the first copy is the owner's.
func keepOnce(path string, read blockFile) error {
	if _, err := os.Lstat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(read.content), read.mode)
}

// removeFromOpenCodeJSON takes the owned keys and entries out of opencode.json. A file left with
// nothing in it is removed: it was created for them. An absent file is left alone.
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
	if !removeOpenCodeJSON(root) {
		return false, nil
	}
	if len(root) == 0 {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return true, removeBlockFile(path, file)
		}
	}
	text, err := renderOpenCodeJSON(root)
	if err != nil {
		return false, err
	}
	return true, writeBlockFile(path, text, file)
}

// openCodeJSONStatus is the status result for opencode.json when it is not as the adapter keeps it:
// partial, naming the file and what is wrong. ok is false when the file is current.
func (a OpenCodeAdapter) openCodeJSONStatus() (message string, ok bool) {
	path := a.openCodeJSONPath()
	file, err := readBlockFile(path)
	if err != nil {
		return "OpenCode config could not be read: " + err.Error(), true
	}
	root, err := parseOpenCodeJSON(file.content)
	if err != nil {
		return fmt.Sprintf("OpenCode config %s cannot be read as a v2 config: %v", path, err), true
	}
	state := inspectOpenCodeJSON(root)
	if len(state.foreign) > 0 {
		return "OpenCode config " + path + " holds values labdrian does not own" + foreignNotices(state), true
	}
	if len(state.missing) > 0 {
		return "OpenCode config " + path + " lacks " + strings.Join(state.missing, ", ") + "; run 'engine runtime install --target opencode'", true
	}
	return "", false
}
