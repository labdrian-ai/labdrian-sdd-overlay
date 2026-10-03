package skills

// Tests for the registry as the skills domain owns it: the RegistryRepository port, the way the
// domain reads a registry through it (ReadRegistry, DecodeRegistry), and the pure rule that says
// what a registry may hold (Registry.Validate). Nothing here knows a file format: the repository
// is a stub that returns the values the test wants it to. What a YAML file says of each
// registry, and what the program prints of it, is pinned by the adapter's tests
// (engine/skills/registryyaml) and by the golden files of engine/cmd.

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

// stubRegistries is a RegistryRepository whose answers are given by the test.
type stubRegistries struct {
	load   func(location string) (Registry, error)
	decode func(data []byte) (Registry, error)
	encode func(reg Registry) ([]byte, error)
}

func (s stubRegistries) Load(location string) (Registry, error) { return s.load(location) }
func (s stubRegistries) Decode(data []byte) (Registry, error)   { return s.decode(data) }
func (s stubRegistries) Encode(reg Registry) ([]byte, error)    { return s.encode(reg) }

// validEntry is an entry of a registry that Validate accepts.
func validEntry(id string) Entry {
	return Entry{
		ID:        id,
		Path:      id,
		Source:    Source{Type: SourceCustom},
		Install:   Install{DefaultScope: ScopeGlobal, Targets: []string{"claude"}},
		Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
	}
}

func registryOfEntries(entries ...Entry) Registry { return Registry{Version: "1", Skills: entries} }

// --- the rule: what a registry may hold ----------------------------------------------------

