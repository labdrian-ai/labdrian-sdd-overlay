# Models

Three tiers, the same on every provider. You, the parent that launches the roles, are frontier at max effort. You are not a generated role file.

- Frontier, high: comprehension, reasoning, and high-complexity work. shaper, reviewer, verifier. Claude alias `opus`. Pi `pi-claude-cli/claude-opus-5`.
- Mid, high: development, polishing, exploration, sweeps. builder, polisher, prototyper, sweeper. Claude alias `sonnet`. Pi `pi-claude-cli/claude-sonnet-5`.
- Light, low: delivery and repetitive low-reasoning work. delivery. Claude alias `haiku`. Pi `pi-claude-cli/claude-haiku-4-5`.

On Claude and Pi the agent file names the model and the effort. A per-launch `model` still overrides it: the second blind review, a refusal retry, and judgment-day's second judge.

Codex files have no `model` (`opus`, `sonnet` and `haiku` mean nothing there) and carry the effort as `model_reasoning_effort`. Pass the model at spawn. Frontier roles, and you, take the provider's frontier model (Sol; not Astra or Fable). Mid roles take the provider's mid model. delivery takes its light model. This repo maps no OpenAI id, so do not invent one. Pass max effort for yourself only when that spawn already accepts an effort flag. `labdrian review launch --runtime codex --model M` takes `--model` and no effort flag: M for the reviewer is the frontier model, and the toml sets the effort. A writing role goes through `spawn_agent` with the same tier.
