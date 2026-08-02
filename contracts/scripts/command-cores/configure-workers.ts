import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  assertExpectedSigner,
  createApplyGate,
  loadScriptRunFile,
  withWriteConnection,
} from "../command-harness.js";
import {
  buildConfigureWorkersPlan,
  configureWorkers,
} from "../configure-workers.js";
import { jsonStringify } from "../lib.js";
import { parseConfigureWorkersCommandInput } from "../commands/configure-workers-input.js";
import {
  buildArtifacts,
  chainClients,
  requireApplyFlag,
  requiredNetwork,
} from "../commands/runtime.js";

export async function runConfigureWorkersCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseConfigureWorkersCommandInput);
  const apply = requireApplyFlag(runFile);
  const network = requiredNetwork(hre);
  const { expectedSigner, ...input } = runFile.input;
  const plan = buildConfigureWorkersPlan(input);
  if (!apply) {
    console.log(jsonStringify({ applied: false, network, plan }));
    return;
  }

  await buildArtifacts(hre);
  const gate = createApplyGate(runFile);
  await withWriteConnection(hre, { network }, async (context) => {
    assertExpectedSigner(
      context.signerAddress,
      expectedSigner,
      "input.expectedSigner"
    );
    await gate.authorize({
      command: "configure:workers",
      targets: [{ network: context.networkName, chainId: context.chainId }],
      actions: [
        "set the destination price snapshot",
        "configure worker price feeds, allowed send library, pathway settings, and fee models",
        ...(input.rateLimit === undefined
          ? []
          : ["configure the TestOFT outbound rate limit"]),
      ],
    });
    const result = await configureWorkers(input, chainClients(context), {
      now: plan.priceSnapshot.updatedAt,
    });
    console.log(jsonStringify(result));
  });
}
