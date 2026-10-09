package app_test

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/app"
)

// answer is one scripted answer of a fake: the content a read returns, or the error it fails with.
type answer struct {
	content string
	err     error
}

// notFound is the answer of a registry that does not exist, as a store reports it.
func notFound(path string) answer {
	return answer{err: &app.NotFoundError{Err: &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}}}
}

func text(content string) answer { return answer{content: content} }

func fails(message string) answer { return answer{err: errors.New(message)} }

// fakeStore is a RegistryStore over a map of files. Reads are answered from the script, in order,
// while it lasts, and from the files after; writes land in the files, and a foreign writer can be
// made to act after one. Every call is logged, so a test can say what the use case did and in what
// order.
type fakeStore struct {
	files   map[string]string
	script  []answer
	log     []string
	writes  int
	failOn  map[int]error
	clobber map[int]string
}

func newFakeStore(files map[string]string) *fakeStore {
	if files == nil {
		files = map[string]string{}
	}
	return &fakeStore{files: files, failOn: map[int]error{}, clobber: map[int]string{}}
}

func (s *fakeStore) Read(path string) ([]byte, error) {
	s.log = append(s.log, "read "+path)
	if len(s.script) > 0 {
		next := s.script[0]
		s.script = s.script[1:]
		if next.err != nil {
			return nil, next.err
		}
		return []byte(next.content), nil
	}
	content, ok := s.files[path]
	if !ok {
		return nil, notFound(path).err
	}
	return []byte(content), nil
}

func (s *fakeStore) Write(path string, content []byte) error {
	s.writes++
	s.log = append(s.log, "write "+path)
	if err := s.failOn[s.writes]; err != nil {
		return err
	}
	s.files[path] = string(content)
	if foreign, ok := s.clobber[s.writes]; ok {
		s.files[path] = foreign
	}
	return nil
}

// count says how many calls of the log start with prefix.
func (s *fakeStore) count(prefix string) int {
	n := 0
	for _, call := range s.log {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

// fakeContract is a ContractSource over a text, or an error, that counts how often it was read.
type fakeContract struct {
	text  string
	err   error
	reads int
}

func (c *fakeContract) Text() (string, error) {
	c.reads++
	return c.text, c.err
}