func TestAValidRegistryIsValid(t *testing.T) {
	core := validEntry("core-one")
	core.Source = Source{Type: SourceCore, Upstream: &Upstream{Owner: "someone"}}
	core.Lifecycle.UpdateStrategy = "vendor-merge"
	external := validEntry("ext-one")
	external.Source = Source{Type: SourceExternal, Repo: "https://example.test/r", Ref: "v1"}
	project := validEntry("proj-one")
	project.Install = Install{DefaultScope: ScopeProject, Targets: []string{"claude", "opencode", "codex", "pi"}, AllowedProjects: []string{"demo"}}
	for name, reg := range map[string]Registry{
		"no skills":                     {Version: "1"},
		"one of each kind":              registryOfEntries(validEntry("a"), core, external, project),
		"a project skill with none":     registryOfEntries(Entry{ID: "p", Path: "p", Source: Source{Type: SourceCustom}, Install: Install{DefaultScope: ScopeProject, Targets: []string{"pi"}}, Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"}}),
		"a core skill with no upstream": registryOfEntries(Entry{ID: "c", Path: "c", Source: Source{Type: SourceCore}, Install: Install{DefaultScope: ScopeGlobal, Targets: []string{"claude"}}, Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"}}),
		"a path that is not the id":     registryOfEntries(func() Entry { e := validEntry("a"); e.Path = "group-dir"; return e }()),
		"no scope: carried over, an entry without one is read as neither": registryOfEntries(func() Entry { e := validEntry("a"); e.Install.DefaultScope = ""; return e }()),
		"the same path twice":     registryOfEntries(validEntry("a"), func() Entry { e := validEntry("b"); e.Path = "a"; return e }()),
		"ids that differ in case": registryOfEntries(validEntry("a"), validEntry("A")),
		"targets repeated":        registryOfEntries(func() Entry { e := validEntry("a"); e.Install.Targets = []string{"pi", "pi"}; return e }()),
	} {
		t.Run(name, func(t *testing.T) {
			if err := reg.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// The words of every refusal are the ones the registry has always been refused in: they reach a
// person through every verb, and the golden files of engine/cmd pin them from the outside.
func TestValidateRefusesWhatARegistryMayNotHold(t *testing.T) {
	with := func(mutate func(e *Entry)) Registry {
		e := validEntry("alpha")
		mutate(&e)
		return registryOfEntries(e)
	}
	for name, tc := range map[string]struct {
		reg  Registry
		want string
	}{
		"no id":                           {with(func(e *Entry) { e.ID = "" }), `skills: entry is missing required field 'id'`},
		"no path":                         {with(func(e *Entry) { e.Path = "" }), `skills: entry "alpha": path must not be empty`},
		"an absolute path":                {with(func(e *Entry) { e.Path = "/etc/alpha" }), `skills: entry "alpha": path "/etc/alpha" must be relative, not absolute`},
		"a path with a dot first":         {with(func(e *Entry) { e.Path = "./alpha" }), `skills: entry "alpha": path "./alpha" must already be a clean relative path`},
		"a path with a trailing /":        {with(func(e *Entry) { e.Path = "alpha/" }), `skills: entry "alpha": path "alpha/" must already be a clean relative path`},
		"a path that climbs":              {with(func(e *Entry) { e.Path = "a/../b" }), `skills: entry "alpha": path "a/../b" must already be a clean relative path`},
		"a path that leaves the root":     {with(func(e *Entry) { e.Path = "../alpha" }), `skills: entry "alpha": path "../alpha" must not contain a ".." component`},
		"only dots":                       {with(func(e *Entry) { e.Path = ".." }), `skills: entry "alpha": path ".." must not contain a ".." component`},
		"no type of source":               {with(func(e *Entry) { e.Source.Type = "" }), `skills: entry "alpha": source.type "" is not valid; must be 'core', 'custom', or 'external'`},
		"an unknown type of source":       {with(func(e *Entry) { e.Source.Type = "wizard" }), `skills: entry "alpha": source.type "wizard" is not valid; must be 'core', 'custom', or 'external'`},
		"a custom skill with an upstream": {with(func(e *Entry) { e.Source.Upstream = &Upstream{Owner: "x"} }), `skills: entry "alpha": source.upstream is not allowed when source.type is 'custom'`},
		"an external skill with an upstream": {with(func(e *Entry) {
			e.Source = Source{Type: SourceExternal, Repo: "https://example.test/r", Upstream: &Upstream{Owner: "x"}}
		}), `skills: entry "alpha": source.upstream is not allowed when source.type is 'external'`},
		"a core upstream with no owner":          {with(func(e *Entry) { e.Source = Source{Type: SourceCore, Upstream: &Upstream{}} }), `skills: entry "alpha": source.upstream.owner must not be empty`},
		"projects on a global skill":             {with(func(e *Entry) { e.Install.AllowedProjects = []string{"demo"} }), `skills: entry "alpha": allowedProjects is only valid for project-scoped entries`},
		"no targets":                             {with(func(e *Entry) { e.Install.Targets = nil }), `skills: entry "alpha": install.targets must not be empty (R-007)`},
		"a target that does not exist":           {with(func(e *Entry) { e.Install.Targets = []string{"claude", "vim"} }), `skills: entry "alpha": install.targets contains invalid value "vim"; must be one of: claude, opencode, codex, pi`},
		"a target in capitals":                   {with(func(e *Entry) { e.Install.Targets = []string{"Claude"} }), `skills: entry "alpha": install.targets contains invalid value "Claude"; must be one of: claude, opencode, codex, pi`},
		"no update strategy":                     {with(func(e *Entry) { e.Lifecycle.UpdateStrategy = "" }), `skills: entry "alpha": lifecycle.updateStrategy "" is not valid; must be 'vendor-merge' or 'overlay-only'`},
		"an update strategy that does not exist": {with(func(e *Entry) { e.Lifecycle.UpdateStrategy = "rolling" }), `skills: entry "alpha": lifecycle.updateStrategy "rolling" is not valid; must be 'vendor-merge' or 'overlay-only'`},
		// New in the domain: the adapter of a file words this with the line it is on, and refuses it
		// before the domain is asked; a registry that comes from anywhere else is held to it here.
		"a scope that does not exist": {with(func(e *Entry) { e.Install.DefaultScope = "workspace" }), `skills: entry "alpha": install.defaultScope "workspace" is not valid; must be 'global' or 'project'`},
		"a scope in capitals":         {with(func(e *Entry) { e.Install.DefaultScope = "Global" }), `skills: entry "alpha": install.defaultScope "Global" is not valid; must be 'global' or 'project'`},
		"two entries of one id":       {registryOfEntries(validEntry("alpha"), validEntry("beta"), validEntry("alpha")), `skills: duplicate id "alpha"`},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.reg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want %q", tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("Validate() = %q, want %q", err, tc.want)
			}
		})
	}
}

// The first fault in the order of the entries is the one named, and for one entry the check of
// its fields comes before the check that its id is new: the order the registry has always been
// judged in, which the golden files pin from the outside.
func TestValidateNamesTheFirstFaultInTheOrderOfTheEntries(t *testing.T) {
	bad := func(id string) Entry { e := validEntry(id); e.Path = ""; return e }
	for name, tc := range map[string]struct {
		reg  Registry
		want string
	}{
		"an invalid entry before a repeated id":                   {registryOfEntries(validEntry("a"), bad("b"), validEntry("a")), `skills: entry "b": path must not be empty`},
		"a repeated id before a later invalid entry":              {registryOfEntries(validEntry("a"), validEntry("a"), bad("c")), `skills: duplicate id "a"`},
		"an entry that is invalid and repeated: its fields first": {registryOfEntries(validEntry("a"), bad("a")), `skills: entry "a": path must not be empty`},
		"the first of two invalid entries":                        {registryOfEntries(bad("x"), bad("y")), `skills: entry "x": path must not be empty`},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tc.reg.Validate(); err == nil || err.Error() != tc.want {
				t.Errorf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

// --- reading a registry through the port ---------------------------------------------------

func TestReadRegistryAsksTheRepositoryForTheLocationItWasGiven(t *testing.T) {
	var asked []string
	want := registryOfEntries(validEntry("a"))
	repo := stubRegistries{load: func(location string) (Registry, error) { asked = append(asked, location); return want, nil }}
	got, err := ReadRegistry(repo, "some/where.yaml")
	if err != nil || len(got.Skills) != 1 || got.Skills[0].ID != "a" {
		t.Fatalf("ReadRegistry() = %+v, %v, want the registry the repository returned", got, err)
	}
	if len(asked) != 1 || asked[0] != "some/where.yaml" {
		t.Errorf("the repository was asked for %v, want exactly the location given", asked)
	}
}

func TestReadRegistryNeedsARepository(t *testing.T) {
	if _, err := ReadRegistry(nil, "x"); err == nil || !strings.Contains(err.Error(), "no registry repository") {
		t.Errorf("ReadRegistry(nil) = %v, want an error that says there is no repository", err)
	}
	if _, err := DecodeRegistry(nil, []byte("x")); err == nil || !strings.Contains(err.Error(), "no registry repository") {
		t.Errorf("DecodeRegistry(nil) = %v, want an error that says there is no repository", err)
	}
}

// A store that cannot be read has nothing of a registry to judge: whatever the repository
// returned with the error is not looked at, and the error is told as it is, with its words.
func TestReadRegistryTellsAnUnreadableStoreAsItIs(t *testing.T) {
	cause := &fs.PathError{Op: "open", Path: "r.yaml", Err: fs.ErrNotExist}
	invalid := registryOfEntries(validEntry(""))
	repo := stubRegistries{load: func(string) (Registry, error) { return invalid, &RegistryReadError{Err: cause} }}
	got, err := ReadRegistry(repo, "r.yaml")
	var unreadable *RegistryReadError
	if !errors.As(err, &unreadable) {
		t.Fatalf("ReadRegistry() = %v, want a *RegistryReadError", err)
	}
	if err.Error() != cause.Error() {
		t.Errorf("the error says %q, want the words of what could not be read, %q", err, cause)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the error does not wrap the cause: errors.Is(err, fs.ErrNotExist) = false")
	}
	if len(got.Skills) != 0 || got.Version != "" {
		t.Errorf("ReadRegistry() returned %+v with an error, want the zero registry", got)
	}
}

// The entries a reader got through before it met a fault are judged first, in order, and the
// fault of the reading is told only when they are fine: the first thing wrong in the file, in the
// order the file says it, is what is named (a registry has always been judged entry by entry as
// it was read, and what a reader that stops half way hands back is what lets that stay true).
func TestReadRegistryJudgesWhatWasReadBeforeTheFaultOfTheReading(t *testing.T) {
	fault := errors.New("line 9: unexpected indentation inside skills sequence")
	bad := validEntry("b")
	bad.Path = ""
	for name, tc := range map[string]struct {
		read    Registry
		readErr error
		want    string
	}{
		"nothing read before the fault":                {Registry{}, fault, fault.Error()},
		"entries that are fine before the fault":       {registryOfEntries(validEntry("a")), fault, fault.Error()},
		"an invalid entry before the fault":            {registryOfEntries(validEntry("a"), bad), fault, `skills: entry "b": path must not be empty`},
		"a repeated id before the fault":               {registryOfEntries(validEntry("a"), validEntry("a")), fault, `skills: duplicate id "a"`},
		"an invalid entry and no fault of the reading": {registryOfEntries(bad), nil, `skills: entry "b": path must not be empty`},
	} {
		t.Run(name, func(t *testing.T) {
			repo := stubRegistries{load: func(string) (Registry, error) { return tc.read, tc.readErr }}
			got, err := ReadRegistry(repo, "r")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("ReadRegistry() = %v, want %q", err, tc.want)
			}
			if len(got.Skills) != 0 || got.Version != "" {
				t.Errorf("ReadRegistry() returned %+v with an error, want the zero registry: no partial data", got)
			}
			var unreadable *RegistryReadError
			if errors.As(err, &unreadable) {
				t.Errorf("%v is a *RegistryReadError: only a store that could not be read is one", err)
			}
		})
	}
}

func TestReadRegistryReturnsAWholeValidRegistryAsItWasRead(t *testing.T) {
	want := registryOfEntries(validEntry("a"), validEntry("b"))
	want.Version = "1"
	repo := stubRegistries{load: func(string) (Registry, error) { return want, nil }}
	got, err := ReadRegistry(repo, "r")
	if err != nil || got.Version != "1" || len(got.Skills) != 2 || got.Skills[0].ID != "a" || got.Skills[1].ID != "b" {
		t.Errorf("ReadRegistry() = %+v, %v, want the registry the repository read", got, err)
	}
}

// What is decoded from bytes is judged exactly as what is loaded from a location: the writing
// verbs read their own output back through it before they write it.
func TestDecodeRegistryJudgesLikeReadRegistry(t *testing.T) {
	fault := errors.New("line 3: tab character not allowed; use spaces for indentation")
	bad := validEntry("b")
	bad.Install.Targets = nil
	for name, tc := range map[string]struct {
		read    Registry
		readErr error
		want    string // "" for success
	}{
		"a whole valid registry":            {registryOfEntries(validEntry("a")), nil, ""},
		"a fault with nothing before":       {Registry{}, fault, fault.Error()},
		"an invalid entry before the fault": {registryOfEntries(bad), fault, `skills: entry "b": install.targets must not be empty (R-007)`},
		"an invalid entry, no fault":        {registryOfEntries(bad), nil, `skills: entry "b": install.targets must not be empty (R-007)`},
	} {
		t.Run(name, func(t *testing.T) {
			var got []byte
			repo := stubRegistries{decode: func(data []byte) (Registry, error) { got = data; return tc.read, tc.readErr }}
			reg, err := DecodeRegistry(repo, []byte("the bytes"))
			if string(got) != "the bytes" {
				t.Errorf("the repository decoded %q, want the bytes given", got)
			}
			switch {
			case tc.want == "" && (err != nil || len(reg.Skills) != 1):
				t.Errorf("DecodeRegistry() = %+v, %v, want the registry", reg, err)
			case tc.want != "" && (err == nil || err.Error() != tc.want || len(reg.Skills) != 0):
				t.Errorf("DecodeRegistry() = %+v, %v, want the zero registry and %q", reg, err, tc.want)
			}
		})
	}
}

func TestTheVocabularyOfARegistryIsTheOneItHasAlwaysHad(t *testing.T) {
	for name, tc := range map[string]struct{ got, want string }{
		"global":   {ScopeGlobal, "global"},
		"project":  {ScopeProject, "project"},
		"core":     {SourceCore, "core"},
		"custom":   {SourceCustom, "custom"},
		"external": {SourceExternal, "external"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s is %q, want %q", name, tc.got, tc.want)
		}
	}
	if got := fmt.Sprint(RegistryTargets()); got != "[claude opencode codex pi]" {
		t.Errorf("RegistryTargets() = %s, want the four runtimes in the order the refusals list them", got)
	}
}
