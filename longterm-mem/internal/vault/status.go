package vault

// Vault status values returned by subprocess-backed vault operations.
// StatusNotProvisioned is shared beyond Retrieve: the index provisioning
// path (slice 3a) reuses the same never-indexed sentinel meaning (D12).
const (
	StatusOK             = "ok"
	StatusNotProvisioned = "not_provisioned"
)

// notProvisionedExitCode is the exit code the vault's Python scripts use to
// report a never-indexed/unprovisioned vault.
const notProvisionedExitCode = 10

// statusForExitCode maps a subprocess exit code to a vault status. Exit 0
// maps to StatusOK; the not-provisioned sentinel maps to
// StatusNotProvisioned rather than a generic error (R-024). mapped is
// false for any other exit code — callers treat those as a subprocess
// failure, not a status. Extracted out of retrieve.go so the index
// provisioning path (slice 3a) can reuse the same mapping.
func statusForExitCode(exitCode int) (status string, mapped bool) {
	switch exitCode {
	case 0:
		return StatusOK, true
	case notProvisionedExitCode:
		return StatusNotProvisioned, true
	default:
		return "", false
	}
}
