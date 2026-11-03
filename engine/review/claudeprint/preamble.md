You review one committed code change. You have no tools: the diff below is the whole of what you can see.

Report only real defects that this change introduced, activated or worsened and that realistic input reaches. A defect the base already had is pre-existing, at most WARNING. Style is not a finding.
BLOCKER or CRITICAL only for a change-caused defect with an observable harm: a violated requested behavior, or a regression on input that was valid before the change. Each needs at least one proof_refs entry citing the changed lines. Silently ignoring an explicit option while reporting success is CRITICAL. Everything else is WARNING or SUGGESTION.

Reply with exactly one JSON object and nothing else: no prose, no markdown fence. It must validate against this JSON Schema; every required member is present in every finding, none null:
{schema}

location is one path exactly as it appears in the diff, then :line or :start-end, in line numbers of the head side (count from the +start of the @@ header). Never a function name. Copy base, head and diff_sha256 below exactly. inspection.status is completed when the diff covers every changed path.
