package settings

import "strings"

// owner is the binary path whose entries the functions below add and remove. Every entry the
// overlay writes is told from anyone else's by that path together with the token of its family,
// so the path is the one thing a merge needs besides the object it works on.
type owner struct{ hookCommand string }

// embeddedDesignName is the engine-owned managed contract that propagates the
// anti-generic-design guard (countering the model's default "Claude/SaaS
// look" design bias). It rides the same propagate/gate-task machinery as the
// minimalism contract but writes a DISTINCT registry block.
const embeddedDesignName = "anti-generic-design"

// mergeHooks inserts our hook entries if not already present. Returns true if
// any change was made. It installs TWO pairs: the minimalism-contract pair
// and the anti-generic-design pair. Each pair is deduped by its own identity
// so both coexist.
//
// Identity matching in this file is entirely name-based
// (isMinimalismEntry / isDesignEntry), not a generic marker or
// command-prefix scan: dropping a contract's support means dropping its
// identity const, its isXEntry predicate, and its builder functions, and
// mergeHooks simply stops writing that pair. Because that alone would also
// stop Uninstall from ever recognizing a pair it no longer installs,
// removeHooks additionally matches the fixed legacyIdentities list below —
// see its doc comment for why that is a deliberate backward-compat
// exception, not a general-purpose mechanism.
func (m owner) mergeHooks(root map[string]interface{}) bool {
	hooks := ensureHooksMap(root)
	changed := false

	// The minimalism-contract pair, the anti-generic-design pair, the SessionEnd sync-trigger entry
	// and the PreToolUse/Bash review-receipt entry, each deduped by its own identity so they
	// coexist.
	for _, family := range []hookFamily{minimalismFamily, designFamily, syncTriggerFamily, reviewReceiptFamily} {
		if family.merge(hooks, m.hookCommand) {
			changed = true
		}
	}

	// PreToolUse shaper clearance guard entries (identity: binary path +
	// shaper guard token), one for Bash and one for the file tools, plus the
	// permissions.deny backstop. A speed bump, not a security boundary.
	for _, matcher := range []string{"Bash", ShaperGuardFileToolMatcher} {
		if !hasEntryMatching(hooks, "PreToolUse", shaperGuardMatcher(m.hookCommand, matcher)) {
			appendHook(hooks, "PreToolUse", m.buildShaperGuardPreToolUseEntry(matcher))
			changed = true
		}
	}
	if !hasDenyRule(root, ShaperClearanceDenyRule) {
		perms, ok := root["permissions"].(map[string]interface{})
		if !ok {
			perms = map[string]interface{}{}
			root["permissions"] = perms
		}
		deny, _ := perms["deny"].([]interface{})
		perms["deny"] = append(deny, ShaperClearanceDenyRule)
		changed = true
	}

	// Projection family (identity: binary path + projection token): the
	// UserPromptSubmit context projection and the two PreToolUse gates.
	if m.mergeProjection(hooks) {
		changed = true
	}

	// Approve guard family (identity: binary path + approve guard token): the
	// two PreToolUse entries that deny the agent running skills approve or
	// writing the approval record. A speed bump, not a security boundary.
	if m.mergeApproveGuard(hooks) {
		changed = true
	}

	root["hooks"] = hooks
	return changed
}

// isShaperGuardEntry reports whether a hook entry is one of our shaper
// clearance guard entries.
func (m owner) isShaperGuardEntry(e interface{}) bool {
	return entryContainsBinary(e, m.hookCommand) && entryContainsBinary(e, LabdrianShaperGuardIdentity)
}

// legacyIdentities are the --embedded-contract identity tokens of hook pairs
// this overlay used to install and no longer does (skill-discovery-safety,
// review-projection-contract — both dropped because gentle-ai now covers
// them natively). mergeHooks never writes these anymore, but removeHooks
// still matches them so Uninstall stays idempotent for a machine that
// installed them under an older version of this overlay: without this list,
// a stale entry from a pre-upgrade install would no longer be recognized by
// any isXEntry predicate and Uninstall would silently leave it behind.
//
// This is a deliberate, fixed, hand-maintained list — not a generic
// marker/command-prefix scan. Retiring a contract's dedicated isXEntry
// predicate and builder functions is exactly the moment its identity token
// belongs here instead. TestRemoveHooksCleansUpLegacySafetyAndProjectionEntries
// (settings_test.go) pins that a stale entry using each of these tokens is
// still removed.
var legacyIdentities = []string{
	"--embedded-contract skill-discovery-safety",
	"--embedded-contract review-projection-contract",
}

