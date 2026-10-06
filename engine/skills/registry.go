package skills

import (
	"errors"
	"fmt"
	"io"
)

// RegistryRepository is the port through which the skills domain reads and writes its own
// registry model. It is owned here and implemented by an adapter that knows where a registry is
// kept and in which format (engine/skills/registryyaml, the YAML file skills.registry.yaml): the
// domain knows neither, only that a registry has a location, can be loaded from it, and has a
// stored form that can be decoded and encoded.
//
// An adapter does not judge what it reads. Whether a registry may hold what it holds is the
// domain's rule (Registry.Validate), applied by the domain to what the adapter returns (ReadRegistry,
// DecodeRegistry); an adapter that called it would be deciding what the domain means.
//
// What an adapter says of a failure is a contract, because the domain judges before it reports:
//
//   - A store that could not be read at all is a *RegistryReadError, and the registry returned
//     with it is the zero value.
//   - A store that was read and could not be decoded returns the entries it did decode, whole and
//     in order, up to the fault, with an error that says the fault. The domain judges those
//     entries first, so what is named is the first fault in the order the store says it. A
//     decoder that stops at the fault and returns no entries makes a registry whose first entry
//     is invalid and whose second line is not understood report the second. (A fault of the
//     store's own version, which selects how the rest is read, comes before any entry and returns
//     nothing.)
//   - A store that was read whole and not understood to the last field is a registry, not an
//     error: the adapter returns what it understood, and Registry.Unread says what it left out,
//     one note per field. The verbs that read warn of it and go on, except validate (which fails,
//     CheckVerifiable); the verbs that rewrite the registry (AddEntry, RemoveEntry) refuse it,
//     because writing it back would drop the fields, and so does the build of the Pi package
//     (CheckBuildable), which would build an artifact from a partial read.
//     What an adapter must not leave out in silence is the fields its own policy says are needed
//     to read the registry as it means (for the YAML file, the must-understand set): it refuses
//     the store instead.
//
// Decode and Encode are the stored form of a registry as bytes. Add and remove call Encode on
// what they will write, decode the bytes back with Decode and compare, and only then write the
// bytes themselves; the repository does not write them. They are written through StagedWrites,
// the port of the writes of an overlay, which stages the registry and the manifest and commits
// them one after the other.
type RegistryRepository interface {
	// Load reads the registry stored at location. Location is opaque to the domain: it is the
	// value of a verb's --registry flag, or what an adapter says of an overlay.
	Load(location string) (Registry, error)
	// Decode reads a registry from its stored form.
	Decode(data []byte) (Registry, error)
	// Encode returns the stored form of reg, the exact bytes Decode reads back as reg. It refuses
	// a registry whose Unread is not empty: the bytes would not have what was left out.
	Encode(reg Registry) ([]byte, error)
}

// RegistryReadError says that the store of a registry could not be read: it does not exist, it
// is not a file, the process may not read it. Nothing of the registry is known. Its text is the
// text of the failure it carries, which is how a person is told.
type RegistryReadError struct {
	Err error
}

func (e *RegistryReadError) Error() string { return e.Err.Error() }
func (e *RegistryReadError) Unwrap() error { return e.Err }

// errNoRegistryRepository is what a verb says when it was given no way to read a registry: a
// composition root that forgot it. It is a refusal, not a panic: a verb is never to crash.
var errNoRegistryRepository = errors.New("skills: no registry repository is wired")

// ReadRegistry loads the registry at location through repo and judges it. The error is a
// *RegistryReadError when the store could not be read, and otherwise the first fault of the
// registry: of an entry (Registry.Validate), or, when every entry the reading got through is
// fine, of the reading itself. The registry returned with an error is the zero value.
func ReadRegistry(repo RegistryRepository, location string) (Registry, error) {
	if repo == nil {
		return Registry{}, errNoRegistryRepository
	}
	return judged(repo.Load(location))
}

// DecodeRegistry reads the registry whose stored form is data through repo and judges it, as
// ReadRegistry does.
func DecodeRegistry(repo RegistryRepository, data []byte) (Registry, error) {
	if repo == nil {
		return Registry{}, errNoRegistryRepository
	}
	return judged(repo.Decode(data))
}

// judged applies the rule to what an adapter read, and tells the first fault: the entries came
// first in the store, so a fault in them is named before the fault of the reading that stopped
// after them.
func judged(reg Registry, readErr error) (Registry, error) {
	var unreadable *RegistryReadError
	if errors.As(readErr, &unreadable) {
		return Registry{}, readErr
	}
	if err := reg.Validate(); err != nil {
		return Registry{}, err
	}
	if readErr != nil {
		return Registry{}, readErr
	}
	return reg, nil
}

