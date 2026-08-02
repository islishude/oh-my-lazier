import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { CheckOFTCanaryInput } from "../check-oft-canary.js";
import {
  loadScriptRunFile,
  withReadOnlyConnection,
} from "../command-harness.js";
import {
  addressField,
  optionalAddressField,
  optionalBigintField,
  optionalHexField,
  parseInputObject,
} from "../commands/input-parsers.js";
import { buildArtifacts, requiredNetwork } from "../commands/runtime.js";

export function parseCheckOFTCanaryCommandInput(
  value: unknown,
  label: string
): CheckOFTCanaryInput {
  const input = parseInputObject(value, label, [
    "endpoint",
    "sourceTxHash",
    "destinationTxHash",
    "sendLib",
    "openExecutor",
    "destinationEndpoint",
    "destinationTestOFT",
    "recipient",
    "minRecipientBalance",
  ]);
  return {
    endpoint: addressField(input, "endpoint", label),
    sourceTxHash: optionalHexField(input, "sourceTxHash", label),
    destinationTxHash: optionalHexField(input, "destinationTxHash", label),
    sendLib: optionalAddressField(input, "sendLib", label),
    openExecutor: optionalAddressField(input, "openExecutor", label),
    destinationEndpoint: optionalAddressField(
      input,
      "destinationEndpoint",
      label
    ),
    destinationTestOFT: optionalAddressField(
      input,
      "destinationTestOFT",
      label
    ),
    recipient: optionalAddressField(input, "recipient", label),
    minRecipientBalance: optionalBigintField(
      input,
      "minRecipientBalance",
      label
    ),
  };
}

export async function runCheckOFTCanaryCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseCheckOFTCanaryCommandInput);
  const network = requiredNetwork(hre);
  await buildArtifacts(hre);
  const { checkOFTCanary } = await import("../check-oft-canary.js");
  await withReadOnlyConnection(hre, { network }, ({ publicClient }) =>
    checkOFTCanary(runFile.input, publicClient)
  );
}
