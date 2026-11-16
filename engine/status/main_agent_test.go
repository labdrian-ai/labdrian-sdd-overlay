package status

import "testing"

const labelMainAgent = `settings: "agent" (main session runs as the foreman)`

func TestTheMainAgentKeyIsReportedPresentWhenTheForemanIsSet(t *testing.T) {
	wantCheck(t, complete(t).check().named(t, labelMainAgent), OK, "installed; "+restart+" if this session started earlier")
}

func TestTheMainAgentKeyIsReportedMissingWithTheInstallRemedy(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { delete(root, "agent") })
	wantCheck(t, w.check().named(t, labelMainAgent), Warn, `"agent" key missing in `+settingsPath+"; the main session does not run as the foreman; run 'labdrian install-hooks'; "+restart)
}

func TestAForeignMainAgentValueIsReportedAsAConflict(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { root["agent"] = "someone-else" })
	wantCheck(t, w.check().named(t, labelMainAgent), Warn, `"agent" is "someone-else", not "foreman"; the overlay never overwrites it, so the main session does not run as the foreman; set it to "foreman" or remove the key, then run 'labdrian install-hooks'`)
}

func TestANonStringMainAgentValueIsPrintedAsJSON(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { root["agent"] = map[string]interface{}{"name": "x"} })
	wantCheck(t, w.check().named(t, labelMainAgent), Warn, `"agent" is {"name":"x"}, not "foreman"; the overlay never overwrites it, so the main session does not run as the foreman; set it to "foreman" or remove the key, then run 'labdrian install-hooks'`)
}
