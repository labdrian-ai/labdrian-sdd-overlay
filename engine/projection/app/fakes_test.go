package app_test

import (
	"errors"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The fakes below answer the ports of the use cases in memory. None of them touches a file, the
// environment or the clock of the machine.

const (
	repoDir  = "/work/repo"
	repoKey  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	otherKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

// fakeLocator knows the keys of the directories it was given, and counts how often it was asked.
type fakeLocator struct {
	keys  map[string]string
	asked int
}

func newLocator() *fakeLocator { return &fakeLocator{keys: map[string]string{repoDir: repoKey}} }

func (l *fakeLocator) RepoKey(dir string) (string, bool) {
	l.asked++
	key, ok := l.keys[dir]
	return key, ok
}

// fakeClock tells a time the test sets, and counts how often it was asked.
type fakeClock struct {
	now   time.Time
	asked int
}

func (c *fakeClock) Now() time.Time { c.asked++; return c.now }

// fakeWorkflows answers with the workflows it was given; one it was not given is absent.
type fakeWorkflows struct {
	byID  map[string]workflow.Loaded
	asked []string
}

func newWorkflows() *fakeWorkflows { return &fakeWorkflows{byID: map[string]workflow.Loaded{}} }

func (w *fakeWorkflows) put(project, id string, loaded workflow.Loaded) {
	w.byID[project+"/"+id] = loaded
}

func (w *fakeWorkflows) Load(project, id string) workflow.Loaded {
	w.asked = append(w.asked, project+"/"+id)
	if loaded, ok := w.byID[project+"/"+id]; ok {
		return loaded
	}
	return workflow.Loaded{Classification: workflow.ClassificationAbsent}
}

// owned is an owned workflow in status, the shape the projection reads.
func owned(status workflow.Status) workflow.Loaded {
	return workflow.Loaded{
		Classification: workflow.ClassificationOwned,
		State: workflow.State{
			Status:  status,
			Profile: "standalone-minimal",
			GoalID:  "goal-1",
			// A digest of 64 characters, as the log records it.
			GoalDigest: strings.Repeat("c", 64),
		},
	}
}

// closed is an owned workflow that was abandoned.
func closed() workflow.Loaded {
	w := owned(workflow.StatusClosed)
	w.State.CloseOutcome = workflow.OutcomeAbandoned
	return w
}

// memoryBindings is a binding store in memory, deciding with the functions of the domain the real
// store decides with. It holds the document of each repository, and its hooks let a test play the
// other process that changes a binding in the window between two steps of a use case.
type memoryBindings struct {
	files map[string][]byte

	loads int
	// loadErr, when set, is what every Load returns.
	loadErr error
	// unbindErr, when set, is what UnbindIfUnchanged returns.
	unbindErr error

	// beforeReplace runs just before BindIfUnchanged judges, afterBind just after Bind or
	// BindIfUnchanged wrote, and beforeRemoval just before UnbindIfUnchanged judges.
	beforeReplace func()
	afterBind     func()
	beforeRemoval func()
	// beforeBind runs just before Bind judges.
	beforeBind func()

	// removals counts the calls of UnbindIfUnchanged.
	removals int
}

func newBindings() *memoryBindings { return &memoryBindings{files: map[string][]byte{}} }

func (s *memoryBindings) Load(key string) (projection.Loaded, error) {
	s.loads++
	if s.loadErr != nil {
		return projection.Loaded{}, s.loadErr
	}
	return s.read(key), nil
}

func (s *memoryBindings) read(key string) projection.Loaded {
	data, ok := s.files[key]
	if !ok {
		return projection.Loaded{Classification: projection.ClassificationAbsent}
	}
	return projection.ClassifyBinding(key, data)
}

func (s *memoryBindings) put(key string, b projection.Binding) {
	data, err := b.Marshal()
	if err != nil {
		panic(err)
	}
	s.files[key] = data
}

func (s *memoryBindings) bound(key string) projection.Binding { return s.read(key).Binding }

func (s *memoryBindings) Bind(key, project, id string, now time.Time, replace bool) error {
	if s.beforeBind != nil {
		s.beforeBind()
	}
	write, err := projection.AdmitBind(s.read(key), projection.NewBinding(key, project, id, now), replace)
	if err != nil {
		return err
	}
	if write {
		s.put(key, projection.NewBinding(key, project, id, now))
	}
	if s.afterBind != nil {
		s.afterBind()
	}
	return nil
}

func (s *memoryBindings) BindIfUnchanged(key, project, id string, now time.Time, expected projection.Binding) error {
	if s.beforeReplace != nil {
		s.beforeReplace()
	}
	record := projection.NewBinding(key, project, id, now)
	write, err := projection.AdmitReplace(s.read(key), expected, record)
	if err != nil {
		return err
	}
	if write {
		s.put(key, record)
	}
	if s.afterBind != nil {
		s.afterBind()
	}
	return nil
}

func (s *memoryBindings) Unbind(key string) (bool, error) {
	remove, err := projection.AdmitUnbind(s.read(key))
	if err != nil {
		return false, err
	}
	if remove {
		delete(s.files, key)
	}
	return remove, nil
}

func (s *memoryBindings) UnbindIfUnchanged(key string, expected projection.Binding) (bool, error) {
	s.removals++
	if s.beforeRemoval != nil {
		s.beforeRemoval()
	}
	if s.unbindErr != nil {
		return false, s.unbindErr
	}
	remove, err := projection.AdmitRemoval(s.read(key), expected)
	if err != nil {
		return false, err
	}
	if remove {
		delete(s.files, key)
	}
	return remove, nil
}

// errStoreDown is an error the store fails with.
var errStoreDown = errors.New("projection store: the state home is not usable")
