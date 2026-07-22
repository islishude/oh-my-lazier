import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  runDeploymentPreflight,
  type RunDeploymentPreflightInput,
} from "../deployment-preflight.js";
import {
  loadScriptRunFile,
  withReadOnlyConnection,
} from "../command-harness.js";
import {
  addressField,
  optionalAddressField,
  optionalBigintField,
  parseInputObject,
} from "../commands/input-parsers.js";
import { buildArtifacts, requiredNetwork } from "../commands/runtime.js";

export function parseDeploymentPreflightCommandInput(
  value: unknown,
  label: string
): RunDeploymentPreflightInput {
  const input = parseInputObject(value, label, [
    "testOFT",
    "openExecutor",
    "openDVN",
    "expectedOwner",
    "minOwnerNativeBalance",
    "canaryTreasury",
    "minCanaryNativeBalance",
    "minCanaryTokenBalance",
    "expectedTestOFTTotalSupply",
  ]);
  return {
    testOFT: addressField(input, "testOFT", label),
    openExecutor: addressField(input, "openExecutor", label),
    openDVN: addressField(input, "openDVN", label),
    expectedOwner: addressField(input, "expectedOwner", label),
    minOwnerNativeBalance:
      optionalBigintField(input, "minOwnerNativeBalance", label) ?? 0n,
    canaryTreasury: optionalAddressField(input, "canaryTreasury", label),
    minCanaryNativeBalance:
      optionalBigintField(input, "minCanaryNativeBalance", label) ?? 0n,
    minCanaryTokenBalance:
      optionalBigintField(input, "minCanaryTokenBalance", label) ?? 0n,
    expectedTestOFTTotalSupply: optionalBigintField(
      input,
      "expectedTestOFTTotalSupply",
      label
    ),
  };
}

export async function runDeploymentPreflightCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseDeploymentPreflightCommandInput);
  const network = requiredNetwork(hre);
  await buildArtifacts(hre);
  await withReadOnlyConnection(hre, { network }, ({ publicClient }) =>
    runDeploymentPreflight(runFile.input, publicClient)
  );
}
