package adoption_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/adoption"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// These cases run the use case over tables that stand for everything it reads through a port: the repository
// (a RepositoryInspector), the stores (Stores and Established), the ledger and the clock. Nothing here reads a
// disk, opens a database or asks the machine for the time, so each case says what the USE CASE does: what it asks,
// in what order, what it records and at what time, and which notes it owes the operator.

const (
	mainRoot = "/src/widgets"
	common   = "/src/widgets/.git"
)

// inspector is one repository: a main checkout at mainRoot, with the configuration and declaration it is given.
type inspector struct {
	locateErr   error
	config      string
	declaration string
}

func (i inspector) Locate(dir string) (projectid.Repository, error) {
	if i.locateErr != nil {
		return projectid.Repository{}, i.locateErr
	}
	return projectid.Repository{WorktreeRoot: mainRoot, MainWorktreeRoot: mainRoot, CommonDir: common}, nil
}

func (i inspector) DeclaredProject(root string) (string, bool, error) {
	return i.declaration, i.declaration != "", nil
}

func (i inspector) GitConfig(string) (string, error) { return i.config, nil }

const widgetsOrigin = "[remote \"origin\"]\n\turl = https://github.com/acme/widgets.git\n"

// stores are the stores of the table: the names that hold memory, the notes Open owes, and a record of how it was
// used.
type stores struct {
	holding  map[string]bool
	notes    []string
	askErr   error
	opened   int
	closed   int
	askedFor []string
}

func (s *stores) Open() (adoption.Established, []string) {
	s.opened++
	return s, s.notes
}

func (s *stores) HasMemory(name string) (bool, error) {
	s.askedFor = append(s.askedFor, name)
	if s.askErr != nil {
		return false, s.askErr
	}
	return s.holding[name], nil
}

func (s *stores) Close() { s.closed++ }

// ledger is the ledger of the table.
type ledger struct {
	known      []adoption.KnownName
	knownErr   error
	recordErr  error
	knownAsked []string

	recorded   []projectid.DerivedName
	recordedAt []time.Time
	recordedIn []string
}

func (l *ledger) Known(commonDir string) ([]adoption.KnownName, error) {
	l.knownAsked = append(l.knownAsked, commonDir)
	return l.known, l.knownErr
}

func (l *ledger) Record(commonDir string, derived []projectid.DerivedName, at time.Time) error {
	l.recordedIn = append(l.recordedIn, commonDir)
	l.recorded = append(l.recorded, derived...)
	l.recordedAt = append(l.recordedAt, at)
	return l.recordErr
}

// clock tells an instant in a zone that is not UTC.
type clock struct{ at time.Time }

func (c clock) Now() time.Time { return c.at }

var instant = time.Date(2026, 8, 1, 9, 30, 0, 0, time.FixedZone("somewhere", 2*60*60))

func useCase(in inspector, s *stores, l *ledger) adoption.UseCase {
	return adoption.UseCase{Identity: projectid.NewResolver(in), Stores: s, Ledger: l, Clock: clock{at: instant}}
}

func names(derived []projectid.DerivedName) []string {
	var out []string
	for _, d := range derived {
		out = append(out, d.Name)
	}
	return out
}

