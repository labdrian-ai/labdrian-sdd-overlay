package registryyaml

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// decoder reads the registry of one version of the file from the tokens of the whole file. What
// it returns with an error is the entries it had read whole before the fault (see Decode).
type decoder func(tokens []tok) (skills.Registry, error)

// decoders are the versions of the file this package reads, each with the decoder that reads it
// (schemas has the fields of each). A change to the format that a reader of the version before it
// would misread is a new version: a new decoder is added here and the old ones stay, so that a file
// of any version this package has ever written is read as it was written.
var decoders = map[string]decoder{"1": decodeV1}

// Decode reads a registry from the YAML file r, which is a strict subset of YAML. It rejects every
// construct outside the documented subset with a line-numbered error, and reads the file by its
// version: the version is found first, a version no decoder is for is refused naming it, and the
// decoder of the version reads the rest (see the top of the package for what each leaves out and
// what it refuses).
//
// It decodes and does not judge: whether the registry may hold what it holds is the domain's rule
// (skills.Registry.Validate), which the domain applies to what this returns. So that the domain can
// name the first fault in the order the file says it (as the file has always been read, entry by
// entry, with each judged as soon as it was whole), a decoder that fails after some entries are
// complete returns them with the error: the entries whole and in order up to the fault. A fault
// of the text itself, before anything is read (a tab, a flow sequence, a line longer than the
// reader's buffer), and a fault of the version, which comes before any entry, return the zero
// registry.
// minimal: forced — ADR-1 (zero-dep invariant)
func Decode(r io.Reader) (skills.Registry, error) { return decodeWith(decoders, r) }

// decodeWith is Decode over the versions it is given.
func decodeWith(versions map[string]decoder, r io.Reader) (skills.Registry, error) {
	tokens, err := tokenize(r)
	if err != nil {
		return skills.Registry{}, err
	}
	version, err := versionOf(tokens)
	if err != nil {
		return skills.Registry{}, err
	}
	decode, ok := versions[version]
	if !ok {
		return skills.Registry{}, unsupported(version, versions)
	}
	reg, err := decode(tokens)
	reg.Version = version
	return reg, err
}

// versionOf finds the version the file says it is in. It is the value of the top-level key
// "version", and, as the strict reader had it, the last one if the file says it more than once.
// It is a field that every version has and no version reads differently, which is what lets it
// be read before a decoder is chosen: as a field of the must-understand set, a version that has
// a block under it is refused, and a file that says none is refused for that.
func versionOf(tokens []tok) (string, error) {
	version, found := "", false
	for i, t := range tokens {
		if t.indent != 0 || t.key != "version" || (t.kind != tokKeyValue && t.kind != tokKeyOnly) {
			continue
		}
		if i+1 < len(tokens) && tokens[i+1].indent > t.indent {
			return "", fmt.Errorf("line %d: %s", t.lineNum, versionField.refusal(nil))
		}
		version, found = t.val, true
	}
	if !found {
		if t, ok := keyWithAByteOrderMark(tokens, versionField.path); ok {
			return "", fmt.Errorf("line %d: the top-level key %q starts with a byte-order mark; save the file without one", t.lineNum, t.key)
		}
		return "", fmt.Errorf("skills: missing required top-level field 'version'")
	}
	return version, nil
}

// keyWithAByteOrderMark finds the top-level key that is name with a byte-order mark before it, as
// an editor that saves the file with one makes of the first key: it is not the key the format
// asks for, and naming it as it is (with the mark, which %q shows) is the one clue that points at
// the encoding of the file and not at a field that is missing.
func keyWithAByteOrderMark(tokens []tok, name string) (tok, bool) {
	for _, t := range tokens {
		if t.indent == 0 && strings.TrimPrefix(t.key, byteOrderMark) == name && t.key != name {
			return t, true
		}
	}
	return tok{}, false
}

// byteOrderMark is U+FEFF, the mark an editor writes at the start of a file saved with one.
const byteOrderMark = "\ufeff"

// unsupported is the refusal of a version no decoder is for, naming it and the ones there are.
func unsupported(version string, versions map[string]decoder) error {
	known := make([]string, 0, len(versions))
	for v := range versions {
		known = append(known, v)
	}
	sort.Strings(known)
	for i, v := range known {
		known[i] = strconv.Quote(v)
	}
	which := "the valid versions are " + strings.Join(known, ", ")
	if len(known) == 1 {
		which = "only version " + known[0] + " is valid"
	}
	return fmt.Errorf("skills: version %q is not supported; %s", version, which)
}
