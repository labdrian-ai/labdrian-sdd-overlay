package fsstore_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// The store's name, "projection store:", is said once by every message the store
// produces. An error it returns carries it from the sentinel it wraps; a detail of an
// unavailable binding (Load's) says what was found and nothing about who found it, as
// every detail of the domain (empty, malformed, foreign) already does. A detail that
// carried the name would say it twice in the refusal that quotes it ("projection store:
// refusing to change the binding: the on-disk state is unavailable: projection store:
// ..."), and one that did not would differ from its sibling for no reason.
const storeName = "projection store:"

func TestEveryUnavailableDetailIsFreeOfTheStoreName(t *testing.T) {
	for _, tc := range unavailableCases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := tc.setup(t)
			loaded := mustLoad(t, s, hex64("a"))
			if loaded.Classification != projection.ClassificationUnavailable || loaded.Detail == "" {
				t.Fatalf("Load() = %+v, want unavailable with a detail", loaded)
			}
			if strings.Contains(loaded.Detail, storeName) {
				t.Errorf("Load() detail = %q, want it free of %q", loaded.Detail, storeName)
			}
		})
	}
}

func TestARefusalNamesTheStoreOnceWhateverBranchProducedIt(t *testing.T) {
	for _, tc := range unavailableCases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := tc.setup(t)

			bindErr := s.Bind(hex64("a"), "proj-1", "wf-1", t0, true)
			_, unbindErr := s.Unbind(hex64("a"))
			for op, err := range map[string]error{"Bind": bindErr, "Unbind": unbindErr} {
				if err == nil {
					t.Errorf("%s() = nil, want the refusal", op)
					continue
				}
				if got := strings.Count(err.Error(), storeName); got != 1 {
					t.Errorf("%s() = %q, names the store %d times, want once", op, err, got)
				}
				if !strings.HasPrefix(err.Error(), projection.ErrBindingUnavailable.Error()+": ") {
					t.Errorf("%s() = %q, want it to start with the refusal %q", op, err, projection.ErrBindingUnavailable)
				}
			}
		})
	}
}

// The words themselves, for the states a person is most likely to meet: the detail
// says what is wrong with which path, and the refusal that quotes it adds only the
// store's name and what the store would not do.
func TestUnavailableDetailsAndRefusalsReadAsTheyAlwaysDid(t *testing.T) {
	refusal := projection.ErrBindingUnavailable.Error() + ": "
	tests := []struct {
		caseName string
		detail   func(path string) string
	}{
		{"the binding path is a directory", func(p string) string { return `binding path "` + p + `" is not a regular file` }},
		{"the binding path is a symlink to a valid binding", func(p string) string { return `refusing symlinked binding file "` + p + `"` }},
		{"the bindings directory is a symlink", func(p string) string { return `refusing symlinked store component "` + p + `"` }},
		{"the labdrian directory is a symlink", func(p string) string { return `refusing symlinked store component "` + p + `"` }},
	}
	byName := map[string]func(*testing.T) (path string, detail, refused string){}
	for _, tc := range unavailableCases {
		tc := tc
		byName[tc.name] = func(t *testing.T) (string, string, string) {
			s, path := tc.setup(t)
			loaded := mustLoad(t, s, hex64("a"))
			err := s.Bind(hex64("a"), "proj-1", "wf-1", t0, true)
			if err == nil {
				t.Fatal("Bind() = nil, want the refusal")
			}
			return path, loaded.Detail, err.Error()
		}
	}
	for _, tt := range tests {
		t.Run(tt.caseName, func(t *testing.T) {
			run, ok := byName[tt.caseName]
			if !ok {
				t.Fatalf("no unavailable case named %q", tt.caseName)
			}
			path, detail, refused := run(t)
			if want := tt.detail(path); detail != want {
				t.Errorf("Load() detail = %q, want %q", detail, want)
			}
			if want := refusal + tt.detail(path); refused != want {
				t.Errorf("Bind() = %q, want %q", refused, want)
			}
		})
	}
}
