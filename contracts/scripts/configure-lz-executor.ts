import { CONFIG_TYPE_EXECUTOR, encodeExecutorConfig } from "./lz-config.js";
import {
  jsonStringify,
  loadABIArtifact,
  waitForTx,
  type ChainClients,
} from "./lib.js";
import type { Address } from "viem";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json"
);

export type ConfigureLzExecutorInput = {
  endpoint: Address;
  oapp: Address;
  remoteEid: number;
  sendUln: Address;
  openExecutor: Address;
  executorMaxMessageSize: bigint;
};

export function buildConfigureLzExecutorPlan(input: ConfigureLzExecutorInput) {
  if (input.executorMaxMessageSize > 0xffffffffn) {
    throw new Error("input.executorMaxMessageSize exceeds uint32");
  }
  return {
    endpoint: input.endpoint,
    oapp: input.oapp,
    remoteEid: input.remoteEid,
    sendUln: input.sendUln,
    executorConfig: {
      maxMessageSize: Number(input.executorMaxMessageSize),
      executor: input.openExecutor,
    },
    encodedConfig: encodeExecutorConfig({
      maxMessageSize: Number(input.executorMaxMessageSize),
      executor: input.openExecutor,
    }),
  };
}

export async function configureLzExecutor(
  input: ConfigureLzExecutorInput,
  clients: ChainClients
): Promise<void> {
  const plan = buildConfigureLzExecutorPlan(input);

  await waitForTx(
    clients.publicClient,
    "Endpoint.setConfig ExecutorConfig",
    await clients.walletClient.writeContract({
      address: input.endpoint,
      abi: endpointArtifact.abi,
      functionName: "setConfig",
      args: [
        input.oapp,
        input.sendUln,
        [
          {
            eid: input.remoteEid,
            configType: CONFIG_TYPE_EXECUTOR,
            config: plan.encodedConfig,
          },
        ],
      ],
      account: clients.account,
      chain: clients.walletClient.chain,
    })
  );

  console.log(
    jsonStringify({
      endpoint: plan.endpoint,
      oapp: plan.oapp,
      remoteEid: plan.remoteEid,
      sendUln: plan.sendUln,
      executorConfig: plan.executorConfig,
    })
  );
}
