package runtime

// Reasons reported by the longterm-mem status matrix. Each is a distinct,
// named cause so an operator reading a result knows exactly what it means
// instead of just that something is — see design.md D4 and R-014.
//
// The first four are the PARTIAL reasons: real defects, each one a state
// only a broken or drifted installation can produce. The last two are
// SUPPORTED reasons: nothing is registered for that runtime and nothing is
// wrong with that.
const (
	LongtermMemReasonConfigRootUnresolvable = "config root unresolvable"
	LongtermMemReasonMissingBinary          = "missing binary"
	LongtermMemReasonRecordWithoutEntry     = "record without entry"
	LongtermMemReasonEntryWithoutRecord     = "entry without record (unmanaged)"
	LongtermMemReasonFingerprintDrift       = "fingerprint drift"
	LongtermMemReasonRuntimeNotInstalled    = "runtime not installed"
	LongtermMemReasonNotRegistered          = "not registered"
)

// LongtermMemComponentState is the pure input to the status matrix: every
// signal EvaluateLongtermMemComponentStatus needs, gathered up front so the
// decision itself has no I/O and is fully table-testable. Exported so the
// status matrix — the substance of 10a.1 — is directly unit-testable
// without going through real filesystem fixtures.
type LongtermMemComponentState struct {
	RootResolvable bool
	BinaryPresent  bool
	// RuntimePresent is whether THIS runtime's own configuration file
	// exists on disk — i.e. whether the runtime is installed on this
	// machine at all. It is deliberately separate from RootResolvable,
	// which only says the config ROOT could be resolved (a machine-wide
	// "does HOME resolve" signal shared by all three runtimes).
	RuntimePresent   bool
	RecordPresent    bool
	EntryPresent     bool
	FingerprintMatch bool
}

// EvaluateLongtermMemComponentStatus is the status matrix's substance
// (10a.1): a total function from observed signals to (status, reason).
// Every PARTIAL outcome carries ONE of the four distinct named reasons the
// spec names, so an operator reading "partial" always knows exactly which
// condition produced it.
//
// The two outcomes where nothing is registered for a runtime are SUPPORTED,
// not partial, and that is the deliberate correction this matrix carries: a
// runtime that is absent from the machine, or present but never registered,
// is not a defect the component can diagnose. The engine reports OBSERVED
// state; asserting user INTENT — which targets were asked for — belongs to
// the caller that knows it. Reporting them as partial made a flawless
// machine that simply does not run all three runtimes permanently unhealthy,
// and it is also what openspec/specs/longterm-mem-mcp-registration/spec.md's
// "Multi-Target Expansion Skips Runtimes That Are Not Installed" requires:
// an absent runtime must not fail a run on its account.
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
	case !s.RuntimePresent:
		// No record, no entry, and no config file: this runtime is simply
		// not on this machine.
		return CapabilitySupported, LongtermMemReasonRuntimeNotInstalled
	default:
		// No record and no entry, but the runtime IS here: longterm-mem was
		// never registered with it.
		return CapabilitySupported, LongtermMemReasonNotRegistered
	}
}
