import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { Plugin } from "@opencode/plugin";
// Tool.Error is how a hook denies a call; the hook bridge (S10) uses it.
import { Tool } from "@opencode/plugin/promise/tool";

// The tool whose launches the contract injection rewrites, and the input field that names the agent.
const INJECTED_TOOL = "task";
const AGENT_FIELD = "subagent_type";

function isRecord(value) {
  return typeof value === "object" && value !== null;
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

function mutatePrompt(prompt, subagentType, config, workContext) {
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

// executeBefore is the execute.before handler: it rewrites the prompt of a launch of the injected
// tool in place, which is what reaches the child session.
function executeBefore(call, config) {
  if (!isRecord(call) || call.tool !== INJECTED_TOOL) return;
  const input = call.input;
  if (!isRecord(input)) return;
  if (typeof input.prompt !== "string") return;
  if (typeof input[AGENT_FIELD] !== "string") return;

  input.prompt = mutatePrompt(input.prompt, input[AGENT_FIELD], config, extractInvocationWorkContext(input));
}

function extractInvocationWorkContext(input) {
  return isRecord(input.work_context) ? input.work_context : undefined;
}

// recordActive writes the active marker: which version, hash and prompt config this load runs.
async function recordActive(pluginPath, configRoot, config) {
  const pluginHash = createHash("sha256")
    .update(await readFile(pluginPath, "utf8"))
    .digest("hex");
  await writeFile(
    join(configRoot, "labdrian-runtime-parity.active.json"),
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
}

// registerContractInjection hooks the contract injection into tool launches.
async function registerContractInjection(ctx, config) {
  await ctx.tool.hook("execute.before", (call) => executeBefore(call, config));
}

export default Plugin.define({
  // The id is also the name of the directory the adapter installs the plugin into.
  id: "labdrian-runtime-parity",
  async setup(ctx) {
    // <config>/plugins/labdrian-runtime-parity/index.js: the config root is three levels up.
    const pluginPath = fileURLToPath(import.meta.url);
    const configRoot = dirname(dirname(dirname(pluginPath)));
    const config = JSON.parse(await readFile(join(configRoot, "labdrian-runtime-parity.json"), "utf8"));

    await recordActive(pluginPath, configRoot, config);
    await registerContractInjection(ctx, config);
  },
});
