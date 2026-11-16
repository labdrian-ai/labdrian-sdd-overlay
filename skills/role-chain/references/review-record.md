# When `labdrian review record` exits 2

The record is refused when `candidate` or `paths` differ from the bound review, when `inspection.status` is not `completed`, or when any other rule of the reviewer's contract breaks. The parent then records `interrupted` with `resume_reason: review not run: <reason>` and retries once with another `model`.
