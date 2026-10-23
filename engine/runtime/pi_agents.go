package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The agent links: the overlay-owned symlinks ~/.pi/agent/agents/<name>.md to the package's own
// agents/<name>.md, one for every agent of the package (GADU and the role agents engine/agentgen
// writes for Pi), how their ownership is proven, and how each is made and removed without touching
// a file that is not the overlay's.

// agentLinkPath returns the overlay-owned link location of an agent file.
func agentLinkPath(home, file string) string {
	return filepath.Join(home, ".pi", "agent", "agents", file)
}

// agentSourcePath returns the package's own agents/<file> -- the STABLE symlink target across
// `swap` rebuilds (only the inode changes, design D10).
func agentSourcePath(destDir, file string) string {
	return filepath.Join(destDir, "agents", file)
}

// packageAgents lists the agent files of the built package: the Markdown files of its agents/
// directory, in the order of their names.
func packageAgents(destDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(destDir, "agents"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// agentLabel names an agent file in the messages about it: its name without the extension, in lower
// case. GADU.md becomes "gadu", which keeps the "gadu link: ..." and "gadu frontmatter: ..." wording
// GADU's messages had before the role agents joined it.
func agentLabel(file string) string {
	return strings.ToLower(strings.TrimSuffix(file, filepath.Ext(file)))
}

// agentLinkStatus is agentLinkState's result, the state of the link and not the link: exactly one
// of missing/current/stale/conflict (D13 -- collapsing these into a single boolean was rejected as
// it hides which failure mode is present).
type agentLinkStatus string

const (
	agentLinkMissing  agentLinkStatus = "missing"
	agentLinkCurrent  agentLinkStatus = "current"
	agentLinkStale    agentLinkStatus = "stale"
	agentLinkConflict agentLinkStatus = "conflict"
)

// agentLinkState reports linkPath's ownership/state relative to
// expectedTarget. Ownership is proven ONLY by os.Readlink equality with
// expectedTarget, never by file contents or a side record: missing (no
// entry), current (our symlink, target resolves), stale (our symlink, but
// its target no longer exists -- e.g. destDir was rebuilt from scratch),
// or conflict (a pre-existing regular file, or a symlink pointing
// elsewhere) -- a conflict is reported and left untouched, never
// overwritten.
func agentLinkState(linkPath, expectedTarget string) agentLinkStatus {
	info, err := os.Lstat(linkPath)
	if err != nil {
		return agentLinkMissing
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return agentLinkConflict
	}
	target, err := os.Readlink(linkPath)
	if err != nil || filepath.Clean(target) != filepath.Clean(expectedTarget) {
		return agentLinkConflict
	}
	if _, err := os.Stat(expectedTarget); err != nil {
		return agentLinkStale
	}
	return agentLinkCurrent
}

// validateAgentFrontmatter mirrors just enough of pi-subagents-j0k3r's own
// frontmatter parser (config.ts parseFrontmatterWithIssues) to catch the
// one failure mode that silently drops the subagent at dispatch time: a
// `tools` field declared BOTH as an inline scalar and a YAML list. `name`
// and `description` are required (the extension falls back to the
// filename/a generic description otherwise, which is not what the agent
// wants); `model` is optional and unconstrained (R-014). The file is read with
// skills.ReadFrontmatter, the reader the skills lint and the package build
// use, so the three agree on where the frontmatter is and which keys are
// at its top level. label names the agent in the refusals ("gadu frontmatter:
// missing required 'name'").
func validateAgentFrontmatter(label, content string) error {
	fm, err := skills.ReadFrontmatter([]byte(content))
	if err != nil {
		// ReadFrontmatter fails only for a missing fence (a *FrontmatterError); anything it
		// might return later is not read as a frontmatter either.
		var fault *skills.FrontmatterError
		if errors.As(err, &fault) && fault.Fault == skills.NoClosingFence {
			return fmt.Errorf("%s frontmatter: missing closing --- delimiter", label)
		}
		return fmt.Errorf("%s frontmatter: missing opening --- delimiter", label)
	}
	var name, description string
	var toolsInline, toolsList bool
	for _, entry := range fm.Entries() {
		switch entry.Key {
		case "name":
			name = entry.Value
		case "description":
			description = entry.Value
		case "tools":
			toolsInline = toolsInline || entry.Value != ""
			toolsList = toolsList || entry.Items > 0
		}
	}
	if name == "" {
		return fmt.Errorf("%s frontmatter: missing required 'name'", label)
	}
	if description == "" {
		return fmt.Errorf("%s frontmatter: missing required 'description'", label)
	}
	if toolsInline && toolsList {
		return fmt.Errorf("%s frontmatter: 'tools' declared both as an inline scalar and a YAML list; choose one", label)
	}
	return nil
}

// agentLinkResult is what became of one agent file of the package when its link was made or
// removed: Err is nil when the link is in place (or gone), and says why it is not otherwise.
type agentLinkResult struct {
	File string
	Err  error
}

// linkPackageAgents links every agent of the package at ~/.pi/agent/agents (R-013) after verifying
// its frontmatter (R-014), and goes on after one it cannot link, so a file of the user at the place
// of one agent never keeps the others from being linked. A pre-existing regular file or a symlink
// pointing elsewhere is a conflict, reported and left untouched -- never overwritten. A stale link
// (ours, but its target no longer exists) is recreated. It returns what became of each agent, in
// the order of their names; the error is for a package whose agents cannot be listed at all.
func linkPackageAgents(home, destDir string) ([]agentLinkResult, error) {
	files, err := packageAgents(destDir)
	if err != nil {
		return nil, fmt.Errorf("agents link: the package agents cannot be read at %s: %w", filepath.Join(destDir, "agents"), err)
	}
	results := make([]agentLinkResult, 0, len(files))
	for _, file := range files {
		results = append(results, agentLinkResult{File: file, Err: linkAgent(home, destDir, file)})
	}
	return results, nil
}

// linkAgent links the one agent file of the package: the rules of linkPackageAgents.
func linkAgent(home, destDir, file string) error {
	label := agentLabel(file)
	target := agentSourcePath(destDir, file)
	content, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("%s link: package agents/%s not found at %s: %w", label, file, target, err)
	}
	if err := validateAgentFrontmatter(label, string(content)); err != nil {
		return err
	}

	linkPath := agentLinkPath(home, file)
	switch agentLinkState(linkPath, target) {
	case agentLinkCurrent:
		return nil
	case agentLinkConflict:
		return fmt.Errorf("%s link: %s exists and is not the overlay-owned symlink; leaving it untouched", label, linkPath)
	case agentLinkStale:
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("%s link: removing stale link: %w", label, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return fmt.Errorf("%s link: creating agents dir: %w", label, err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("%s link: creating symlink: %w", label, err)
	}
	return nil
}

// unlinkPackageAgents removes the link of every agent of the package from ~/.pi/agent/agents ONLY
// when it is the overlay-owned symlink (Readlink equality against destDir's agents/<file>). Never
// touches a conflicting entry or any other file in that directory, and never removes the Subagents
// extension package itself (R-016). The agents are those of the package plus every link of the home
// that still points into the package's agents/ -- so a link whose source is gone (the package's
// agents/ removed, or an agent a rebuild dropped) is removed too. It returns what became of each
// agent, in the order of their names. The error tells a side that cannot be listed -- a package
// agents directory that is there but unreadable, or the links directory of the home -- and comes
// with the results of the other side, so a listing that fails never hides the links still removed.
func unlinkPackageAgents(home, destDir string) ([]agentLinkResult, error) {
	files, packageErr := packageAgents(destDir)
	owned, linksErr := ownedAgentLinks(home, destDir)
	var listing []string
	if packageErr != nil && !errors.Is(packageErr, os.ErrNotExist) {
		listing = append(listing, fmt.Sprintf("the package agents cannot be read at %s: %v", filepath.Join(destDir, "agents"), packageErr))
	}
	if linksErr != nil {
		listing = append(listing, fmt.Sprintf("the agent links cannot be read at %s: %v", agentLinkPath(home, ""), linksErr))
	}
	var err error
	if len(listing) > 0 {
		err = errors.New("agents link: " + strings.Join(listing, "; "))
	}
	for _, file := range owned {
		if !slices.Contains(files, file) {
			files = append(files, file)
		}
	}
	slices.Sort(files)
	results := make([]agentLinkResult, 0, len(files))
	for _, file := range files {
		results = append(results, agentLinkResult{File: file, Err: unlinkAgent(home, destDir, file)})
	}
	return results, err
}

// ownedAgentLinks lists the files of ~/.pi/agent/agents that are the overlay-owned symlink of an
// agent of the package (Readlink equality against destDir's agents/<file>), whether or not that
// agent is still in the package. A home with no agents directory has none.
func ownedAgentLinks(home, destDir string) ([]string, error) {
	entries, err := os.ReadDir(agentLinkPath(home, ""))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		file := entry.Name()
		if entry.Type()&os.ModeSymlink == 0 || filepath.Ext(file) != ".md" {
			continue
		}
		if state := agentLinkState(agentLinkPath(home, file), agentSourcePath(destDir, file)); state != agentLinkConflict && state != agentLinkMissing {
			files = append(files, file)
		}
	}
	return files, nil
}

// unlinkAgent removes the link of the one agent file of the package: the rules of
// unlinkPackageAgents.
func unlinkAgent(home, destDir, file string) error {
	linkPath := agentLinkPath(home, file)
	switch agentLinkState(linkPath, agentSourcePath(destDir, file)) {
	case agentLinkMissing:
		return nil
	case agentLinkConflict:
		return fmt.Errorf("%s link: %s is not the overlay-owned symlink; leaving it untouched", agentLabel(file), linkPath)
	default: // current or stale -- still ours
		return os.Remove(linkPath)
	}
}

// agentsInstallNotes tells, for Install's message, what became of every agent of the package: that
// it is linked, or why it is not.
func agentsInstallNotes(home, destDir string) []string {
	results, err := linkPackageAgents(home, destDir)
	if err != nil {
		return []string{err.Error() + "."}
	}
	notes := make([]string, 0, len(results))
	for _, result := range results {
		if result.Err != nil {
			notes = append(notes, result.Err.Error()+".")
			continue
		}
		notes = append(notes, result.File+" linked at ~/.pi/agent/agents/"+result.File+".")
	}
	return notes
}

// agentsUninstallNote tells, for Uninstall's message, what became of the link of every agent of the
// package: that it is removed, or why it is left. It starts with a space, to follow the sentence
// before it, and is empty when the package has no agent to tell about.
func agentsUninstallNote(home, destDir string) string {
	results, err := unlinkPackageAgents(home, destDir)
	var note strings.Builder
	for _, result := range results {
		if result.Err != nil {
			note.WriteString(" " + result.Err.Error() + ".")
			continue
		}
		note.WriteString(" " + result.File + " link removed.")
	}
	if err != nil {
		note.WriteString(" " + err.Error() + ".")
	}
	return note.String()
}

// agentLinkProblems returns, for Status, one entry for every agent of the package whose link it
// cannot prove, in the form it always used for GADU.md. Nothing is returned for a link that is
// current. With no home to look in, none can be proven.
func agentLinkProblems(home, destDir string) []string {
	files, err := packageAgents(destDir)
	if err != nil {
		return []string{"agents linked at ~/.pi/agent/agents (the package agents cannot be read: " + err.Error() + "; run: labdrian-overlay apply --target pi)"}
	}
	var problems []string
	for _, file := range files {
		entry := file + " linked at ~/.pi/agent/agents/" + file
		if home == "" {
			problems = append(problems, entry+" (cannot resolve home directory)")
			continue
		}
		switch agentLinkState(agentLinkPath(home, file), agentSourcePath(destDir, file)) {
		case agentLinkMissing:
			problems = append(problems, entry+" (missing; run: labdrian-overlay apply --target pi)")
		case agentLinkStale:
			problems = append(problems, entry+" (stale; run: labdrian-overlay apply --target pi)")
		case agentLinkConflict:
			problems = append(problems, entry+" (conflict: a foreign file already exists there)")
		}
	}
	return problems
}
