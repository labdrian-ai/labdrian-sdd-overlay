---
name: clarify-request
description: "Trigger: user wants to explore an idea, clarify a request, prepare requirements or understand a problem before a change. Restates it, asks at most 3 questions."
license: Apache-2.0
metadata:
  author: labdrian
  version: "2.0"
---

## Activation Contract

Load when the user wants to explore an idea, clarify a request, prepare requirements or understand a problem before a change. Run inline, in the parent conversation with the user. Talk about the user's actual request, in the user's language.

## Hard Rules

- Use a hidden checklist of 6 items to find the gaps: goal, current gap, constraints, done-when, alternatives, risks. Each may be `n/a`. The items are not ranked and are never shown to the user as a grid or a score; they appear only in the final brief, as a plain list. An item is a gap only when its answer would change what gets built; otherwise mark it `n/a` or record it as an assumption.
- Plain wording: no therapist phrasing, no menu of business archetypes. Never ask how the user feels about it or what deeper need lies behind it. Use the repository's own terms (API, module, plugin).
- Never derive a change-name and never save under a topic starting with `sdd/`.

## Decision Gates

| The user describes | Do |
|---|---|
| Their own request | Ask about it directly; never use Mom-Test framing |
| A third party's problem (a client, a user, a stakeholder) | Use Mom-Test framing: ask about past behavior, not hypotheticals |

## Execution Steps

1. Ground before asking anything: read the request, `mem_search` for the project and topic, the project manifest when one exists, and the files the user named.
2. Restate in 3-6 lines what you understood, each tagged `said` (the user's words) or `inferred` (from the repository or memory, naming the source).
3. Ask only the gaps that block a decision, one question per message, each quoting the user's words or a repository artifact it is about. No blocking gap: ask nothing.
4. Update the restatement with each answer. Never ask again about something already answered or already found in the grounding.
5. Hard stop at 3 questions in total. After that, record any open gap as an assumption and stop asking.

Example:

```text
- said: "the export is too slow"
- inferred: the export is reports/export.go, one query per row (read from the file)
- inferred: no time target is recorded (mem_search found none)
```

Question: you said "the export is too slow": what time would be fast enough?

## Output Contract

Show the brief: the final restatement, the checklist (with `n/a` items), the questions asked with their answers, and the assumptions. Write it in English; quotes stay in the user's language. When the user confirms it, save it:

```text
mem_save(title: "clarify/{YYYYMMDD-HHMM}", topic_key: "project/{project}/clarify/{YYYYMMDD-HHMM}", type: "architecture", project: "{project}", capture_prompt: false, content: <brief>)
```

Then hand off to `requirements-from-transcripts` with the brief as input: it owns the change-name and the EARS requirements.

## References

- `../_shared/pre-sdd-contracts.md`: topic-key authority and language rule.
- `../requirements-from-transcripts/SKILL.md`: downstream consumer of the brief.
