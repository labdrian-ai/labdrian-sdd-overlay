import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

function isRecord(value) {
  return typeof value === "object" && value !== null;
}

function isToolExecuteInput(value) {
  return isRecord(value);
}

function isTaskArgs(value) {
  return isRecord(value);
}

function isToolExecuteOutput(value) {
  return isRecord(value) && (!("args" in value) || isTaskArgs(value.args));
}

function hasExactLine(prompt, line) {
  return prompt.split("\n").some((candidate) => candidate.trim() === line);
}

function normalizePromptConfig(config) {
  const promptConfig = isRecord(config) ? config.prompt_config : undefined;
  if (!isRecord(promptConfig)) return undefined;
  if (typeof promptConfig.contract_path !== "string") return undefined;
  if (typeof promptConfig.injection_point !== "string") return undefined;
  if (!Array.isArray(promptConfig.included_phases)) return undefined;
  if (!Array.isArray(promptConfig.excluded_phases)) return undefined;
  if (!promptConfig.included_phases.every((item) => typeof item === "string")) return undefined;
  if (!promptConfig.excluded_phases.every((item) => typeof item === "string")) return undefined;
  return {
    contractPath: promptConfig.contract_path,
    injectionHeader: promptConfig.injection_point,
    includedPhases: new Set(promptConfig.included_phases),
    excludedPhases: new Set(promptConfig.excluded_phases),
  };
}

function injectPrompt(prompt, config) {
  if (hasExactLine(prompt, config.contractPath)) return prompt;
  if (hasExactLine(prompt, config.injectionHeader)) {
    const lines = prompt.split("\n");
    const out = [];
    for (const line of lines) {
      out.push(line);
      if (line.trim() === config.injectionHeader) out.push(config.contractPath);
    }
    return out.join("\n");
  }
  const separator = prompt.endsWith("\n") ? "\n" : "\n\n";
  return prompt + separator + config.injectionHeader + "\n" + config.contractPath + "\n";
}

function stripPrompt(prompt, config) {
  if (!hasExactLine(prompt, config.contractPath)) return prompt;
  return prompt
    .split("\n")
    .filter((line) => line.trim() !== config.contractPath)
    .join("\n");
}

export function mutatePrompt(prompt, subagentType, config) {
  const promptConfig = normalizePromptConfig(config);
  if (!promptConfig) return prompt;
  if (promptConfig.includedPhases.has(subagentType)) {
    return injectPrompt(prompt, promptConfig);
  }
  if (promptConfig.excludedPhases.has(subagentType)) {
    return stripPrompt(prompt, promptConfig);
  }
  return prompt;
}

async function toolExecuteBefore(input, output, config) {
  if (!isToolExecuteInput(input) || input.tool !== "task") return;
  if (!isToolExecuteOutput(output) || !isTaskArgs(output.args)) return;
  if (typeof output.args.prompt !== "string") return;
  if (typeof output.args.subagent_type !== "string") return;

  output.args.prompt = mutatePrompt(output.args.prompt, output.args.subagent_type, config);
}

export default async function labdrianRuntimeParityPlugin() {
  const pluginPath = fileURLToPath(import.meta.url);
  const configRoot = dirname(dirname(pluginPath));
  const configPath = join(configRoot, "labdrian-runtime-parity.json");
  const activeMarkerPath = join(configRoot, "labdrian-runtime-parity.active.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  const pluginHash = createHash("sha256")
    .update(await readFile(pluginPath, "utf8"))
    .digest("hex");

  await writeFile(
    activeMarkerPath,
    JSON.stringify(
      {
        active_version: config.installed_version,
        active_hash: pluginHash,
        active_prompt_config_hash: config.prompt_config_hash,
        plugin_path: pluginPath,
        config_root: configRoot,
      },
      null,
      2,
    ),
  );

  return {
    "tool.execute.before": (input, output) => toolExecuteBefore(input, output, config),
  };
}