// Where nothing holds memory the chain decides, the adoption is no adoption, and what the repository derives is
// recorded, dated by the clock of the use case in UTC.
func TestAdopt_NothingEstablishedResolvesByTheChainAndRecordsWhatIsDerived(t *testing.T) {
	s, l := &stores{}, &ledger{}

	got, notes, err := useCase(inspector{config: widgetsOrigin}, s, l).Adopt("/anywhere", adoption.RecordDerived)

	if err != nil || len(notes) != 0 {
		t.Fatalf("Adopt = %v, %v; want no error and no note", err, notes)
	}
	if got.Adopted || got.Identity.Project != "github.com/acme/widgets" || got.Identity.Rule != projectid.RuleRemote {
		t.Errorf("Adopt = %+v, want the remote rule's answer, not adopted", got)
	}
	if want := []string{"github.com/acme/widgets", "widgets", common}; !reflect.DeepEqual(names(l.recorded), want) {
		t.Errorf("the ledger was given %v, want the names the repository derives, %v", names(l.recorded), want)
	}
	if want := []string{common}; !reflect.DeepEqual(l.recordedIn, want) {
		t.Errorf("recorded in %v, want the repository's common directory %v", l.recordedIn, want)
	}
	if len(l.recordedAt) != 1 || !l.recordedAt[0].Equal(instant) || l.recordedAt[0].Location() != time.UTC {
		t.Errorf("recorded at %v, want the clock's instant in UTC: %v", l.recordedAt, instant.UTC())
	}
}

// The name the memory already lives under is the name used, and the operator is told it was adopted.
func TestAdopt_ANameThatHoldsMemoryIsAdopted(t *testing.T) {
	s := &stores{holding: map[string]bool{"widgets": true}}

	got, _, err := useCase(inspector{config: widgetsOrigin}, s, &ledger{}).Adopt("/anywhere", adoption.RecordDerived)

	if err != nil || !got.Adopted || got.Identity.Project != "widgets" {
		t.Errorf("Adopt = %+v, %v; want the alias \"widgets\" adopted", got, err)
	}
}

// A command that promises to change nothing reads the ledger (that is what lets an established name win) and
// writes nothing.
func TestAdopt_ThePolicyThatRecordsNothingStillReadsTheLedger(t *testing.T) {
	s := &stores{holding: map[string]bool{"former-name": true}}
	l := &ledger{known: []adoption.KnownName{{Name: "former-name", Adoptable: true}}}

	got, _, err := useCase(inspector{}, s, l).Adopt("/anywhere", adoption.DoNotRecordDerived)

	if err != nil || got.Identity.Project != "former-name" || got.Identity.Rule != projectid.RuleRemembered {
		t.Errorf("Adopt = %+v, %v; want the remembered name adopted", got, err)
	}
	if !reflect.DeepEqual(l.knownAsked, []string{common}) {
		t.Errorf("the ledger was read for %v, want %v", l.knownAsked, []string{common})
	}
	if len(l.recordedIn) != 0 {
		t.Errorf("the ledger was written (%v) by a policy that records nothing", l.recordedIn)
	}
}

// Only the names a ledger vouches for are offered for adoption, and in the order it gave them.
func TestAdopt_OnlyTheAdoptableNamesOfTheLedgerAreRemembered(t *testing.T) {
	s := &stores{}
	l := &ledger{known: []adoption.KnownName{
		{Name: "newest", Adoptable: true}, {Name: "loose", Adoptable: false}, {Name: "oldest", Adoptable: true},
	}}

	if _, _, err := useCase(inspector{}, s, l).Adopt("/anywhere", adoption.DoNotRecordDerived); err != nil {
		t.Fatal(err)
	}

	// Derived names first (the main checkout's name, then the realpath of the common directory, C23 M2),
	// remembered ones after, in the ledger's order: storage was asked about each once.
	want := []string{"widgets", common, "newest", "oldest"}
	if !reflect.DeepEqual(s.askedFor, want) {
		t.Errorf("storage was asked about %v, want %v (the loose name is not offered)", s.askedFor, want)
	}
}

// What an operator is told when a place the adoption looks in could not be consulted: the cause, and what it may
// have cost. The words are the use case's, because they say what the rule could not do; a store only says what
// failed.
const (
	missed      = ", so an identity the memory already uses may have been missed"
	notRecorded = "; the ledger is left as it is and nothing was recorded"
)

