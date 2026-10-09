// Package fsadapter is the shaper's file-backed adapter: what the domain asks of the
// world through its ports (engine/shaper/ports.go), answered from files.
//
// ClearanceStore is the shaper.ClearanceStore: clearance records kept as one immutable
// JSON file each, under the user's state home. What a record is and means
// (shaper.ParseRecord, shaper.Verify) is the domain's, and pure. This package is the rest:
// where the files live, how they are read without following a symlink, and how a record is
// published so that no writer can replace another's. The state home is handed in by the
// composition root, which resolves it from the environment once (see
// engine/statestore.Home); this package reads no environment variable.
//
// The on-disk format and layout are a contract with every earlier version of the program,
// and they do not change here:
// <state home>/labdrian/shaper-clearance/<project_id>/<goal_id>/<handoff_sha256>.json, the
// record's own bytes exactly as offered, files 0600 in directories of mode 0700. The
// runtime deny guards refuse any path that contains guardmarkers.Store, which is this
// layout's fixed directory, and a test pins the two to each other. testdata holds a
// sequence of states recorded from the version that kept this code in the shaper package,
// and the tests read them, extend them, and write them again, byte for byte.
//
// It is built on engine/statestore (the state home, the directory chain, the no-follow
// read, the immutable publish). Its error messages are the ones the store printed when it
// lived in the shaper package ("clearance store: ..."), except that a failure to write or
// publish a record now carries engine/atomicfile's words. A platform without a no-follow
// open (anything but linux and darwin) is refused when the store is built, instead of
// reading a record it cannot vouch for.
//
// ContainedSource is the shaper.ContainedSource: the read of a handoff or a Goal named by
// a path inside a worktree. It opens the file once without following a symlink, checks the
// descriptor it holds, proves from that descriptor (not from the path) that the file lies
// inside the worktree root, and reads from the same descriptor, so nothing is proven of one
// file and read from another. It keeps the messages it had in the shaper package. The
// no-follow open is engine/statestore's, the one owner of it, shared with the clearance
// store above; the source's two test seams, a hook that lets a test race the file system
// between those steps and the opener itself (so a test runs a platform that has no
// no-follow open on any other), are fields of the struct rather than variables of the
// package. On a platform that cannot open without following a symlink or ask the kernel
// which path a descriptor names (anything but linux and darwin) it refuses every read when
// it is asked, and says, as it always did, that the contained read is unsupported there.
package fsadapter
