import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  runPriceConfigCheck,
  type RunPriceConfigCheckInput,
} from "../price-config-check.js";
import {
  loadScriptRunFile,
  withReadOnlyConnection,
} from "../command-harness.js";
import {
  addressField,
  bigintField,
  optionalBigintField,
  parseInputObject,
  uint32Field,
} from "../commands/input-parsers.js";
import { buildArtifacts, requiredNetwork } from "../commands/runtime.js";

export function parsePriceConfigCommandInput(
  value: unknown,
  label: string
): RunPriceConfigCheckInput {
  const input = parseInputObject(value, label, [
    "dstEid",
    "maxPriceAgeSeconds",
    "expectedStaleAfter",
    "priceFeed",
    "openExecutor",
    "openDVN",
  ]);
  return {
    dstEid: uint32Field(input, "dstEid", label),
    maxPriceAgeSeconds: bigintField(input, "maxPriceAgeSeconds", label),
    expectedStaleAfter: optionalBigintField(
      input,
      "expectedStaleAfter",
      label
    ),
    priceFeed: addressField(input, "priceFeed", label),
    openExecutor: addressField(input, "openExecutor", label),
    openDVN: addressField(input, "openDVN", label),
  };
}

export async function runPriceConfigCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parsePriceConfigCommandInput);
  const network = requiredNetwork(hre);
  await buildArtifacts(hre);
  await withReadOnlyConnection(hre, { network }, ({ publicClient }) =>
    runPriceConfigCheck(runFile.input, publicClient)
  );
}
