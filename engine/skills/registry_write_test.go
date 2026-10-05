package skills

import (
	"bytes"
	"os"
	"testing"
)

// spyRegistries is the real repository of the tests with a record of what the verbs asked it to
// encode, and the bytes it answered.
type spyRegistries struct {
	RegistryRepository
	encoded []Registry
	written [][]byte
}

func (s *spyRegistries) Encode(reg Registry) ([]byte, error) {
	data, err := s.RegistryRepository.Encode(reg)
	s.encoded = append(s.encoded, reg)
	s.written = append(s.written, data)
	return data, err
}

// The registry a verb writes is the stored form its repository gave it, byte for byte: a verb
// knows no format of its own, so a repository that stores a registry another way is obeyed.
func TestAddAndRemoveWriteTheRegistryTheRepositoryEncodes(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir, minimalRegistry("existing"), minimalManifest("existing"), []string{"existing", "foo"})
	spy := &spyRegistries{RegistryRepository: testRegistries(os.ReadFile)}

	var out, errBuf bytes.Buffer
	exit := -1
	AddCore([]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile, spy, os.Stat, &out, &errBuf, func(c int) { exit = c })
	if exit != 0 {
		t.Fatalf("add exit %d; stderr=%q", exit, errBuf.String())
	}
	if len(spy.encoded) != 1 {
		t.Fatalf("add asked the repository to encode %d registries, want 1", len(spy.encoded))
	}
	if ids := entryIDs(spy.encoded[0]); len(ids) != 2 || ids[0] != "existing" || ids[1] != "foo" {
		t.Errorf("add encoded the entries %v, want existing and foo", ids)
	}
	if got, _ := os.ReadFile(regPath); !bytes.Equal(got, spy.written[0]) {
		t.Errorf("add wrote %q, want the bytes the repository encoded, %q", got, spy.written[0])
	}

	exit = -1
	RemoveCore([]string{"--registry", regPath, "--manifest", mfPath, "foo"},
		os.ReadFile, spy, &out, &errBuf, func(c int) { exit = c })
	if exit != 0 {
		t.Fatalf("remove exit %d; stderr=%q", exit, errBuf.String())
	}
	if len(spy.encoded) != 2 {
		t.Fatalf("remove asked the repository to encode %d registries in all, want 2", len(spy.encoded))
	}
	if ids := entryIDs(spy.encoded[1]); len(ids) != 1 || ids[0] != "existing" {
		t.Errorf("remove encoded the entries %v, want existing", ids)
	}
	if got, _ := os.ReadFile(regPath); !bytes.Equal(got, spy.written[1]) {
		t.Errorf("remove wrote %q, want the bytes the repository encoded, %q", got, spy.written[1])
	}
}

func entryIDs(reg Registry) []string {
	var ids []string
	for _, e := range reg.Skills {
		ids = append(ids, e.ID)
	}
	return ids
}
