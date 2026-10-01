package archguard

import "strings"

// pure reports whether packages of the ring are held to the pure-standard-library
// rule.
func (r Ring) pure() bool { return r == Domain || r == Application }

// importable lists, for each ring, the rings inside the module that its packages
// may import.
var importable = map[Ring][]Ring{
	Domain:      {Domain},
	Application: {Domain, Application},
	Adapter:     {Domain, Application, Adapter},
	Root:        {Domain, Application, Adapter},
	Support:     {Domain, Application, Adapter, Root, Support},
}

func (r Ring) mayImport(target Ring) bool {
	for _, allowed := range importable[r] {
		if allowed == target {
			return true
		}
	}
	return false
}

// describeImportable names what a ring may import, for a failure message.
func (r Ring) describeImportable() string {
	names := make([]string, 0, len(importable[r]))
	for _, allowed := range importable[r] {
		names = append(names, allowed.String())
	}
	return strings.Join(names, " and ")
}

// pureStd is the standard library a domain or application package may import. It
// is a list to widen on purpose, never by accident: anything not here (os,
// os/exec, syscall, net and net/http, crypto/rand, math/rand, log, flag, runtime,
// embed, unsafe, the go/* parsers) acts on the process or its surroundings.
// Packages that are pure to import but not to use have their ambient members
// listed in ambientMembers.
var pureStd = map[string]bool{
	"bufio": true, "bytes": true, "cmp": true, "container/list": true, "context": true,
	"crypto/sha256": true, "crypto/sha512": true,
	"encoding/base64": true, "encoding/binary": true, "encoding/hex": true, "encoding/json": true,
	"errors": true, "fmt": true, "hash": true, "hash/fnv": true,
	"io": true, "io/fs": true, "maps": true, "math": true, "math/bits": true,
	"net/url": true, "path": true, "path/filepath": true, "reflect": true, "regexp": true,
	"slices": true, "sort": true, "strconv": true, "strings": true,
	"sync": true, "sync/atomic": true, "time": true,
	"unicode": true, "unicode/utf16": true, "unicode/utf8": true,
}

// ambientMembers lists, for the standard packages that are pure to import, the
// members that are not pure to use.
var ambientMembers = []struct {
	pkg   string
	why   string
	names []string
}{
	{"time", "reads the wall clock or sleeps; take the time as a value or inject a clock",
		[]string{"Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick", "NewTicker", "NewTimer"}},
	{"path/filepath", "reaches the file system; walking and resolving paths belongs in an adapter",
		[]string{"Abs", "EvalSymlinks", "Glob", "Walk", "WalkDir"}},
	{"io/fs", "reads through a file system; a port, not the domain, walks one",
		[]string{"Glob", "ReadDir", "ReadFile", "Stat", "WalkDir"}},
	{"fmt", "uses the process's standard streams; take an io.Writer or io.Reader",
		[]string{"Print", "Printf", "Println", "Scan", "Scanf", "Scanln"}},
}

// ambientReason returns why pkg.member is not pure, or "" when it is.
func ambientReason(pkg, member string) string {
	for _, group := range ambientMembers {
		if group.pkg != pkg {
			continue
		}
		for _, name := range group.names {
			if name == member {
				return group.why
			}
		}
	}
	return ""
}

func isAmbientPackage(pkg string) bool {
	for _, group := range ambientMembers {
		if group.pkg == pkg {
			return true
		}
	}
	return false
}

// isStandard reports whether an import path belongs to the standard library: its
// first element has no dot, which is how the go tool tells them apart.
func isStandard(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
