package settings

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/guardmarkers"

const (
	// Minimalism, design, and sync-trigger identity tokens are exposed to
	// status checks so caller code can assert provable Labdrian ownership
	// without duplicating parsing logic.
	LabdrianMinimalismIdentity = "minimalism-contract.md"
	LabdrianDesignIdentity     = "--embedded-contract " + embeddedDesignName
	// LabdrianSyncTriggerIdentity is both the sync-trigger verb name and the
	// dedup/uninstall identity token for the SessionEnd hook entry — the verb
	// argument itself distinguishes it from every other entry sharing our
	// binary path.
	LabdrianSyncTriggerIdentity = "sync-trigger"
	// LabdrianReviewReceiptIdentity is both the review-receipt verb name and
	// the dedup/uninstall identity token for the PreToolUse/Bash review-receipt
	// hook entry, mirroring LabdrianSyncTriggerIdentity's role for its family.
	LabdrianReviewReceiptIdentity = "review-receipt"
	// LabdrianShaperGuardIdentity is the shaper clearance guard verb and the
	// dedup/uninstall identity token for its PreToolUse entries.
	LabdrianShaperGuardIdentity = "shaper guard-hook"
	// ShaperGuardFileToolMatcher is the PreToolUse matcher of the guard entry
	// that refuses file tools writing into the clearance store.
	ShaperGuardFileToolMatcher = "Write|Edit|MultiEdit|NotebookEdit"
	// ShaperClearanceDenyRule is the permissions.deny backstop for the
	// clearance record entry point. Claude Code documents deny rules as
	// holding in every permission mode, including bypassPermissions. Like
	// the hook, it matches command text only: it is a speed bump, not a
	// security boundary.
	ShaperClearanceDenyRule = "Bash(*" + guardmarkers.Command + "*)"
)

// embeddedDesignName is the engine-owned managed contract that propagates the
// anti-generic-design guard (countering the model's default "Claude/SaaS
// look" design bias). It rides the same propagate/gate-task machinery as the
// minimalism contract but writes a DISTINCT registry block.
const embeddedDesignName = "anti-generic-design"
