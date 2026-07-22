import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  createApplyGate,
  loadScriptRunFile,
} from "../command-harness.js";
import { runDeployProfile } from "../deploy-profile.js";
import { jsonStringify } from "../lib.js";
import { parseDeployProfileInput } from "../commands/deploy-profile-input.js";
import { requireApplyFlag } from "../commands/runtime.js";

export async function runDeployProfileCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseDeployProfileInput);
  requireApplyFlag(runFile);
  const result = await runDeployProfile(
    runFile.input,
    hre,
    createApplyGate(runFile)
  );
  console.log(jsonStringify(result));
}