// isLegacyEntry reports whether a hook entry is a leftover from a retired
// contract pair: it references our binary AND one of legacyIdentities.
func (m owner) isLegacyEntry(e interface{}) bool {
	for _, identity := range legacyIdentities {
		if entryContainsBinary(e, m.hookCommand) && entryContainsBinary(e, identity) {
			return true
		}
	}
	return false
}

// removeHooks removes our hook entries. Returns true if any change was made.
// Identity is Labdrian-owned entry shape: our minimalism, design, or
// sync-trigger entries, the review-receipt, shaper guard, projection, and
// approve guard entries, plus any stale entry matching legacyIdentities (see
// its doc comment), not merely any entry that happens to reference the same
// binary path. SessionEnd is scanned alongside UserPromptSubmit/PreToolUse
// so an owned sync-trigger entry there is removed the same way; Stop is
// never scanned because nothing owned is ever installed there.
func (m owner) removeHooks(root map[string]interface{}) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}

	changed := false
	for _, key := range []string{"UserPromptSubmit", "PreToolUse", "SessionEnd"} {
		entries, ok := hooks[key].([]interface{})
		if !ok {
			continue
		}
		var filtered []interface{}
		for _, e := range entries {
			if minimalismFamily.owns(e, m.hookCommand) || designFamily.owns(e, m.hookCommand) || syncTriggerFamily.owns(e, m.hookCommand) || reviewReceiptFamily.owns(e, m.hookCommand) || m.isShaperGuardEntry(e) || m.isProjectionEntry(e) || m.isApproveGuardEntry(e) || m.isLegacyEntry(e) {
				changed = true
				continue
			}
			filtered = append(filtered, e)
		}
		if len(filtered) == 0 {
			// Remove the key entirely rather than writing null or an empty
			// array — avoids structural noise in the emitted JSON.
			delete(hooks, key)
		} else {
			hooks[key] = filtered
		}
	}

	if perms, ok := root["permissions"].(map[string]interface{}); ok {
		if deny, ok := perms["deny"].([]interface{}); ok {
			var kept []interface{}
			for _, r := range deny {
				if r == ShaperClearanceDenyRule {
					changed = true
					continue
				}
				kept = append(kept, r)
			}
			if len(kept) == 0 {
				delete(perms, "deny")
			} else {
				perms["deny"] = kept
			}
			if len(perms) == 0 {
				delete(root, "permissions")
			}
		}
	}

	root["hooks"] = hooks
	return changed
}

// ensureHooksMap retrieves or creates the "hooks" map in root.
func ensureHooksMap(root map[string]interface{}) map[string]interface{} {
	if h, ok := root["hooks"].(map[string]interface{}); ok {
		return h
	}
	h := map[string]interface{}{}
	root["hooks"] = h
	return h
}

// hasEntryMatching reports whether hooks[key] already contains an entry for
// which the predicate returns true. Used to dedup each of our hook pairs by its
// own identity so adding a second pair does not collapse into the first.
func hasEntryMatching(hooks map[string]interface{}, key string, match func(interface{}) bool) bool {
	entries, ok := hooks[key].([]interface{})
	if !ok {
		return false
	}
	for _, e := range entries {
		if match(e) {
			return true
		}
	}
	return false
}

// entryContainsBinary returns true if the hook entry (an outer object) contains
// hookCommand as a substring in any of its inner hooks[].command strings.
func entryContainsBinary(e interface{}, hookCommand string) bool {
	em, ok := e.(map[string]interface{})
	if !ok {
		return false
	}
	innerHooks, ok := em["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, ih := range innerHooks {
		ihm, ok := ih.(map[string]interface{})
		if !ok {
			continue
		}
		if cmdStr, ok := ihm["command"].(string); ok {
			if strings.Contains(cmdStr, hookCommand) {
				return true
			}
		}
	}
	return false
}

// appendHook appends entry to hooks[key], creating the slice if absent.
func appendHook(hooks map[string]interface{}, key string, entry map[string]interface{}) {
	var entries []interface{}
	if existing, ok := hooks[key].([]interface{}); ok {
		entries = existing
	}
	hooks[key] = append(entries, entry)
}
