import {
  CONFIG_TYPE_ULN,
  encodeUlnConfig,
  requiredDVNsConfig,
} from "./lz-config.js";
import {
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
const endpoint = envAddress("ENDPOINT");
const oapp = envAddress("OAPP");
const remoteEid = envUint32("REMOTE_EID");
const sendUln = envAddress("SEND_ULN");
const receiveUln = envAddress("RECEIVE_ULN");
const openDVN = envAddress("OPEN_DVN");
const layerZeroLabsDVN = envAddress("LAYERZERO_LABS_DVN");
const confirmations = envBigInt("CONFIRMATIONS");

const ulnConfig = requiredDVNsConfig(confirmations, [
  openDVN,
  layerZeroLabsDVN,
]);
const encodedUlnConfig = encodeUlnConfig(ulnConfig);

await waitForTx(
  publicClient,
  "Endpoint.setConfig SendUln302 UlnConfig",
  await walletClient.writeContract({
    address: endpoint,
    abi: endpointArtifact.abi,
    functionName: "setConfig",
    args: [
      oapp,
      sendUln,
      [
        {
          eid: remoteEid,
          configType: CONFIG_TYPE_ULN,
          config: encodedUlnConfig,
        },
      ],
    ],
    account,
    chain: null,
  }),
);

await waitForTx(
  publicClient,
  "Endpoint.setConfig ReceiveUln302 UlnConfig",
  await walletClient.writeContract({
    address: endpoint,
    abi: endpointArtifact.abi,
    functionName: "setConfig",
    args: [
      oapp,
      receiveUln,
      [
        {
          eid: remoteEid,
          configType: CONFIG_TYPE_ULN,
          config: encodedUlnConfig,
        },
      ],
    ],
    account,
    chain: null,
  }),
);

console.log(
  jsonStringify({
    endpoint,
    oapp,
    remoteEid,
    sendUln,
    receiveUln,
    ulnConfig,
  }),
);
