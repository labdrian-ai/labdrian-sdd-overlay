package status

import "testing"

const (
	labelMainAgent = `settings: "agent" (main session runs as the foreman)`
	mainAgentFix   = "run 'labdrian uninstall-hooks' then 'labdrian install-hooks'"
)

func TestTheMainAgentKeyIsReportedPresentWhenTheForemanIsSet(t *testing.T) {
	wantCheck(t, complete(t).check().named(t, labelMainAgent), OK, "installed; "+restart+" if this session started earlier")
}

func TestTheMainAgentKeyIsReportedMissingWithTheRemediation(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { delete(root, "agent") })
	wantCheck(t, w.check().named(t, labelMainAgent), Warn, `"agent" key missing in `+settingsPath+"; the main session does not run as the foreman; "+mainAgentFix+"; "+restart)
}

func TestAForeignMainAgentValueIsReportedAsAConflict(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { root["agent"] = "someone-else" })
	wantCheck(t, w.check().named(t, labelMainAgent), Warn, `"agent" is "someone-else", not "foreman"; the overlay never overwrites it, so the main session does not run as the foreman; set it yourself or remove the key, then run 'labdrian install-hooks'`)
}
