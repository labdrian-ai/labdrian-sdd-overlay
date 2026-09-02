package runtime

// Reasons reported by the longterm-mem status matrix. Each is a distinct,
// named cause so an operator reading a "partial" result knows exactly what
// is wrong instead of just that something is — see design.md D4 and R-014.
const (
	LongtermMemReasonConfigRootUnresolvable = "config root unresolvable"
	LongtermMemReasonMissingBinary          = "missing binary"
	LongtermMemReasonRecordWithoutEntry     = "record without entry"
	LongtermMemReasonEntryWithoutRecord     = "entry without record (unmanaged)"
	LongtermMemReasonFingerprintDrift       = "fingerprint drift"
	LongtermMemReasonNotInstalled           = "not installed"
)

// LongtermMemComponentState is the pure input to the status matrix: every
// signal EvaluateLongtermMemComponentStatus needs, gathered up front so the
// decision itself has no I/O and is fully table-testable. Exported so the
// status matrix — the substance of 10a.1 — is directly unit-testable
// without going through real filesystem fixtures.
type LongtermMemComponentState struct {
	RootResolvable   bool
	BinaryPresent    bool
	RecordPresent    bool
	EntryPresent     bool
	FingerprintMatch bool
}

// EvaluateLongtermMemComponentStatus is the status matrix's substance
// (10a.1): a total function from observed signals to (status, reason).
// Every partial outcome carries ONE of four distinct named reasons (plus a
// fifth "not installed" default for total-function coverage, which is not
// one of the four the spec names) so an operator reading "partial" always
// knows exactly which condition produced it.
//
// It lives in its own file, separated from the adapter that gathers those
// signals, because it is the one part of the component with no I/O at all:
// a decision table that can be reviewed and exhaustively tested on its own
// terms, without a filesystem fixture in sight.
func EvaluateLongtermMemComponentStatus(s LongtermMemComponentState) (CapabilityStatus, string) {
	if !s.RootResolvable {
		return CapabilityUnsupported, LongtermMemReasonConfigRootUnresolvable
	}
	if !s.BinaryPresent {
		return CapabilityPartial, LongtermMemReasonMissingBinary
	}
	switch {
	case s.RecordPresent && !s.EntryPresent:
		return CapabilityPartial, LongtermMemReasonRecordWithoutEntry
	case !s.RecordPresent && s.EntryPresent:
		return CapabilityPartial, LongtermMemReasonEntryWithoutRecord
	case s.RecordPresent && s.EntryPresent && !s.FingerprintMatch:
		return CapabilityPartial, LongtermMemReasonFingerprintDrift
	case s.RecordPresent && s.EntryPresent && s.FingerprintMatch:
		return CapabilitySupported, ""
	default:
		return CapabilityPartial, LongtermMemReasonNotInstalled
	}
}
