package register

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// memoryState is an InstallStateStore that keeps the records in memory, by state directory, and remembers what it
// was asked: the state directory a Writer works over is the store's business, not a path the Writer joins.
type memoryState struct {
	records map[string]*InstallState
	loads   []string
	saves   []string
	// failLoad and failSave are the errors the store answers with, when set.
	failLoad, failSave error
}

var _ InstallStateStore = (*memoryState)(nil)

func newMemoryState() *memoryState { return &memoryState{records: map[string]*InstallState{}} }

func (s *memoryState) Load(stateDir string) (*InstallState, error) {
	s.loads = append(s.loads, stateDir)
	if s.failLoad != nil {
		return nil, s.failLoad
	}
	if record, ok := s.records[stateDir]; ok {
		// A copy: a store hands out what it holds, not the Writer's working value.
		copied := &InstallState{Schema: record.Schema, Targets: map[string]TargetRecord{}}
		for target, rec := range record.Targets {
			copied.Targets[target] = rec
		}
		return copied, nil
	}
	return &InstallState{Schema: installStateSchemaVersion, Targets: map[string]TargetRecord{}}, nil
}

func (s *memoryState) Save(stateDir string, state *InstallState) error {
	s.saves = append(s.saves, stateDir)
	if s.failSave != nil {
		return s.failSave
	}
	s.records[stateDir] = state
	return nil
}

func seedConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return path
}

// The record of what was installed is the store's: the Writer asks it for the record of the state directory it was
// given and keeps the new one there, and writes no file of its own for it.
func TestWriter_KeepsTheRecordInTheStoreItWasGivenForTheStateDirectoryItWasGiven(t *testing.T) {
	configRoot, stateDir := t.TempDir(), t.TempDir()
	seedConfig(t, configRoot, claudeConfigFileName, `{"mcpServers":{}}`)
	store := newMemoryState()

	if err := (Writer{State: store}).RegisterClaude(configRoot, stateDir, "/opt/bin/longterm-mem"); err != nil {
		t.Fatalf("RegisterClaude: %v", err)
	}

	if len(store.loads) != 1 || store.loads[0] != stateDir || len(store.saves) != 1 || store.saves[0] != stateDir {
		t.Errorf("the store was asked to load %v and to save %v, want the state directory %q once each", store.loads, store.saves, stateDir)
	}
	record, ok := store.records[stateDir].Get(claudeTarget)
	if !ok || record.Fingerprint == "" {
		t.Errorf("the store holds %+v for the state directory, want a fingerprint recorded for claude", store.records[stateDir])
	}
	if _, err := os.Stat(filepath.Join(stateDir, installStateFileName)); !os.IsNotExist(err) {
		t.Errorf("a file was written for the record (stat error %v), want the store to be the only place it is kept", err)
	}
}

// What the store holds is what makes a second register a no-op and an unregister a removal, whatever it is held in.
func TestWriter_ReadsTheRecordFromTheStoreToDecideWhatToDo(t *testing.T) {
	configRoot, stateDir := t.TempDir(), t.TempDir()
	configPath := seedConfig(t, configRoot, claudeConfigFileName, `{"mcpServers":{}}`)
	store := newMemoryState()
	writer := Writer{State: store}

	for i := 0; i < 2; i++ {
		if err := writer.RegisterClaude(configRoot, stateDir, "/opt/bin/longterm-mem"); err != nil {
			t.Fatalf("RegisterClaude #%d: %v", i+1, err)
		}
	}
	if len(store.saves) != 1 {
		t.Errorf("the record was saved %d times by two registers of the same entry, want once: the second is a no-op", len(store.saves))
	}

	outcome, err := writer.Unregister(claudeTarget, configRoot, stateDir)
	if err != nil || outcome != UnregisterRemoved {
		t.Fatalf("Unregister = (%v, %v), want UnregisterRemoved", outcome, err)
	}
	if _, ok := store.records[stateDir].Get(claudeTarget); ok {
		t.Error("the record of claude is still in the store after the unregister")
	}
	if got, _ := os.ReadFile(configPath); strings.Contains(string(got), "longterm-mem") {
		t.Errorf("the entry is still in the config after the unregister:\n%s", got)
	}
}

// A record that cannot be read stops the write before the runtime's configuration is touched: with nothing to say
// what is ours, nothing is written, and the failure is the store's own, named by the direction that asked.
func TestWriter_ARecordThatCannotBeReadStopsTheWriteBeforeTheConfigIsTouched(t *testing.T) {
	boom := errors.New("the record is unreadable")

	t.Run("register", func(t *testing.T) {
		configRoot := t.TempDir()
		configPath := seedConfig(t, configRoot, claudeConfigFileName, `{"mcpServers":{}}`)
		err := (Writer{State: &memoryState{records: map[string]*InstallState{}, failLoad: boom}}).RegisterClaude(configRoot, t.TempDir(), "/opt/bin/longterm-mem")

		if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "register: claude: ") {
			t.Errorf("RegisterClaude = %v, want the store's failure, named register: claude", err)
		}
		if got, _ := os.ReadFile(configPath); string(got) != `{"mcpServers":{}}` {
			t.Errorf("the config changed to %q though the record could not be read", got)
		}
	})
	t.Run("unregister", func(t *testing.T) {
		configRoot := t.TempDir()
		seedConfig(t, configRoot, claudeConfigFileName, `{"mcpServers":{}}`)
		_, err := (Writer{State: &memoryState{records: map[string]*InstallState{}, failLoad: boom}}).Unregister(claudeTarget, configRoot, t.TempDir())

		if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "unregister: claude: ") {
			t.Errorf("Unregister = %v, want the store's failure, named unregister: claude", err)
		}
	})
}

