package registryyaml

import (
	"fmt"
	"sort"
	"strings"
)

// shape is the form a field takes in the file.
type shape int

const (
	shapeScalar  shape = iota // "key: value", or "key:" with nothing under it
	shapeBlock                // "key:" with a mapping under it
	shapeList                 // "key:" with "- value" lines under it
	shapeEntries              // "key:" with "- key: value" items under it
)

// noun is the shape as a note says it of a field that was not in it.
func (s shape) noun() string {
	switch s {
	case shapeScalar:
		return "a single value"
	case shapeBlock:
		return "a block of fields"
	case shapeList:
		return "a list of values"
	default:
		return "a list of entries"
	}
}

// field is one field of a version of the file: where it is (its path from the entry, or from the
// top of the file for the two that are there), the shape it has, and whether a reader that does
// not read it as it is has to refuse the file. That last is the must-understand set of the
// version, read from this table by MustUnderstand and by the decoder, so that the set that is
// documented, the set that is answered for and the set that is refused are one.
type field struct {
	path  string
	shape shape
	must  bool
}

// refusal says why the file is refused for the shape this field was given, after "line N: ".
// children are the names of the fields of a block, in the order the file says them.
func (f field) refusal(children []string) string {
	switch f.shape {
	case shapeScalar:
		return fmt.Sprintf("%s must be a single value, not a block", f.path)
	case shapeBlock:
		return fmt.Sprintf("%s must be a block of fields (%s), not a value", f.path, strings.Join(children, ", "))
	case shapeList:
		return fmt.Sprintf("%s must be a list of values, one \"- value\" line each", f.path)
	default:
		return fmt.Sprintf("%s must be a list of entries, not a value", f.path)
	}
}

// note says that this field, which has a block under it where it should have a value, was left
// out, after "line N: ".
func (f field) note() string { return fmt.Sprintf("%s is not %s", f.path, f.shape.noun()) }

// strayValueNote says that the value on the line of this field, which is a block, was left out
// and the block under it was read, after "line N: ".
func (f field) strayValueNote() string {
	return fmt.Sprintf("%s has a value, which is left out", f.path)
}

// schema is the fields of one version of the file.
type schema struct {
	fields []field
	byPath map[string]field
}

func newSchema(fields ...field) schema {
	s := schema{fields: fields, byPath: make(map[string]field, len(fields))}
	for _, f := range fields {
		s.byPath[f.path] = f
	}
	return s
}

// childrenOf is the names of the fields directly under the field at path, in the order of the table.
func (s schema) childrenOf(path string) []string {
	var names []string
	for _, f := range s.fields {
		if rest, ok := strings.CutPrefix(f.path, path+"."); ok && !strings.Contains(rest, ".") {
			names = append(names, rest)
		}
	}
	return names
}

// mustUnderstand is the paths of the fields a reader must read as they are, sorted, in a list of
// the caller's own.
func (s schema) mustUnderstand() []string {
	var set []string
	for _, f := range s.fields {
		if f.must {
			set = append(set, f.path)
		}
	}
	sort.Strings(set)
	return set
}

// schemas are the fields of each version this package reads (decoders has the same versions).
var schemas = map[string]schema{"1": schemaV1}

// MustUnderstand is the must-understand set of a version of the file: the fields whose meaning
// changes what install, approval or projection does, so that a reader that cannot read one as
// it is must refuse the file instead of reading it without. It is sorted, and nil for a version
// that has no decoder. The rationale of each field is at the top of the package.
func MustUnderstand(version string) []string {
	s, ok := schemas[version]
	if !ok {
		return nil
	}
	return s.mustUnderstand()
}

// versionField is the one field every version of the file has, and the one that selects the
// decoder: it is read before any decoder is chosen, so it is described here and not by a version.
var versionField = field{path: "version", shape: shapeScalar, must: true}

// schemaV1 is the table of version 1. The order is the one the file says its fields in (it is
// the order Encode writes them), and the names a refusal lists for a block follow it.
var schemaV1 = newSchema(
	versionField,
	field{"skills", shapeEntries, true},
	field{"id", shapeScalar, true},
	field{"path", shapeScalar, true},
	field{"source", shapeBlock, true},
	field{"source.type", shapeScalar, true},
	field{"source.upstream", shapeBlock, false},
	field{"source.upstream.owner", shapeScalar, false},
	field{"source.repo", shapeScalar, false},
	field{"source.ref", shapeScalar, false},
	field{"install", shapeBlock, true},
	field{"install.defaultScope", shapeScalar, true},
	field{"install.allowedProjects", shapeList, true},
	field{"install.targets", shapeList, true},
	field{"lifecycle", shapeBlock, false},
	field{"lifecycle.updateStrategy", shapeScalar, false},
)
