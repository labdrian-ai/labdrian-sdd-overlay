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

// The vault of a project that is named by the flag or the environment is that vault, and the registry is neither
// read nor made: a command that is told where the vault is has no business creating a file in the user's home.
func TestVaults_Resolve_ANamedVaultLeavesTheRegistryAlone(t *testing.T) {
	for name, tc := range map[string]struct{ flag, env, want string }{
		"the flag":        {flag: "/vaults/flag", want: "/vaults/flag"},
		"the environment": {env: "/vaults/env", want: "/vaults/env"},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			v := vaultsOver(store)
			v.Environment.Vault = tc.env

			got, err := v.Resolve("the-default", tc.flag)

			if err != nil || got != tc.want {
				t.Fatalf("Resolve = (%q, %v), want %q", got, err, tc.want)
			}
			if store.exists+store.loads+store.saves != 0 {
				t.Errorf("the store was asked %d question(s), %d load(s) and %d save(s), want it left alone", store.exists, store.loads, store.saves)
			}
		})
	}
}

// A machine that has no registry gets one, written from the Defaults, before the row is looked up: the first
// project of the machine has its vault the first time it is asked for.
func TestVaults_Resolve_SeedsTheRegistryFromTheDefaultsWhenThereIsNone(t *testing.T) {
	store := &fakeStore{}

	got, err := vaultsOver(store).Resolve("the-default", "")

	if err != nil || got != "/vaults/default" {
		t.Fatalf("Resolve = (%q, %v), want the default's vault", got, err)
	}
	if store.saves != 1 || store.registry == nil || store.registry.Vaults["the-default"] != (Entry{Path: "/vaults/default", Seeded: true}) {
		t.Errorf("the store was saved %d time(s) with %+v, want once with the Defaults", store.saves, store.registry)
	}
}

// A registry that is there is not seeded again, whatever it holds: a person who deleted the default row has said
// the project is not configured, and it is not.
func TestVaults_Resolve_NeverTouchesARegistryThatIsThere(t *testing.T) {
	store := &fakeStore{registry: &Registry{Vaults: map[string]Entry{"another": {Path: "/vaults/another"}}, Origin: "/registry/vaults.json"}}

	_, err := vaultsOver(store).Resolve("the-default", "")

	if !errors.Is(err, ErrVaultNotConfigured) {
		t.Errorf("Resolve = %v, want ErrVaultNotConfigured (the deleted row is not seeded again)", err)
	}
	if store.saves != 0 {
		t.Errorf("the store was saved %d time(s), want none", store.saves)
	}
	if got, err := vaultsOver(store).Resolve("another", ""); err != nil || got != "/vaults/another" {
		t.Errorf("Resolve of the row there = (%q, %v), want its vault", got, err)
	}
}

// Every failure of the store is the answer, as the store said it, and what comes after it does not happen: a
// registry that cannot be told about is not seeded, one that cannot be written is not read, and one that cannot be
// read is not looked in.
func TestVaults_Resolve_AnswersTheFailuresOfTheStoreAsTheyAre(t *testing.T) {
	boom := errors.New("boom")

	t.Run("it cannot be told whether there is a registry", func(t *testing.T) {
		store := &fakeStore{failExists: boom}
		if _, err := vaultsOver(store).Resolve("the-default", ""); !errors.Is(err, boom) || store.saves+store.loads != 0 {
			t.Errorf("Resolve = %v with %d save(s) and %d load(s), want the failure and nothing after it", err, store.saves, store.loads)
		}
	})
	t.Run("it cannot be written", func(t *testing.T) {
		store := &fakeStore{failSave: boom}
		if _, err := vaultsOver(store).Resolve("the-default", ""); !errors.Is(err, boom) || store.loads != 0 {
			t.Errorf("Resolve = %v with %d load(s), want the failure and no read", err, store.loads)
		}
	})
	t.Run("it cannot be read", func(t *testing.T) {
		store := &fakeStore{registry: &Registry{}, failLoad: boom}
		if _, err := vaultsOver(store).Resolve("the-default", ""); !errors.Is(err, boom) {
			t.Errorf("Resolve = %v, want the failure", err)
		}
	})
}