// A Writer with no store writes nothing, and says why, for every way into it: a config entry with no record of it is
// the shape every later run refuses. It says what is missing in the package's own form, the one layer that names the
// command and the target ("register: claude: ..." / "unregister: claude: ...") around words that name neither, and
// the words say what happened to the config.
func TestWriter_WithoutAStoreWritesNothing(t *testing.T) {
	for _, target := range []string{claudeTarget, opencodeTarget, codexTarget, piTarget} {
		t.Run(target, func(t *testing.T) {
			configRoot := t.TempDir()
			before := map[string]string{
				claudeTarget:   `{"mcpServers":{}}`,
				opencodeTarget: `{"mcp":{}}`,
				codexTarget:    "",
				piTarget:       `{"mcpServers":{}}`,
			}[target]
			name := map[string]string{claudeTarget: claudeConfigFileName, opencodeTarget: opencodeConfigFileName, codexTarget: codexConfigFileName, piTarget: piConfigFileName}[target]
			path := seedConfig(t, configRoot, name, before)

			err := (Writer{}).Register(target, configRoot, t.TempDir(), "/opt/bin/longterm-mem")
			_, unregErr := (Writer{}).Unregister(target, configRoot, t.TempDir())

			if !errors.Is(err, errNoInstallState) || !errors.Is(unregErr, errNoInstallState) {
				t.Errorf("Register = %v and Unregister = %v, want both to say the writer has no store", err, unregErr)
			}
			const words = "no install-state store was given, so nothing was recorded and nothing was written"
			if want := "register: " + target + ": " + words; err == nil || err.Error() != want {
				t.Errorf("Register = %v, want %q", err, want)
			}
			if want := "unregister: " + target + ": " + words; unregErr == nil || unregErr.Error() != want {
				t.Errorf("Unregister = %v, want %q", unregErr, want)
			}
			if got, _ := os.ReadFile(path); string(got) != before {
				t.Errorf("the config changed to %q though the writer had nowhere to record it", got)
			}
		})
	}
}

// Register is the writer of the runtime the target names, and a target it does not know is named in its refusal.
func TestWriter_RegisterDispatchesToTheWriterOfTheTargetAndRefusesAnUnknownOne(t *testing.T) {
	for target, name := range map[string]string{claudeTarget: claudeConfigFileName, opencodeTarget: opencodeConfigFileName, codexTarget: codexConfigFileName, piTarget: piConfigFileName} {
		t.Run(target, func(t *testing.T) {
			configRoot := t.TempDir()
			seedConfig(t, configRoot, name, map[string]string{claudeTarget: `{"mcpServers":{}}`, opencodeTarget: `{"mcp":{}}`, codexTarget: "", piTarget: `{"mcpServers":{}}`}[target])
			store := newMemoryState()

			if err := (Writer{State: store}).Register(target, configRoot, "state", "/opt/bin/longterm-mem"); err != nil {
				t.Fatalf("Register(%s): %v", target, err)
			}

			if _, ok := store.records["state"].Get(target); !ok {
				t.Errorf("Register(%s) recorded %+v, want a record for the target it was asked for", target, store.records["state"])
			}
		})
	}

	err := (Writer{State: newMemoryState()}).Register("bogus", t.TempDir(), "state", "/opt/bin/longterm-mem")
	if err == nil || err.Error() != "register: bogus: unknown target" {
		t.Errorf("Register(bogus) = %v, want %q", err, "register: bogus: unknown target")
	}
}

// The file store keeps the record where it has always been, install-state.json in the state directory, in the words
// of LoadInstallState and Save, and a state directory that has none yet is an empty record.
func TestFileInstallState_KeepsTheRecordInInstallStateJSONOfTheStateDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	var store FileInstallState

	empty, err := store.Load(stateDir)
	if err != nil || len(empty.Targets) != 0 {
		t.Fatalf("Load of a state directory with no record = (%+v, %v), want an empty record", empty, err)
	}

	empty.Set(claudeTarget, TargetRecord{Fingerprint: "abc"})
	if err := store.Save(stateDir, empty); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, installStateFileName)); err != nil {
		t.Errorf("the record is not in %s of the state directory: %v", installStateFileName, err)
	}
	loaded, err := store.Load(stateDir)
	if err != nil || loaded.Targets[claudeTarget].Fingerprint != "abc" {
		t.Errorf("Load after Save = (%+v, %v), want the record that was saved", loaded, err)
	}
}
