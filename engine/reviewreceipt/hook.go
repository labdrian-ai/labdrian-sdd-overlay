package reviewreceipt

// acknowledgeMarker is the exact substring Service.CheckCommand matches inside the command of a
// tool call to recognize the acknowledge-approved invocation. The hook never parses the lineage or
// executes anything of its own beyond Capture -- it only string-matches this marker, per the threat
// matrix (Subprocess boundary).
//
// The match is textual on purpose, and the owner decided to keep it so (Phase 9 sweep): the
// invocation reaches a shell in forms no parser of the command line could follow without
// becoming a shell -- inside a heredoc (`bash <<'EOF'` ... `EOF`), a `sh -c "..."` string, a
// script written and then run, a variable or an `eval` -- and a text match sees the phrase in all
// of them. The cost is that a look-alike (e.g. `echo "gentle-ai review acknowledge-approved"` in a
// comment or log line) matches too, and that is accepted: a false-positive capture is harmless
// (idempotent), while a false negative would let an acknowledgement burn an uncaptured receipt.
// Narrowing the match to commands it can parse would trade the second error for the first, so a
// change to it is a change to the hook's contract and the owner's to make. The tests that hold
// the decision are TestCheckCommandRecognizesTheAcknowledgementWhereverItSitsInTheCommand and the
// golden cases of the hook in engine/cmd (a look-alike, a heredoc).
const acknowledgeMarker = "gentle-ai review acknowledge-approved"
