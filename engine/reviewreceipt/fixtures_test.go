package reviewreceipt_test

import "fmt"

// stateJSON builds a minimal gentle-ai 2.7.0+ review-state.json payload.
func stateJSON(lineage, state string) []byte {
	return []byte(fmt.Sprintf(
		`{"schema":"gentle-ai.review-transaction/v2","revision":3,"state":{`+
			`"schema":"gentle-ai.review-state/v2","lineage_id":%q,"generation":1,"state":%q,`+
			`"risk_level":"medium","selected_lenses":["review-risk","review-readability"],`+
			`"initial_snapshot":{"base_tree":"basetree1"},`+
			`"current_snapshot":{"kind":"candidate","base_tree":"basetree1","candidate_tree":"candidatetree1"}}}`,
		lineage, state))
}