// The notes come in the order things happened, and none of them stops the command: the stores first, then the
// ledger that could not be written. The adoption itself is made all the same. Each says what was not done and what
// that may have cost.
func TestAdopt_NotesAreOwedInTheOrderTheyHappenedAndNeverFailTheCommand(t *testing.T) {
	s := &stores{notes: []string{"the vault registry could not be read (boom)"}}
	l := &ledger{recordErr: errors.New("unwritable")}

	got, notes, err := useCase(inspector{config: widgetsOrigin}, s, l).Adopt("/anywhere", adoption.RecordDerived)

	if err != nil || got.Identity.Project != "github.com/acme/widgets" {
		t.Fatalf("Adopt = %+v, %v; want the chain's answer in spite of the notes", got, err)
	}
	want := []string{
		"the vault registry could not be read (boom)" + missed,
		"this repository's identity ledger could not be written (unwritable)" + missed,
	}
	if !reflect.DeepEqual(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	if len(l.recordedIn) != 1 {
		t.Errorf("the ledger was written to %d times, want the attempt that failed, once", len(l.recordedIn))
	}
}

// A ledger that could not be read is not written: the write would read it first and meet the same fault, and
// saying so a second time, as a failure to write, would be the same note twice with a cause that is not the
// ledger's being unwritable. So there is one note, the read one, and it says the ledger was left alone (the owner's
// decision of 2026-10-14). Nothing remembered is offered for adoption, and the adoption itself is made all the same.
func TestAdopt_ALedgerThatCannotBeReadIsReportedOnceAndNotWritten(t *testing.T) {
	s := &stores{notes: []string{"the vault registry could not be read (boom)"}, holding: map[string]bool{"former-name": true}}
	l := &ledger{knownErr: errors.New("unreadable"), recordErr: errors.New("unwritable"), known: []adoption.KnownName{{Name: "former-name", Adoptable: true}}}

	got, notes, err := useCase(inspector{config: widgetsOrigin}, s, l).Adopt("/anywhere", adoption.RecordDerived)

	if err != nil || got.Adopted || got.Identity.Project != "github.com/acme/widgets" {
		t.Fatalf("Adopt = %+v, %v; want the chain's answer, not an adoption: the ledger's word was not available", got, err)
	}
	want := []string{
		"the vault registry could not be read (boom)" + missed,
		"this repository's identity ledger could not be read (unreadable)" + missed + notRecorded,
	}
	if !reflect.DeepEqual(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	if len(l.recordedIn) != 0 {
		t.Errorf("the ledger was written to (%v) after it could not be read", l.recordedIn)
	}
}

// The clause that says nothing was recorded belongs to a command that would have recorded something. One that
// promises to record nothing (doctor, sync --dry-run) reads the ledger and says the cause, as it always has.
func TestAdopt_APolicyThatRecordsNothingDoesNotSayNothingWasRecorded(t *testing.T) {
	l := &ledger{knownErr: errors.New("unreadable")}

	_, notes, err := useCase(inspector{config: widgetsOrigin}, &stores{}, l).Adopt("/anywhere", adoption.DoNotRecordDerived)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{"this repository's identity ledger could not be read (unreadable)" + missed}
	if !reflect.DeepEqual(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	if len(l.recordedIn) != 0 {
		t.Errorf("the ledger was written to (%v) by a policy that records nothing", l.recordedIn)
	}
}

// A store that cannot say whether a name holds memory is the failure of the adoption, not an empty store, and
// the ledger is not written on the strength of it. The notes gathered so far come back with it.
func TestAdopt_AStoreThatCannotAnswerFailsTheAdoption(t *testing.T) {
	boom := errors.New("storage unavailable")
	s := &stores{askErr: boom, notes: []string{"a store was not consulted"}}
	l := &ledger{}

	got, notes, err := useCase(inspector{config: widgetsOrigin}, s, l).Adopt("/anywhere", adoption.RecordDerived)

	if !errors.Is(err, boom) || !reflect.DeepEqual(got, projectid.Adoption{}) {
		t.Errorf("Adopt = %+v, %v; want the store's error and no adoption", got, err)
	}
	if !reflect.DeepEqual(notes, []string{"a store was not consulted" + missed}) {
		t.Errorf("notes = %q, want the ones gathered before the failure", notes)
	}
	if len(l.recordedIn) != 0 {
		t.Errorf("the ledger was written (%v) after the adoption failed", l.recordedIn)
	}
}

// A directory in no repository is named by its name (C23 M2, decision 4); it has no ledger, so the ledger is
// neither asked anything nor written, and the stores that were opened are closed.
func TestAdopt_ADirectoryInNoRepositoryIsNamedByItsNameAndTheStoresAreClosed(t *testing.T) {
	s, l := &stores{notes: []string{"a store was not consulted"}}, &ledger{}

	got, notes, err := useCase(inspector{locateErr: projectid.ErrNotARepository}, s, l).Adopt("/elsewhere", adoption.RecordDerived)

	if err != nil || got.Identity != (projectid.Identity{Project: "elsewhere", Rule: projectid.RuleDirectory}) || got.Adopted {
		t.Errorf("Adopt = %+v, %v; want the directory's name, not adopted", got, err)
	}
	if !reflect.DeepEqual(s.askedFor, []string{"elsewhere"}) {
		t.Errorf("storage was asked about %v, want the directory's name only", s.askedFor)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %q, want the store's", notes)
	}
	if len(l.knownAsked) != 0 || len(l.recordedIn) != 0 {
		t.Errorf("the ledger was read (%v) or written (%v) for a directory in no repository", l.knownAsked, l.recordedIn)
	}
	if s.opened != 1 || s.closed != 1 {
		t.Errorf("stores opened %d times and closed %d, want once each", s.opened, s.closed)
	}
}

// appearsAfterTheFirstLook is a repository that cannot be located the first time it is asked about and can the
// second (a directory that became a repository between the two looks).
type appearsAfterTheFirstLook struct {
	inspector
	looked int
}

func (a *appearsAfterTheFirstLook) Locate(dir string) (projectid.Repository, error) {
	a.looked++
	if a.looked == 1 {
		return projectid.Repository{}, errors.New("not yet")
	}
	return a.inspector.Locate(dir)
}

// Where the ledger's place was not found, nothing is recorded: a record with no directory to go to would land in
// a directory of the process's choosing, and a ledger that is in the wrong place is one that vouches for nothing.
func TestAdopt_NothingIsRecordedWhereTheLedgersPlaceWasNotFound(t *testing.T) {
	l := &ledger{known: []adoption.KnownName{{Name: "former-name", Adoptable: true}}}
	inspector := &appearsAfterTheFirstLook{inspector: inspector{config: widgetsOrigin}}
	u := adoption.UseCase{Identity: projectid.NewResolver(inspector), Stores: &stores{}, Ledger: l, Clock: clock{at: instant}}

	got, notes, err := u.Adopt("/anywhere", adoption.RecordDerived)

	if err != nil || got.Identity.Project != "github.com/acme/widgets" || len(notes) != 0 {
		t.Fatalf("Adopt = %+v, %v, %v; want the chain's answer", got, notes, err)
	}
	if len(l.knownAsked) != 0 || len(l.recordedIn) != 0 {
		t.Errorf("the ledger was read for %v and written to %v; it has no place to be in", l.knownAsked, l.recordedIn)
	}
}

// The stores are closed once whatever happens, so a database opened for the question is never left open.
func TestAdopt_TheStoresAreClosedOnceOnEveryPath(t *testing.T) {
	for name, in := range map[string]inspector{
		"an adoption":                  {config: widgetsOrigin},
		"a directory in no repository": {locateErr: projectid.ErrNotARepository},
	} {
		s := &stores{}

		_, _, _ = useCase(in, s, &ledger{}).Adopt("/anywhere", adoption.RecordDerived)

		if s.opened != 1 || s.closed != 1 {
			t.Errorf("%s: stores opened %d times and closed %d, want once each", name, s.opened, s.closed)
		}
	}
}
