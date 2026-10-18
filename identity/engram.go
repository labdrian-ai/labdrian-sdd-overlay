package identity

import "strings"

// EngramName is the name Engram gives the project whose shared identity is project: Engram names a
// project by its repository, which is the last segment of the path its origin remote names, and,
// where there is no remote, the name of the directory, which the shared identity already is. So it
// is the last segment of project, and project itself when it has no slash. The case is kept.
//
// It exists for the memory gate's Engram project check (C23, decision 3 of M2; wired by M4), which
// compares the project an Engram read names with EngramName of the workflow's identity by exact
// equality. Nothing calls it yet.
func EngramName(project string) string {
	if i := strings.LastIndex(project, "/"); i >= 0 {
		return project[i+1:]
	}
	return project
}
