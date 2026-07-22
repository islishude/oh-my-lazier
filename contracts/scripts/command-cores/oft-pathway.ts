import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { Address } from "viem";
import {
  applyOFTPathway,
  inspectOFTPathway,
  type OFTPathwayAction,
  type RateLimitConfig,
  type RunOFTPathwayInput,
} from "../oft-pathway-control.js";
import {
  assertExpectedSigner,
  createApplyGate,
  loadScriptRunFile,
  optionalField,
  withReadOnlyConnection,
  withWriteConnection,
} from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import {
  addressField,
  bigintField,
  enumField,
  optionalAddressField,
  parseInputObject,
  uint32Field,
} from "../commands/input-parsers.js";
import {
  buildArtifacts,
  chainClients,
  requireApplyFlag,
  requiredNetwork,
} from "../commands/runtime.js";

const actions = [
  "inspect",
  "pause-send",
  "unpause-send",
  "pause-receive",
  "unpause-receive",
  "drain",
  "clear-rate-limit",
  "set-rate-limit",
] as const satisfies readonly OFTPathwayAction[];

export type OFTPathwayCommandInput = RunOFTPathwayInput & {
  expectedSigner?: Address;
};

export function parseOFTPathwayCommandInput(
  value: unknown,
  label: string
): OFTPathwayCommandInput {
  const input = parseInputObject(value, label, [
    "action",
    "testOFT",
    "remoteEid",
    "rateLimit",
    "expectedSigner",
  ]);
  const action = enumField(input, "action", actions, label);
  const rateLimit = optionalField(input, "rateLimit", parseRateLimit, label);
  if ((action === "set-rate-limit") !== (rateLimit !== undefined)) {
    throw new Error(
      `${label}.rateLimit is required only when action is set-rate-limit`
    );
  }
  const expectedSigner = optionalAddressField(input, "expectedSigner", label);
  if (action !== "inspect" && expectedSigner === undefined) {
    throw new Error(`${label}.expectedSigner is required for write actions`);
  }
  return {
    action,
    testOFT: addressField(input, "testOFT", label),
    remoteEid: uint32Field(input, "remoteEid", label),
    rateLimit,
    expectedSigner,
  };
}

export async function runOFTPathwayCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseOFTPathwayCommandInput);
  const network = requiredNetwork(hre);
  await buildArtifacts(hre);
  if (runFile.input.action === "inspect") {
    await withReadOnlyConnection(hre, { network }, ({ publicClient }) =>
      inspectOFTPathway(runFile.input, publicClient)
    );
    return;
  }
  const apply = requireApplyFlag(runFile);
  if (!apply) {
    console.log(
      jsonStringify({ applied: false, network, input: runFile.input })
    );
    return;
  }
  const gate = createApplyGate(runFile);
  await withWriteConnection(hre, { network }, async (context) => {
    if (runFile.input.expectedSigner === undefined) {
      throw new Error("input.expectedSigner is required for write actions");
    }
    assertExpectedSigner(context.signerAddress, runFile.input.expectedSigner);
    await gate.authorize({
      command: "oft:pathway",
      targets: [{ network: context.networkName, chainId: context.chainId }],
      actions: [`apply OFT pathway action ${runFile.input.action}`],
    });
    await applyOFTPathway(runFile.input, chainClients(context));
  });
}

function parseRateLimit(value: unknown, label: string): RateLimitConfig {
  const input = parseInputObject(value, label, ["capacity", "refillPerSecond"]);
  return {
    capacity: bigintField(input, "capacity", label),
    refillPerSecond: bigintField(input, "refillPerSecond", label),
  };
}
