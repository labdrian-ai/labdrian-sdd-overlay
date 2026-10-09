// Package app holds the use case of `engine propagate` (Phase 9 unit H28): ensure that the
// registry of a project holds the scoped row of a contract, and be sure the row stayed. It takes
// a typed request and the two ports it needs, a RegistryStore and a ContractSource, and answers
// with a typed Outcome or with an error that says what was refused. It reads no argument vector,
// prints nothing, exits nowhere and takes no lock: parsing the command line, telling a person
// what happened and holding the lock of the registry are the work of the command in engine/cmd,
// which is the one place that knows the words of a terminal and the order in which they are
// owed. The same request over the same ports gives the same answer, whoever asks it.
//
// There are two ways in. Propagate is one read-decide-write pass and says what it found. The
// verified one, PropagateVerified, is the loop the command runs: it asks Propagate again while
// the evidence says a foreign writer is racing the registry, up to MaxAttempts times, and
// reads every write back. It decides on what each pass found (the Outcome), never on what the
// pass would have printed.
package app
