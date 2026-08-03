import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  resolveRegtestDeployInput,
  runRegtestDeploy,
  type RegtestDeployBusinessInput,
} from "../regtest-deploy.js";
import { createApplyGate, loadScriptRunFile } from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import { parseInputObject, stringField } from "../commands/input-parsers.js";
import { requireApplyFlag } from "../commands/runtime.js";

export function parseRegtestDeployCommandInput(
  value: unknown,
  label: string
): RegtestDeployBusinessInput {
  const input = parseInputObject(value, label, ["tmpDir"]);
  return { tmpDir: stringField(input, "tmpDir", label) };
}

export async function runRegtestDeployCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseRegtestDeployCommandInput);
  requireApplyFlag(runFile);
  await hre.tasks.getTask(["build"]).run({
    force: false,
    files: [],
    quiet: true,
    defaultBuildProfile: "production",
    noTests: true,
    noContracts: false,
  });
  const result = await runRegtestDeploy(
    resolveRegtestDeployInput(runFile.input),
    {
      hre,
      gate: createApplyGate(runFile),
      displayUi: true,
    }
  );
  console.log(jsonStringify(result));
}
