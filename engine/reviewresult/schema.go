package reviewresult

// Schema is the JSON Schema of the reply of the reviewer (phase9-c26-review-contract S2 a), as compact JSON: the bytes
// json.dumps(SCHEMA, separators=(',', ':')) prints for the SCHEMA of the capsule-lab A/B (re/ab/build.py). Every member
// of every object is required, a finding's id is F<n>, its location is path:line or path:start-end in head-side line
// numbers, and the three enumerations are the constants of this package; schema_test.go holds it to them.
//
// It is a string constant and not an embedded file: the domain ring imports the pure standard library only, and embed
// is not in it (archguard).
func Schema() string { return schema }

const schema = `{"type":"object","additionalProperties":false,"required":["candidate","inspection","findings","evidence"],"properties":{"candidate":{"type":"object","additionalProperties":false,"required":["base","head","diff_sha256"],"properties":{"base":{"type":"string"},"head":{"type":"string"},"diff_sha256":{"type":"string"}}},"inspection":{"type":"object","additionalProperties":false,"required":["status","paths","reason"],"properties":{"status":{"enum":["completed","incomplete"]},"paths":{"type":"array","items":{"type":"string"},"description":"every changed path, once"},"reason":{"type":"string","description":"why incomplete, else empty"}}},"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","severity","location","claim","evidence_class","causal_disposition","proof_refs"],"properties":{"id":{"type":"string","pattern":"^F[1-9][0-9]*$"},"severity":{"enum":["BLOCKER","CRITICAL","WARNING","SUGGESTION"]},"location":{"type":"string","pattern":"^.+:[1-9][0-9]*(-[1-9][0-9]*)?$"},"claim":{"type":"string","minLength":1},"evidence_class":{"enum":["deterministic","inferential"]},"causal_disposition":{"enum":["introduced","behavior-activated","worsened","pre-existing","base-only","unknown"]},"proof_refs":{"type":"array","items":{"type":"string"}}}}},"evidence":{"type":"array","minItems":1,"items":{"type":"string"}}}}`
