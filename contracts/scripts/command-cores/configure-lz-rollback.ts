import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { Address } from "viem";
import {
  applyLzRollback,
  printLzRollbackPlan,
} from "../configure-lz-rollback.js";
import {
  assertExpectedSigner,
  createApplyGate,
  loadScriptRunFile,
  withWriteConnection,
} from "../command-harness.js";
import {
  addressField,
  parseInputObject,
  stringField,
} from "../commands/input-parsers.js";
import {
  chainClients,
  requireApplyFlag,
  requiredNetwork,
} from "../commands/runtime.js";

export type ConfigureLzRollbackCommandInput = {
  lzConfigSnapshot: string;
  expectedSigner: Address;
};

export function parseConfigureLzRollbackCommandInput(
  value: unknown,
  label: string
): ConfigureLzRollbackCommandInput {
  const input = parseInputObject(value, label, [
    "lzConfigSnapshot",
    "expectedSigner",
  ]);
  return {
    lzConfigSnapshot: stringField(input, "lzConfigSnapshot", label),
    expectedSigner: addressField(input, "expectedSigner", label),
  };
}

export async function runConfigureLzRollbackCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseConfigureLzRollbackCommandInput);
  const apply = requireApplyFlag(runFile);
  const network = requiredNetwork(hre);
  if (!apply) {
    printLzRollbackPlan(runFile.input.lzConfigSnapshot);
    return;
  }
  const gate = createApplyGate(runFile);
  await withWriteConnection(hre, { network }, async (context) => {
    assertExpectedSigner(context.signerAddress, runFile.input.expectedSigner);
    await gate.authorize({
      command: "configure:lz-rollback",
      targets: [{ network: context.networkName, chainId: context.chainId }],
      actions: ["restore LayerZero configuration snapshot"],
    });
    await applyLzRollback(
      runFile.input.lzConfigSnapshot,
      chainClients(context)
    );
  });
}
