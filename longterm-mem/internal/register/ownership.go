package register

import (
	"bytes"
	"encoding/json"
)

// This file holds the ONE derivation that answers "is the entry currently
// on disk one longterm-mem wrote?" — Decide's entryOwned input, and the
// whole of the difference between adopting a lost ownership record and
// refusing the user access to their own installation (D9's adopt row).
//
// It is deliberately not Fingerprint. Fingerprint hashes exact bytes,
// which is right for install-state's record — that record's job is to say
// what longterm-mem last WROTE, and the writer wrote exact bytes. It is
// wrong for the ownership question, because the bytes on disk are not
// only ours to shape: a runtime's own settings UI, a config formatter,
// `jq . > config`, or a human tidying the file all re-serialize the entry
// without changing a thing about what it means. Under a raw-byte
// comparison every one of those turns longterm-mem's own entry into a
// stranger's, and `register` answers exit 6 over an entry it wrote itself
// — the exact lockout the adopt row exists to prevent, reintroduced by
// the comparison used to detect it.
//
// The second reason is agreement. engine/runtime's read-only
// LongtermMemAdapter answers the same ownership question about the same
// files for `doctor`/`status`, and it canonicalizes: JSON entries are
// decoded and re-marshaled (encoding/json sorts object keys and emits no
// insignificant whitespace) and codex sections are compared with trailing
// newlines trimmed. Two derivations of one fact that disagree do not have
// a "stricter" side and a "looser" side; they have a side that is wrong
// about a real file. This module and that adapter must call the same
// entry ours, so the functions below reproduce that adapter's
// canonicalization exactly (see doc.go).
//
// Nothing about CONTENT is canonicalized away. A different command, a
// different args list, an extra field ours never writes: each of those
// changes the canonical form too, so it is still refused and still left
// byte-identical (ownership_test.go pins all three).

// ownershipFingerprintJSON returns entry's canonical ownership
// fingerprint: the hash of its decode-then-remarshal form, so object key
// order and every byte of insignificant whitespace drop out.
//
// An entry that is not valid JSON has NO fingerprint (""), never a hash of
// its raw bytes. Callers compare two fingerprints for equality, and a
// blob that cannot be parsed is exactly the case where equality must not
// be reachable: two identically-corrupt entries are not evidence of
// ownership. Callers therefore also reject the empty fingerprint before
// comparing, so "" can never match "".
func ownershipFingerprintJSON(entry []byte) string {
	var decoded interface{}
	if err := json.Unmarshal(entry, &decoded); err != nil {
		return ""
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return ""
	}
	return Fingerprint(canonical)
}

// ownershipFingerprintTOML returns a codex section's canonical ownership
// fingerprint: the hash of its text with trailing newlines trimmed,
// mirroring engine/runtime's tomlSectionFingerprint/ownedCodexFingerprint
// pair byte for byte.
//
// Trailing newlines are the one thing that legitimately differs between
// the section template longterm-mem writes (always newline-terminated)
// and the same section sitting at the end of a file an editor saved
// without a final newline. Everything else inside the section is compared
// verbatim: this is a line-oriented format with no parse step on the
// write side (tomlsplice.go), so there is no canonical form to normalize
// toward beyond that, and inventing one would start accepting sections
// longterm-mem did not write.
func ownershipFingerprintTOML(section []byte) string {
	return Fingerprint(bytes.TrimRight(section, "\n"))
}
