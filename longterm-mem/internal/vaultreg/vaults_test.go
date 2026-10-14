package vaultreg

import (
	"errors"
	"io/fs"
	"testing"
)

// fakeStore is a Store that keeps the registry in memory, counts what it is asked and fails where a case says so.
type fakeStore struct {
	registry *Registry
	// failExists, failLoad and failSave are the errors the store answers with, when set.
	failExists, failLoad, failSave error

	exists, loads, saves int
}

var _ Store = (*fakeStore)(nil)

func (s *fakeStore) Exists() (bool, error) {
	s.exists++
	if s.failExists != nil {
		return false, s.failExists
	}
	return s.registry != nil, nil
}

func (s *fakeStore) Load() (Registry, error) {
	s.loads++
	if s.failLoad != nil {
		return Registry{}, s.failLoad
	}
	if s.registry == nil {
		return Registry{}, fs.ErrNotExist
	}
	return *s.registry, nil
}

func (s *fakeStore) Save(reg Registry) error {
	s.saves++
	if s.failSave != nil {
		return s.failSave
	}
	s.registry = &reg
	return nil
}

// defaults are the rows a machine without a registry starts with in these cases.
func defaults() Registry {
	return Registry{Vaults: map[string]Entry{"the-default": {Path: "/vaults/default", Seeded: true}}, Origin: "/registry/vaults.json"}
}

func vaultsOver(store Store) Vaults {
	return Vaults{Store: store, Defaults: defaults(), Environment: Environment{Home: home("/home/someone")}}
}

// A command that only reads finds the vault of the default project on a machine that has no registry, and leaves
// the machine as it found it: the Defaults are what a registry nobody has written holds, and nothing is written.
func TestVaults_ToRead_ReadsTheDefaultsOfAMachineWithNoRegistryAndWritesNothing(t *testing.T) {
	store := &fakeStore{}

	got, err := vaultsOver(store).ToRead("the-default", "")

	if err != nil || got != "/vaults/default" {
		t.Fatalf("ToRead = (%q, %v), want the default's vault", got, err)
	}
	if store.saves != 0 || store.registry != nil {
		t.Errorf("the store was saved %d time(s) and holds %+v, want nothing written", store.saves, store.registry)
	}
}

// The words of a project with no row name the place the registry is kept, which a registry nobody has written has
// not told: they are the Defaults', so what a command that reads says is what it said when it made the file first.
func TestVaults_ToRead_ANoRowOnAMachineWithNoRegistryNamesWhereTheRegistryWouldBe(t *testing.T) {
	_, err := vaultsOver(&fakeStore{}).ToRead("another", "")

	if !errors.Is(err, ErrVaultNotConfigured) {
		t.Fatalf("ToRead = %v, want ErrVaultNotConfigured", err)
	}
	if want := `vaultreg: vault not configured: project "another" has no vault-registry entry in /registry/vaults.json`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// A registry that is there is what a command that reads reads, whatever the Defaults say: the default row a person
// deleted is a project that is not configured.
func TestVaults_ToRead_ReadsARegistryThatIsThere(t *testing.T) {
	store := &fakeStore{registry: &Registry{Vaults: map[string]Entry{"another": {Path: "/vaults/another"}}, Origin: "/registry/vaults.json"}}

	if got, err := vaultsOver(store).ToRead("another", ""); err != nil || got != "/vaults/another" {
		t.Errorf("ToRead of the row there = (%q, %v), want its vault", got, err)
	}
	if _, err := vaultsOver(store).ToRead("the-default", ""); !errors.Is(err, ErrVaultNotConfigured) {
		t.Errorf("ToRead of the deleted default row = %v, want ErrVaultNotConfigured", err)
	}
	if store.saves != 0 {
		t.Errorf("the store was saved %d time(s), want none", store.saves)
	}
}

func TestVaults_ToRead_LeavesTheRegistryAloneWhenTheVaultIsNamed(t *testing.T) {
	store := &fakeStore{}
	v := vaultsOver(store)
	v.Environment.Vault = "/vaults/env"

	if got, err := v.ToRead("the-default", ""); err != nil || got != "/vaults/env" {
		t.Fatalf("ToRead = (%q, %v), want the environment's vault", got, err)
	}
	if store.exists+store.loads+store.saves != 0 {
		t.Errorf("the store was asked %d question(s), %d load(s) and %d save(s), want it left alone", store.exists, store.loads, store.saves)
	}
}

// Every failure of the store is the answer, as it was said: a registry that cannot be told about or read is not
// taken for one nobody has written.
func TestVaults_ToRead_AnswersTheFailuresOfTheStoreAsTheyAre(t *testing.T) {
	boom := errors.New("boom")

	for name, store := range map[string]*fakeStore{
		"it cannot be told whether there is a registry": {failExists: boom},
		"it cannot be read":                             {registry: &Registry{}, failLoad: boom},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := vaultsOver(store).ToRead("the-default", ""); !errors.Is(err, boom) {
				t.Errorf("ToRead = %v, want the failure", err)
			}
			if store.saves != 0 {
				t.Errorf("the store was saved %d time(s), want none", store.saves)
			}
		})
	}
}

