package status

import "testing"

const labelOutputStyle = `settings: "outputStyle" (labdrian output style)`

func TestTheOutputStyleKeyIsReportedPresentWhenItIsLabdrian(t *testing.T) {
	wantCheck(t, complete(t).check().named(t, labelOutputStyle), OK, "installed; "+restart+" if this session started earlier")
}

func TestTheOutputStyleKeyIsReportedMissingWithTheInstallRemedy(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { delete(root, "outputStyle") })
	wantCheck(t, w.check().named(t, labelOutputStyle), Warn, `"outputStyle" key missing in `+settingsPath+"; the labdrian output style is not enabled; run 'labdrian install-hooks'; "+restart)
}

func TestAForeignOutputStyleIsReportedAsAConflict(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { root["outputStyle"] = "Gentleman" })
	wantCheck(t, w.check().named(t, labelOutputStyle), Warn, `"outputStyle" is "Gentleman", not "labdrian"; the overlay never overwrites it, so the labdrian output style is not enabled; set it to "labdrian" or remove the key, then run 'labdrian install-hooks'`)
}

func TestANonStringOutputStyleIsPrintedAsJSON(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) { root["outputStyle"] = map[string]interface{}{"name": "x"} })
	wantCheck(t, w.check().named(t, labelOutputStyle), Warn, `"outputStyle" is {"name":"x"}, not "labdrian"; the overlay never overwrites it, so the labdrian output style is not enabled; set it to "labdrian" or remove the key, then run 'labdrian install-hooks'`)
}
