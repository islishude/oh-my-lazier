import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { Address } from "viem";
import {
  buildConfigureLzExecutorPlan,
  configureLzExecutor,
  type ConfigureLzExecutorInput,
} from "../configure-lz-executor.js";
import {
  assertExpectedSigner,
  createApplyGate,
  loadScriptRunFile,
  withWriteConnection,
} from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import {
  addressField,
  bigintField,
  parseInputObject,
  uint32Field,
} from "../commands/input-parsers.js";
import {
  chainClients,
  requireApplyFlag,
  requiredNetwork,
} from "../commands/runtime.js";

export type ConfigureLzExecutorCommandInput = ConfigureLzExecutorInput & {
  expectedSigner: Address;
};

export function parseConfigureLzExecutorCommandInput(
  value: unknown,
  label: string
): ConfigureLzExecutorCommandInput {
  const input = parseInputObject(value, label, [
    "endpoint",
    "oapp",
    "remoteEid",
    "sendUln",
    "openExecutor",
    "executorMaxMessageSize",
    "expectedSigner",
  ]);
  return {
    endpoint: addressField(input, "endpoint", label),
    oapp: addressField(input, "oapp", label),
    remoteEid: uint32Field(input, "remoteEid", label),
    sendUln: addressField(input, "sendUln", label),
    openExecutor: addressField(input, "openExecutor", label),
    executorMaxMessageSize: bigintField(
      input,
      "executorMaxMessageSize",
      label
    ),
    expectedSigner: addressField(input, "expectedSigner", label),
  };
}

export async function runConfigureLzExecutorCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseConfigureLzExecutorCommandInput);
  const apply = requireApplyFlag(runFile);
  const network = requiredNetwork(hre);
  const plan = buildConfigureLzExecutorPlan(runFile.input);
  if (!apply) {
    console.log(jsonStringify({ applied: false, network, plan }));
    return;
  }
  const gate = createApplyGate(runFile);
  await withWriteConnection(hre, { network }, async (context) => {
    assertExpectedSigner(
      context.signerAddress,
      runFile.input.expectedSigner,
      "input.expectedSigner"
    );
    await gate.authorize({
      command: "configure:lz-executor",
      targets: [{ network: context.networkName, chainId: context.chainId }],
      actions: ["set LayerZero executor configuration"],
    });
    await configureLzExecutor(runFile.input, chainClients(context));
  });
}