// Known is the registry the program knows of when it asks whose memory it holds: the one that is kept, or the
// Defaults of a machine where none is. It does not ask whether the registry is there before reading it.
func TestVaults_Known_IsTheRegistryOrTheDefaultsOfAMachineWithNone(t *testing.T) {
	absent := &fakeStore{}
	got, err := vaultsOver(absent).Known()
	if err != nil || len(got.Vaults) != 1 || got.Vaults["the-default"].Path != "/vaults/default" {
		t.Errorf("Known of a machine with no registry = (%+v, %v), want the Defaults", got, err)
	}
	if absent.saves != 0 || absent.exists != 0 {
		t.Errorf("the store was saved %d time(s) and asked %d time(s) whether it exists, want a read and nothing else", absent.saves, absent.exists)
	}

	present := &fakeStore{registry: &Registry{Vaults: map[string]Entry{"another": {Path: "/vaults/another"}}}}
	got, err = vaultsOver(present).Known()
	if err != nil || len(got.Vaults) != 1 || got.Vaults["another"].Path != "/vaults/another" {
		t.Errorf("Known of a registry that is there = (%+v, %v), want its rows alone", got, err)
	}
}

func TestVaults_Known_AnswersAFailureToReadAsItWas(t *testing.T) {
	boom := errors.New("boom")

	if _, err := vaultsOver(&fakeStore{registry: &Registry{}, failLoad: boom}).Known(); !errors.Is(err, boom) {
		t.Errorf("Known = %v, want the failure, which is not taken for a machine with no registry", err)
	}
}

// The vault of a project that is named by the flag or the environment is that vault, and the registry is neither
// read nor made: a command that is told where the vault is has no business creating a file in the user's home.
func TestVaults_ToWrite_ANamedVaultLeavesTheRegistryAlone(t *testing.T) {
	for name, tc := range map[string]struct{ flag, env, want string }{
		"the flag":        {flag: "/vaults/flag", want: "/vaults/flag"},
		"the environment": {env: "/vaults/env", want: "/vaults/env"},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			v := vaultsOver(store)
			v.Environment.Vault = tc.env

			got, err := v.ToWrite("the-default", tc.flag)

			if err != nil || got != tc.want {
				t.Fatalf("ToWrite = (%q, %v), want %q", got, err, tc.want)
			}
			if store.exists+store.loads+store.saves != 0 {
				t.Errorf("the store was asked %d question(s), %d load(s) and %d save(s), want it left alone", store.exists, store.loads, store.saves)
			}
		})
	}
}

// A machine that has no registry gets one, written from the Defaults, before the row is looked up: the first
// project of the machine has its vault the first time it is asked for.
func TestVaults_ToWrite_SeedsTheRegistryFromTheDefaultsWhenThereIsNone(t *testing.T) {
	store := &fakeStore{}

	got, err := vaultsOver(store).ToWrite("the-default", "")

	if err != nil || got != "/vaults/default" {
		t.Fatalf("ToWrite = (%q, %v), want the default's vault", got, err)
	}
	if store.saves != 1 || store.registry == nil || store.registry.Vaults["the-default"] != (Entry{Path: "/vaults/default", Seeded: true}) {
		t.Errorf("the store was saved %d time(s) with %+v, want once with the Defaults", store.saves, store.registry)
	}
}

// A registry that is there is not seeded again, whatever it holds: a person who deleted the default row has said
// the project is not configured, and it is not.
func TestVaults_ToWrite_NeverTouchesARegistryThatIsThere(t *testing.T) {
	store := &fakeStore{registry: &Registry{Vaults: map[string]Entry{"another": {Path: "/vaults/another"}}, Origin: "/registry/vaults.json"}}

	_, err := vaultsOver(store).ToWrite("the-default", "")

	if !errors.Is(err, ErrVaultNotConfigured) {
		t.Errorf("ToWrite = %v, want ErrVaultNotConfigured (the deleted row is not seeded again)", err)
	}
	if store.saves != 0 {
		t.Errorf("the store was saved %d time(s), want none", store.saves)
	}
	if got, err := vaultsOver(store).ToWrite("another", ""); err != nil || got != "/vaults/another" {
		t.Errorf("ToWrite of the row there = (%q, %v), want its vault", got, err)
	}
}

// Every failure of the store is the answer, as the store said it, and what comes after it does not happen: a
// registry that cannot be told about is not seeded, one that cannot be written is not read, and one that cannot be
// read is not looked in.
func TestVaults_ToWrite_AnswersTheFailuresOfTheStoreAsTheyAre(t *testing.T) {
	boom := errors.New("boom")

	t.Run("it cannot be told whether there is a registry", func(t *testing.T) {
		store := &fakeStore{failExists: boom}
		if _, err := vaultsOver(store).ToWrite("the-default", ""); !errors.Is(err, boom) || store.saves+store.loads != 0 {
			t.Errorf("ToWrite = %v with %d save(s) and %d load(s), want the failure and nothing after it", err, store.saves, store.loads)
		}
	})
	t.Run("it cannot be written", func(t *testing.T) {
		store := &fakeStore{failSave: boom}
		if _, err := vaultsOver(store).ToWrite("the-default", ""); !errors.Is(err, boom) || store.loads != 0 {
			t.Errorf("ToWrite = %v with %d load(s), want the failure and no read", err, store.loads)
		}
	})
	t.Run("it cannot be read", func(t *testing.T) {
		store := &fakeStore{registry: &Registry{}, failLoad: boom}
		if _, err := vaultsOver(store).ToWrite("the-default", ""); !errors.Is(err, boom) {
			t.Errorf("ToWrite = %v, want the failure", err)
		}
	})
}
