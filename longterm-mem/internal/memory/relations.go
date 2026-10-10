package memory

// Edge is one accepted relation edge between two observations, identified by their sync ids: the
// relation ledger keys its source and target on SyncID, not on the integer id.
type Edge struct {
	Relation     string
	SourceSyncID string
	TargetSyncID string
}
