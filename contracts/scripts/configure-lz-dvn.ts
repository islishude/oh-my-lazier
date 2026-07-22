import {
  CONFIG_TYPE_ULN,
  encodeUlnConfig,
  requiredDVNsConfig,
} from "./lz-config.js";
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

export type ConfigureLzDVNInput = {
  endpoint: Address;
  oapp: Address;
  remoteEid: number;
  sendUln: Address;
  receiveUln: Address;
  requiredDVNs: Address[];
  confirmations: bigint;
};

export function buildConfigureLzDVNPlan(input: ConfigureLzDVNInput) {
  const ulnConfig = requiredDVNsConfig(input.confirmations, input.requiredDVNs);
  return {
    endpoint: input.endpoint,
    oapp: input.oapp,
    remoteEid: input.remoteEid,
    sendUln: input.sendUln,
    receiveUln: input.receiveUln,
    ulnConfig,
    encodedConfig: encodeUlnConfig(ulnConfig),
  };
}

export async function configureLzDVN(
  input: ConfigureLzDVNInput,
  clients: ChainClients
): Promise<void> {
  const plan = buildConfigureLzDVNPlan(input);

  for (const [label, library] of [
    ["SendUln302", input.sendUln],
    ["ReceiveUln302", input.receiveUln],
  ] as const) {
    await waitForTx(
      clients.publicClient,
      `Endpoint.setConfig ${label} UlnConfig`,
      await clients.walletClient.writeContract({
        address: input.endpoint,
        abi: endpointArtifact.abi,
        functionName: "setConfig",
        args: [
          input.oapp,
          library,
          [
            {
              eid: input.remoteEid,
              configType: CONFIG_TYPE_ULN,
              config: plan.encodedConfig,
            },
          ],
        ],
        account: clients.account,
        chain: clients.walletClient.chain,
      })
    );
  }

  console.log(
    jsonStringify({
      endpoint: plan.endpoint,
      oapp: plan.oapp,
      remoteEid: plan.remoteEid,
      sendUln: plan.sendUln,
      receiveUln: plan.receiveUln,
      ulnConfig: plan.ulnConfig,
    })
  );
}
