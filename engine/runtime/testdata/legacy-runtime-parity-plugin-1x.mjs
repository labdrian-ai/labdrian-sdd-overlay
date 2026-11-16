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
  if (Array.isArray(promptConfig.contracts)) {
    const contracts = [];
    for (const contract of promptConfig.contracts) {
      const normalized = normalizeContractConfig(contract);
      if (normalized) contracts.push(normalized);
    }
    if (contracts.length === 0) return undefined;
    return { contracts };
  }
	  const legacy = normalizeContractConfig({
	    contract_path: promptConfig.contract_path,
	    injection_point: promptConfig.injection_point,
	    included_phases: promptConfig.included_phases,
	    excluded_phases: promptConfig.excluded_phases,
	    ...(Object.prototype.hasOwnProperty.call(promptConfig, "language_context")
	      ? { language_context: promptConfig.language_context }
	      : {}),
	    ...(Object.prototype.hasOwnProperty.call(promptConfig, "activation_context")
	      ? { activation_context: promptConfig.activation_context }
	      : {}),
	    ...(Object.prototype.hasOwnProperty.call(promptConfig, "context_operator")
	      ? { context_operator: promptConfig.context_operator }
	      : {}),
	  });
  if (!legacy) return undefined;
  return { contracts: [legacy] };
}

function normalizeContractConfig(promptConfig) {
  if (!isRecord(promptConfig)) return undefined;
  if (typeof promptConfig.contract_path !== "string") return undefined;
  if (typeof promptConfig.injection_point !== "string") return undefined;
  if (!Array.isArray(promptConfig.included_phases)) return undefined;
  if (!Array.isArray(promptConfig.excluded_phases)) return undefined;
  if (!promptConfig.included_phases.every((item) => typeof item === "string")) return undefined;
  if (!promptConfig.excluded_phases.every((item) => typeof item === "string")) return undefined;
  if ("context_operator" in promptConfig) return undefined;
  if ("language_context" in promptConfig && !isStringArray(promptConfig.language_context)) return undefined;
  if ("activation_context" in promptConfig && !isStringArray(promptConfig.activation_context)) return undefined;
  return {
    contractPath: promptConfig.contract_path,
    injectionHeader: promptConfig.injection_point,
    includedPhases: new Set(promptConfig.included_phases),
    excludedPhases: new Set(promptConfig.excluded_phases),
    languageContext: new Set((promptConfig.language_context ?? []).map((item) => item.toLowerCase())),
    activationContext: new Set((promptConfig.activation_context ?? []).map((item) => item.toLowerCase())),
  };
}

function isStringArray(value) {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
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

export function mutatePrompt(prompt, subagentType, config, workContext) {
	  const promptConfig = normalizePromptConfig(config);
	  if (!promptConfig) return prompt;
	  const invocationWorkContext = resolveInvocationWorkContext(workContext);
	  let nextPrompt = prompt;
	  for (const contract of promptConfig.contracts) {
	    if (contract.excludedPhases.has(subagentType)) {
	      nextPrompt = stripPrompt(nextPrompt, contract);
	      continue;
	    }
	    if (!contract.includedPhases.has(subagentType)) continue;
	    if (contextRequired(contract) && !workContextMatches(invocationWorkContext, contract)) continue;
	    nextPrompt = injectPrompt(nextPrompt, contract);
	  }
	  return nextPrompt;
}

function resolveInvocationWorkContext(workContext) {
	  return isRecord(workContext) ? workContext : undefined;
}

function contextRequired(contract) {
  return contract.languageContext.size > 0 || contract.activationContext.size > 0;
}

function workContextMatches(workContext, contract) {
  if (!isRecord(workContext) || workContext.trusted !== true) return false;
  if (contract.languageContext.size > 0 && !setIntersects(contract.languageContext, workContext.languages)) return false;
  if (contract.activationContext.size > 0 && !setIntersects(contract.activationContext, workContext.activations)) return false;
  if (!isStringArray(workContext.work_kinds) || workContext.work_kinds.length === 0) return false;
  if (!hasString(workContext.work_kinds, "application-code")) return false;
  return true;
}

function setIntersects(want, got) {
  if (!isStringArray(got)) return false;
  return got.some((item) => want.has(item.toLowerCase()));
}

function hasString(items, want) {
  return items.some((item) => item.toLowerCase() === want);
}

async function toolExecuteBefore(input, output, config) {
	  if (!isToolExecuteInput(input) || input.tool !== "task") return;
	  if (!isToolExecuteOutput(output) || !isTaskArgs(output.args)) return;
	  if (typeof output.args.prompt !== "string") return;
	  if (typeof output.args.subagent_type !== "string") return;

	  output.args.prompt = mutatePrompt(
	    output.args.prompt,
	    output.args.subagent_type,
	    config,
	    extractInvocationWorkContext(input, output),
	  );
}

function extractInvocationWorkContext(input, output) {
	  const outputContext = isTaskArgs(output?.args) ? output.args.work_context : undefined;
	  if (isRecord(outputContext)) return outputContext;
	  const inputArgs = isRecord(input) ? input.args : undefined;
	  const inputContext = isRecord(inputArgs) ? inputArgs.work_context : undefined;
	  if (isRecord(inputContext)) return inputContext;
	  const directInputContext = isRecord(input) ? input.work_context : undefined;
	  if (isRecord(directInputContext)) return directInputContext;
	  return undefined;
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
