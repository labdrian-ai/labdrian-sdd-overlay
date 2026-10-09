package settings

// The helpers below read a parsed settings.json and say what of the overlay's hooks it holds. They
// take the object as it was decoded (map[string]interface{}) so a caller that has already read the
// file, as the status commands do, needs no second parse. Every one answers by identity, the
// installed binary path together with the family's token, never by position.

// HasLabdrianOwnedHook reports whether a hook key contains at least one entry
// for the provided identity token.
func HasLabdrianOwnedHook(root map[string]interface{}, key, hookCommand, identity string) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}
	return hasEntryMatching(hooks, key, func(e interface{}) bool {
		return entryContainsBinary(e, hookCommand) && entryContainsBinary(e, identity)
	})
}

// HasLabdrianMinimalismHook reports whether key has our minimalism-contract hook.
func HasLabdrianMinimalismHook(root map[string]interface{}, key, hookCommand string) bool {
	return HasLabdrianOwnedHook(root, key, hookCommand, LabdrianMinimalismIdentity)
}

// HasLabdrianDesignHook reports whether key has our anti-generic-design
// embedded-contract hook.
func HasLabdrianDesignHook(root map[string]interface{}, key, hookCommand string) bool {
	return HasLabdrianOwnedHook(root, key, hookCommand, LabdrianDesignIdentity)
}

// HasLabdrianSyncTriggerHook reports whether key has our sync-trigger hook.
func HasLabdrianSyncTriggerHook(root map[string]interface{}, key, hookCommand string) bool {
	return HasLabdrianOwnedHook(root, key, hookCommand, LabdrianSyncTriggerIdentity)
}

// HasLabdrianReviewReceiptHook reports whether key has our review-receipt hook.
func HasLabdrianReviewReceiptHook(root map[string]interface{}, key, hookCommand string) bool {
	return HasLabdrianOwnedHook(root, key, hookCommand, LabdrianReviewReceiptIdentity)
}

// HasSupportedClaudeLifecycleState reports whether settings contain all known
// Labdrian-owned Claude hook families: the minimalism pair, the
// anti-generic-design pair, the SessionEnd sync-trigger entry, the
// PreToolUse/Bash review-receipt entry, the shaper clearance guard, the workflow
// projection family, and the skills approve guard.
func HasSupportedClaudeLifecycleState(root map[string]interface{}, hookCommand string) bool {
	return HasLabdrianMinimalismHook(root, "UserPromptSubmit", hookCommand) &&
		HasLabdrianMinimalismHook(root, "PreToolUse", hookCommand) &&
		HasLabdrianDesignHook(root, "UserPromptSubmit", hookCommand) &&
		HasLabdrianDesignHook(root, "PreToolUse", hookCommand) &&
		HasLabdrianSyncTriggerHook(root, "SessionEnd", hookCommand) &&
		HasLabdrianReviewReceiptHook(root, "PreToolUse", hookCommand) &&
		HasShaperClearanceGuard(root, hookCommand) &&
		HasProjectionHooks(root, hookCommand) &&
		HasApproveGuard(root, hookCommand)
}

// HasShaperClearanceGuard reports whether the shaper clearance deny guard is
// fully in place: the PreToolUse guard entries for Bash and for the file
// tools, the permissions.deny backstop, and hooks not globally disabled.
// Even fully in place the guard is a speed bump, not a security boundary.
func HasShaperClearanceGuard(root map[string]interface{}, hookCommand string) bool {
	return len(MissingShaperClearanceGuardParts(root, hookCommand)) == 0
}

// MissingShaperClearanceGuardParts names every missing part of the shaper
// clearance deny guard in root, in a fixed order. It returns nil when the
// guard is fully in place.
func MissingShaperClearanceGuardParts(root map[string]interface{}, hookCommand string) []string {
	var missing []string
	hooks, _ := root["hooks"].(map[string]interface{})
	for _, spec := range shaperGuardFamily.specs {
		entries, _ := hooks[spec.event].([]interface{})
		if !shaperGuardFamily.holds(entries, hookCommand, spec) {
			missing = append(missing, `PreToolUse matcher="`+spec.matcher+`" guard hook`)
		}
	}
	if !hasDenyRule(root, ShaperClearanceDenyRule) {
		missing = append(missing, "permissions.deny "+ShaperClearanceDenyRule)
	}
	if disabled, _ := root["disableAllHooks"].(bool); disabled {
		missing = append(missing, "hooks enabled (disableAllHooks is true)")
	}
	return missing
}

func hasDenyRule(root map[string]interface{}, rule string) bool {
	perms, _ := root["permissions"].(map[string]interface{})
	deny, _ := perms["deny"].([]interface{})
	for _, r := range deny {
		if r == rule {
			return true
		}
	}
	return false
}
