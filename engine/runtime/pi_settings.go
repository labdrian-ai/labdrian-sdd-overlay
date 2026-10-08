package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// piSettings is what the Pi adapter knows of ~/.pi/agent/settings.json after reading it once:
// the packages the `pi` CLI lists there. The adapter never writes the file (only `pi` does); it
// asks this value, however many questions it has, so a lifecycle step reads and parses the file
// one time and every answer comes from the same content.
//
// A missing home, a missing file and content that does not parse all give the value that lists
// nothing: what is not proven is reported as unproven, never as an error.
type piSettings struct {
	// home is the home the file was read under; gentle-pi's own package.json is found under it.
	home     string
	packages []string
}

// piAgentDir is the directory the `pi` CLI keeps its settings in, ~/.pi/agent.
func piAgentDir(home string) string {
	return filepath.Join(home, ".pi", "agent")
}

// readPiSettings reads and parses the settings file of the Pi CLI under home. An empty home
// reads nothing, so the adapter never looks in a location relative to the working directory.
func readPiSettings(home string) piSettings {
	if home == "" {
		return piSettings{}
	}
	raw, err := os.ReadFile(filepath.Join(piAgentDir(home), "settings.json"))
	if err != nil {
		return piSettings{home: home}
	}
	return parsePiSettings(home, raw)
}

// parsePiSettings parses the content of a settings file read under home.
func parsePiSettings(home string, raw []byte) piSettings {
	var doc struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return piSettings{home: home}
	}
	return piSettings{home: home, packages: doc.Packages}
}

// listsPackage reports whether destDir is one of the listed packages. Pi resolves relative
// entries against the settings file's directory (packages.md), and `pi install <abs>` records
// them that way, so they are resolved the same way here instead of compared as strings.
func (s piSettings) listsPackage(destDir string) bool {
	if s.home == "" {
		return false
	}
	want := filepath.Clean(destDir)
	settingsDir := piAgentDir(s.home)
	for _, p := range s.packages {
		if !filepath.IsAbs(p) {
			p = filepath.Join(settingsDir, p)
		}
		if filepath.Clean(p) == want {
			return true
		}
	}
	return false
}

// listsSubagentsExtension reports whether either accepted Subagents extension name is listed
// (it mirrors listsPackage's parse, without the destDir-relative resolution a package path needs).
func (s piSettings) listsSubagentsExtension() bool {
	for _, p := range s.packages {
		if hasAcceptedSubagentsPrefix(p) {
			return true
		}
	}
	return false
}

// nativeSubagents reports whether the packages list "npm:gentle-pi" at a version >=
// minNativeSubagentsVersion -- the version whose native subagent_* tools replace the third-party
// Subagents extension. An unversioned "npm:gentle-pi" entry resolves the installed version from
// gentle-pi's own package.json. Absent, unparseable, or below the minimum version all report
// false.
func (s piSettings) nativeSubagents() bool {
	for _, p := range s.packages {
		if p != gentlePiPackagePrefix && !strings.HasPrefix(p, gentlePiPackagePrefix+"@") {
			continue
		}
		versionStr := strings.TrimPrefix(strings.TrimPrefix(p, gentlePiPackagePrefix), "@")
		if versionStr == "" {
			versionStr = installedGentlePiVersion(s.home)
		}
		if versionStr == "" {
			return false
		}
		v, ok := parsePiVersion(versionStr)
		if !ok {
			return false
		}
		return !v.lessThan(minNativeSubagentsVersion)
	}
	return false
}

// subagentRunner reports which dispatch runner(s) are proven present.
func (s piSettings) subagentRunner() subagentRunner {
	native := s.nativeSubagents()
	legacy := s.listsSubagentsExtension()
	switch {
	case native && legacy:
		return subagentRunnerConflict
	case native:
		return subagentRunnerNative
	case legacy:
		return subagentRunnerLegacy
	default:
		return subagentRunnerAbsent
	}
}

// installedGentlePiVersion reads the installed gentle-pi package's own package.json under
// <home>/.pi/agent/npm/node_modules/gentle-pi (the path verified live 2026-09-13) and returns
// its "version" field, or "" when it cannot be read or parsed. Used only when the settings list
// an unversioned "npm:gentle-pi" entry.
func installedGentlePiVersion(home string) string {
	raw, err := os.ReadFile(filepath.Join(piAgentDir(home), "npm", "node_modules", "gentle-pi", "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return ""
	}
	return pkg.Version
}
