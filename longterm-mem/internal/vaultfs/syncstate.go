package vaultfs

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// syncStateSchema is the version of the record RecordSyncCompleted writes.
const syncStateSchema = 1

// RecordSyncCompleted makes the vault's sync state say that a sync completed at the instant given, in RFC 3339
// and in the zone the instant has. The record is a JSON object holding the schema version and that instant,
// indented by two spaces and ended by a newline, and replaces the file atomically as the module's other files in
// the vault, keeping the mode of the one that is there and creating a new one owner-only; a failure leaves the
// record as it was.
//
// The errors name what failed. The failure to encode the record cannot happen to a record of an int and a
// string, and says so in the words it has always had.
func (v *Vault) RecordSyncCompleted(at time.Time) error {
	record := struct {
		Schema              int    `json:"schema"`
		LastSyncCompletedAt string `json:"last_sync_completed_at"`
	}{Schema: syncStateSchema, LastSyncCompletedAt: at.Format(time.RFC3339)}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync-state record: %w", err)
	}
	if err := writeFileAtomic(v.layout.Path(vaultlayout.SyncStateFile), append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// LastSyncCompletedAt returns the completion time the vault's sync state holds, as it holds it. A vault with no
// record, and a record that holds no completion time (an object without the field, an empty one, the JSON null),
// have none: the empty string and no error. A record that cannot be read, or cannot be understood (it is not
// JSON, or its completion is not a string), is an error that names the file and says whether it could not read it
// or could not parse it.
func (v *Vault) LastSyncCompletedAt() (string, error) {
	full := v.layout.Path(vaultlayout.SyncStateFile)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", full, err)
	}
	var record struct {
		LastSyncCompletedAt string `json:"last_sync_completed_at"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return "", fmt.Errorf("parse %s: %w", full, err)
	}
	return record.LastSyncCompletedAt, nil
}
