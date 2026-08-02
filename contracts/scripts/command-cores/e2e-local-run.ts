import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { LocalE2ERunBusinessInput } from "../e2e-local-run.js";
import {
  createApplyGate,
  loadScriptRunFile,
} from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import {
  optionalStringField,
  parseInputObject,
  stringField,
} from "../commands/input-parsers.js";
import { buildArtifacts, requireApplyFlag } from "../commands/runtime.js";

export function parseLocalE2ERunCommandInput(
  value: unknown,
  label: string
): LocalE2ERunBusinessInput {
  const input = parseInputObject(value, label, ["tmpDir", "workerReadyUrl"]);
  return {
    tmpDir: stringField(input, "tmpDir", label),
    workerReadyUrl: optionalStringField(input, "workerReadyUrl", label),
  };
}

export async function runLocalE2ERunCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseLocalE2ERunCommandInput);
  requireApplyFlag(runFile);
  await buildArtifacts(hre);
  const { resolveLocalE2ERunInput, runLocalE2E } = await import(
    "../e2e-local-run.js"
  );
  const result = await runLocalE2E(resolveLocalE2ERunInput(runFile.input), {
    hre,
    gate: createApplyGate(runFile),
  });
  console.log(jsonStringify(result));
}
