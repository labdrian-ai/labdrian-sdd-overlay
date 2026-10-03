package reviewreceipt

// acknowledgeMarker is the exact substring Service.CheckCommand matches inside
// the command of a tool call to recognize the acknowledge-approved invocation. The
// hook never parses the lineage or executes anything of its own beyond
// Capture -- it only string-matches this marker, per the threat matrix
// (Subprocess boundary): a look-alike command (e.g. `echo
// "gentle-ai review acknowledge-approved"` in a comment or log line) still
// matches, which is intentionally conservative -- a false-positive capture
// is harmless (idempotent), while a false negative would let an
// acknowledgement burn an uncaptured receipt.
const acknowledgeMarker = "gentle-ai review acknowledge-approved"
