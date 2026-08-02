import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  resolveLocalE2EDeployInput,
  runLocalE2EDeploy,
  type LocalE2EDeployBusinessInput,
} from "../e2e-local-deploy.js";
import {
  createApplyGate,
  loadScriptRunFile,
} from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import { parseInputObject, stringField } from "../commands/input-parsers.js";
import { requireApplyFlag } from "../commands/runtime.js";

export function parseLocalE2EDeployCommandInput(
  value: unknown,
  label: string
): LocalE2EDeployBusinessInput {
  const input = parseInputObject(value, label, ["tmpDir"]);
  return { tmpDir: stringField(input, "tmpDir", label) };
}

export async function runLocalE2EDeployCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseLocalE2EDeployCommandInput);
  requireApplyFlag(runFile);
  await hre.tasks.getTask(["build"]).run({
    force: false,
    files: [],
    quiet: true,
    defaultBuildProfile: "production",
    noTests: true,
    noContracts: false,
  });
  const result = await runLocalE2EDeploy(
    resolveLocalE2EDeployInput(runFile.input),
    {
      hre,
      gate: createApplyGate(runFile),
      displayUi: true,
    }
  );
  console.log(jsonStringify(result));
}
