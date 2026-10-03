package registryyaml

import (
	"bytes"
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// Repository is the skills.RegistryRepository of the YAML file: a location is the path of the
// file, which is read through the function it was built with (os.ReadFile in the program, a map
// of files in a test), so the package reaches the file system only where its composition root says.
type Repository struct {
	read func(path string) ([]byte, error)
}

var _ skills.RegistryRepository = Repository{}

// NewRepository returns the repository that reads a registry file through read.
func NewRepository(read func(path string) ([]byte, error)) Repository {
	return Repository{read: read}
}

// errNoReader is what Load says for a Repository built with no way to read a file.
var errNoReader = errors.New("registryyaml: the repository was built with no way to read a file")

// Load reads the file at location and decodes it. A file that cannot be read is a
// *skills.RegistryReadError carrying the reader's own error; what can be read and not decoded is
// Decode's fault, with the entries before it (see skills.RegistryRepository).
func (r Repository) Load(location string) (skills.Registry, error) {
	if r.read == nil {
		return skills.Registry{}, &skills.RegistryReadError{Err: errNoReader}
	}
	data, err := r.read(location)
	if err != nil {
		return skills.Registry{}, &skills.RegistryReadError{Err: err}
	}
	return r.Decode(data)
}

// Decode reads a registry from the bytes of its file.
func (Repository) Decode(data []byte) (skills.Registry, error) { return Decode(bytes.NewReader(data)) }

// Encode returns the bytes of the file of reg.
func (Repository) Encode(reg skills.Registry) ([]byte, error) { return Encode(reg) }
