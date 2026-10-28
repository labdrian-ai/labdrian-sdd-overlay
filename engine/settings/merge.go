package settings

// installedFamilies are the families Merge puts in settings.json, in install order. The order is
// the order of the entries a fresh install writes under one event, so it is part of the file's
// bytes; a family added here goes after the ones that exist.
var installedFamilies = []hookFamily{
	minimalismFamily,
	designFamily,
	syncTriggerFamily,
	reviewReceiptFamily,
	shaperGuardFamily,
	projectionFamily,
	approveGuardFamily,
	reviewStopFamily,
}

// retiredFamilies are the hook pairs this overlay used to install and no longer does
// (skill-discovery-safety, review-projection-contract: both dropped because gentle-ai now covers
// them natively). Merge never writes them, but Remove still finds them so uninstall stays idempotent
// for a machine that installed them under an older version of this overlay: without them, a stale
// entry from a pre-upgrade install would be recognized by no family and left behind. A retired
// family has an identity and nothing else.
//
// This is a deliberate, fixed, hand-maintained list, not a generic marker or command-prefix scan.
// Retiring a family is exactly the moment its identity belongs here instead of in installedFamilies.
// TestRemoveHooksCleansUpLegacySafetyAndProjectionEntries pins that a stale entry using each of
// these tokens is still removed.
var retiredFamilies = []hookFamily{
	{identity: "--embedded-contract skill-discovery-safety"},
	{identity: "--embedded-contract review-projection-contract"},
}

// ownedByAFamily reports whether a hook entry belongs to any family, installed or retired, for the
// binary at hookCommand. Identity is the family's shape, the binary path together with its token,
// not merely any entry that references the same binary path.
func ownedByAFamily(e interface{}, hookCommand string) bool {
	for _, families := range [][]hookFamily{installedFamilies, retiredFamilies} {
		for _, family := range families {
			if family.owns(e, hookCommand) {
				return true
			}
		}
	}
	return false
}

// mergeHooks inserts our hook entries if not already present. Returns true if any change was made.
// Each family is put in place as its upkeep says, so a family that keeps what is installed adds
// only what is missing and one that repairs makes itself exact; the permissions.deny backstop of
// the shaper clearance guard is added beside them.
//
// Identity matching is entirely by family (hookFamily.owns): dropping a family's support means
// moving it from installedFamilies to retiredFamilies, and Merge simply stops writing it while
// Remove still finds it.
func mergeHooks(root map[string]interface{}, hookCommand string) bool {
	hooks := ensureHooksMap(root)
	changed := false
	for _, family := range installedFamilies {
		if family.merge(hooks, hookCommand) {
			changed = true
		}
	}
	// A speed bump, not a security boundary.
	if addClearanceDenyRule(root) {
		changed = true
	}
	root["hooks"] = hooks
	return changed
}

// addClearanceDenyRule puts ShaperClearanceDenyRule in permissions.deny, creating the permissions
// object and the list if they are not there (a permissions value that is not an object is replaced).
// Returns true if the rule was added.
func addClearanceDenyRule(root map[string]interface{}) bool {
	if hasDenyRule(root, ShaperClearanceDenyRule) {
		return false
	}
	perms, ok := root["permissions"].(map[string]interface{})
	if !ok {
		perms = map[string]interface{}{}
		root["permissions"] = perms
	}
	deny, _ := perms["deny"].([]interface{})
	perms["deny"] = append(deny, ShaperClearanceDenyRule)
	return true
}

// removeHooks removes our hook entries. Returns true if any change was made.
// Identity is Labdrian-owned entry shape: an entry of any family, installed or
// retired (ownedByAFamily), not merely any entry that happens to reference the
// same binary path. SessionEnd, SessionStart and PostToolUse are scanned alongside
// UserPromptSubmit/PreToolUse so an owned sync-trigger entry, or the projection
// family's SessionStart (C23 M6) and PostToolUse (C23 M7) entries, is removed the
// same way. Stop is scanned for the entry of the review stop-hook alone (removable):
// nothing else was ever installed there, and an entry of another family that an
// older version left on Stop stays, as it always has.
func removeHooks(root map[string]interface{}, hookCommand string) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}

	changed := false
	for _, key := range []string{"UserPromptSubmit", "PreToolUse", "SessionEnd", "SessionStart", "PostToolUse", "Stop"} {
		entries, ok := hooks[key].([]interface{})
		if !ok {
			continue
		}
		var filtered []interface{}
		for _, e := range entries {
			if removable(key, e, hookCommand) {
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

// removable reports whether removeHooks takes the entry e out of the list of event: an entry of any
// family, installed or retired, under the events the overlay has always scanned, and under Stop the
// entry of the review stop-hook only.
func removable(event string, e interface{}, hookCommand string) bool {
	if event == "Stop" {
		return reviewStopFamily.owns(e, hookCommand)
	}
	return ownedByAFamily(e, hookCommand)
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