// UnreadSummary says what the reader left out of the registry as a person is told it: the first
// note, and how many more there are. It is empty when nothing was left out.
func (r Registry) UnreadSummary() string {
	switch len(r.Unread) {
	case 0:
		return ""
	case 1:
		return r.Unread[0]
	default:
		return fmt.Sprintf("%s (and %d more)", r.Unread[0], len(r.Unread)-1)
	}
}

// UnreadWarning is what a person is told of the fields the reader left out, in the one wording
// every caller that tells it uses. It is empty when nothing was left out.
func (r Registry) UnreadWarning() string {
	if left := r.UnreadSummary(); left != "" {
		return "warning: registry fields left unread: " + left
	}
	return ""
}

// WarnOfUnread returns a repository that tells stderr what the reader left out of a registry it
// loads or decodes, in the words of Registry.UnreadWarning, for the callers that read a registry
// through the repository they are given and have no stderr of their own (the Pi package, the Pi
// runtime adapter). It tells it once, and only of a registry that is usable: one the domain
// refuses is refused in its own words. Everything else is the repository's own answer. A nil
// repository stays nil, so that what reads through it refuses for want of one.
func WarnOfUnread(repo RegistryRepository, stderr io.Writer) RegistryRepository {
	if repo == nil {
		return nil
	}
	return warningRegistries{RegistryRepository: repo, stderr: stderr}
}

type warningRegistries struct {
	RegistryRepository
	stderr io.Writer
}

func (w warningRegistries) Load(location string) (Registry, error) {
	reg, err := w.RegistryRepository.Load(location)
	w.tell(reg, err)
	return reg, err
}

func (w warningRegistries) Decode(data []byte) (Registry, error) {
	reg, err := w.RegistryRepository.Decode(data)
	w.tell(reg, err)
	return reg, err
}

func (w warningRegistries) tell(reg Registry, err error) {
	if err != nil || reg.Validate() != nil {
		return
	}
	if warning := reg.UnreadWarning(); warning != "" {
		fmt.Fprintln(w.stderr, warning)
	}
}

// CheckWritable says whether the registry may be written back as it is, and when it may not, why:
// a registry the reader did not read whole would lose what the reader left out, so it is not
// changed and written (add, remove) and an adapter does not encode it. Reading it, listing it,
// installing from it lose nothing and are not refused.
func (r Registry) CheckWritable() error { return r.refuseIfReadInPart("rewriting it would drop them") }

// CheckBuildable says whether a package may be built from the registry, and when it may not, why:
// a package is an artifact others consume, and one built from a registry the reader did not read
// whole would be built from a partial read, with no trace of what was left out.
func (r Registry) CheckBuildable() error {
	return r.refuseIfReadInPart("a package built from it would be built from a partial read")
}

// CheckVerifiable says whether the registry may be vouched for, and when it may not, why:
// validate is the verb a CI uses to detect drift, and a registry the reader did not read whole
// has parts nobody compared.
func (r Registry) CheckVerifiable() error {
	return r.refuseIfReadInPart("validate cannot vouch for a registry it read in part")
}

// refuseIfReadInPart is the one owner of the rule and of its words: a registry whose Unread is
// not empty is refused for what the question would cost, and a registry read whole is not.
func (r Registry) refuseIfReadInPart(consequence string) error {
	if len(r.Unread) == 0 {
		return nil
	}
	return fmt.Errorf("skills: the registry has fields this program does not read, and %s: %s", consequence, r.UnreadSummary())
}

// readRegistryForVerb reads the registry at path for a verb that works on it, and says what a
// person is told when it cannot: the words of a store that could not be read, or of a registry
// that is not usable (a verb that names the registry in its refusal, quotePath, puts the path
// in the second). It reports false, having printed and exited, when the verb has nothing more
// to do.
func readRegistryForVerb(registries RegistryRepository, path string, quotePath bool, stderr io.Writer, exit func(int)) (Registry, bool) {
	reg, err := ReadRegistry(registries, path)
	if err == nil {
		if warning := reg.UnreadWarning(); warning != "" {
			fmt.Fprintln(stderr, warning)
		}
		return reg, true
	}
	var unreadable *RegistryReadError
	switch {
	case errors.As(err, &unreadable):
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", path, err)
	case quotePath:
		fmt.Fprintf(stderr, "error: parsing registry %q: %v\n", path, err)
	default:
		fmt.Fprintf(stderr, "error: parsing registry: %v\n", err)
	}
	exit(1)
	return Registry{}, false
}
