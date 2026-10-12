package skillstalefs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
)

// PathCommands finds commands the way a shell finds them: by looking for a file of that name in each directory of
// a PATH value (the directories joined by the platform's list separator; an empty entry is the working
// directory). It looks, and does not run anything.
type PathCommands struct {
	// PathEnv is the PATH to search. It is given, never read from the environment here.
	PathEnv string
}

var _ skillstale.CommandResolver = PathCommands{}

// ResolvesCommand reports whether a file (not a directory) named command is in a directory of the PATH. An empty
// command resolves: there is nothing to look for.
func (p PathCommands) ResolvesCommand(command string) bool {
	if command == "" {
		return true
	}
	for _, dir := range strings.Split(p.PathEnv, string(os.PathListSeparator)) {
		if dir == "" {
			dir = "."
		}
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(command)))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
