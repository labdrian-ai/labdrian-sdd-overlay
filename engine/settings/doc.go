// Package settings is the model of the hooks the overlay installs in Claude Code's settings.json,
// with no file in it.
//
// A Document is the parsed object. Merge adds the hook entries the overlay owns and Remove takes
// them out; both are PRESERVING (every other key and every foreign hook stays), IDEMPOTENT (a
// second Merge adds nothing), and report whether anything changed. The Has and Missing helpers say
// which of the overlay's hook families a decoded settings object holds. Reading the file, backing it
// up and replacing it atomically is the adapter's: settings/settingsfile.
//
// Every entry is told from anyone else's by the installed binary path together with one identity
// token inside hooks[].command, never by position or by an outer key. The families are the
// minimalism pair, the anti-generic-design pair, the SessionEnd sync-trigger entry, the
// PreToolUse/Bash review-receipt entry, the shaper clearance guard (with its permissions.deny
// rule), the workflow projection family and the skills approve guard.
//
// VERIFIED HOOK ENTRY SHAPE (Claude Code 2.1.185 / docs):
//
//	UserPromptSubmit entry:
//	  {"hooks":[{"type":"command","command":"<bash>"}]}
//
//	PreToolUse entry (with matcher):
//	  {"matcher":"Agent","hooks":[{"type":"command","command":"<bash>"}]}
//
// There are no outer "type" or "command" keys.
//
// Where the rest of what used to be in this package went: the file (read, backup, atomic write,
// links) is settings/settingsfile; the rules for the Claude root (it must be named and absolute;
// settings.json and bin/gentle-ai-overlay under it) are the Claude adapter's, in runtime
// (ClaudeAdapter.configPaths), the only code that has a root.
//
// The package imports nothing outside the pure standard library and guardmarkers, so it can be
// tested, and reasoned about, without a file system.
package settings
