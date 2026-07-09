import { CONFIG_TYPE_EXECUTOR, encodeExecutorConfig } from "./lz-config.js";
import {
  assertConfiguredChain,
  createClients,
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  loadABIArtifact,
  waitForTx,
} from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);

const { account, publicClient, walletClient } = createClients();
await assertConfiguredChain(publicClient);
const endpoint = envAddress("ENDPOINT");
const oapp = envAddress("OAPP");
const remoteEid = envUint32("REMOTE_EID");
const sendUln = envAddress("SEND_ULN");
const openExecutor = envAddress("OPEN_EXECUTOR");
const maxMessageSizeValue = envBigInt("EXECUTOR_MAX_MESSAGE_SIZE");
if (maxMessageSizeValue > 0xffffffffn) {
  throw new Error("EXECUTOR_MAX_MESSAGE_SIZE exceeds uint32");
}

const config = encodeExecutorConfig({
  maxMessageSize: Number(maxMessageSizeValue),
  executor: openExecutor,
});

await waitForTx(
  publicClient,
  "Endpoint.setConfig ExecutorConfig",
  await walletClient.writeContract({
    address: endpoint,
    abi: endpointArtifact.abi,
    functionName: "setConfig",
    args: [
      oapp,
      sendUln,
      [{ eid: remoteEid, configType: CONFIG_TYPE_EXECUTOR, config }],
    ],
    account,
    chain: walletClient.chain,
  }),
);

console.log(
  jsonStringify({
    endpoint,
    oapp,
    remoteEid,
    sendUln,
    executorConfig: {
      maxMessageSize: Number(maxMessageSizeValue),
      executor: openExecutor,
    },
  }),
);
