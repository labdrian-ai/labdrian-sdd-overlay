# Models

Three tiers, the same on every provider. You, the parent that launches the roles, are frontier at medium effort. You are not a generated role file.

- Frontier, medium: comprehension, reasoning, and high-complexity work. shaper, reviewer, verifier. Claude alias `opus`. Pi `pi-claude-cli/claude-opus-5`.
- Mid, medium: development, polishing, exploration, sweeps. builder, polisher, prototyper, sweeper, classifier. Claude alias `sonnet`. Pi `pi-claude-cli/claude-sonnet-5`.
- Light, low: delivery and repetitive low-reasoning work. delivery. Claude alias `haiku`. Pi `pi-claude-cli/claude-haiku-4-5`.

On Pi the agent file names the model and the effort (`thinking`). On Claude the agent file names the model. A per-launch `model` still overrides it: the second blind review, a refusal retry, and judgment-day's second judge.

Codex files have no `model` (`opus`, `sonnet` and `haiku` mean nothing there) and carry the effort as `model_reasoning_effort`. Pass the model at spawn. Frontier roles, and you, take the provider's frontier model (Sol; not Astra or Fable). Mid roles take the provider's mid model. delivery takes its light model. The `opencode` column of the tier table below is the only place this repo maps an OpenAI id (OpenCode ids, measured on 2026-11-10); it maps none for Codex, so do not invent one. Pass medium effort for yourself only when that spawn already accepts an effort flag. `labdrian review launch --runtime codex --model M` takes `--model` and no effort flag: M for the reviewer is the frontier model, and the toml sets the effort. A writing role goes through `spawn_agent` with the same tier.

## Tier table

`engine/agentgen` reads the model ids of the generated files from this table, so each id is written here once. Keep one row per tier (`frontier`, `mid`, `light`) and one column per runtime (`claude`, `pi`, `opencode`), each id in backticks. The OpenCode id is `openai/<model>#<effort>`, where the effort is the tier's effort above.

| tier | claude | pi | opencode |
| --- | --- | --- | --- |
| frontier | `opus` | `pi-claude-cli/claude-opus-5` | `openai/gpt-6.1-sol#medium` |
| mid | `sonnet` | `pi-claude-cli/claude-sonnet-5` | `openai/gpt-6.1-terra#medium` |
| light | `haiku` | `pi-claude-cli/claude-haiku-4-5` | `openai/gpt-6-luna#low` |

Assumption, not yet observed: that `terra` is the provider's mid model and `luna` its light one. The ids were measured (L10); their tiers are inferred from the Sol, Terra, Luna naming. A live run of each tier checks it.
