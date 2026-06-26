import { type Address, type Hex } from "viem";
import {
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  decodeExecutorConfig,
  decodeUlnConfig,
} from "./lz-config.js";
import {
  createPublicClientFromEnv,
  envAddress,
  envUint32,
  jsonStringify,
  loadABIArtifact,
} from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);

const publicClient = createPublicClientFromEnv();
const endpoint = envAddress("ENDPOINT");
const oapp = envAddress("OAPP");
const remoteEid = envUint32("REMOTE_EID");
const sendUln = envAddress("SEND_ULN");
const receiveUln = envAddress("RECEIVE_ULN");

const localEid = await publicClient.readContract({
  address: endpoint,
  abi: endpointArtifact.abi,
  functionName: "eid",
});
const activeSendLibrary = await publicClient.readContract({
  address: endpoint,
  abi: endpointArtifact.abi,
  functionName: "getSendLibrary",
  args: [oapp, remoteEid],
});
const [activeReceiveLibrary, receiveLibraryIsDefault] =
  (await publicClient.readContract({
    address: endpoint,
    abi: endpointArtifact.abi,
    functionName: "getReceiveLibrary",
    args: [oapp, remoteEid],
  })) as [Address, boolean];

const executorConfigBytes = (await publicClient.readContract({
  address: endpoint,
  abi: endpointArtifact.abi,
  functionName: "getConfig",
  args: [oapp, sendUln, remoteEid, CONFIG_TYPE_EXECUTOR],
})) as Hex;
const sendUlnConfigBytes = (await publicClient.readContract({
  address: endpoint,
  abi: endpointArtifact.abi,
  functionName: "getConfig",
  args: [oapp, sendUln, remoteEid, CONFIG_TYPE_ULN],
})) as Hex;
const receiveUlnConfigBytes = (await publicClient.readContract({
  address: endpoint,
  abi: endpointArtifact.abi,
  functionName: "getConfig",
  args: [oapp, receiveUln, remoteEid, CONFIG_TYPE_ULN],
})) as Hex;

console.log(
  jsonStringify({
    endpoint,
    localEid,
    oapp,
    remoteEid,
    activeSendLibrary,
    activeReceiveLibrary,
    receiveLibraryIsDefault,
    inspectedLibraries: {
      sendUln,
      receiveUln,
    },
    executorConfig: decodeExecutorConfig(executorConfigBytes),
    sendUlnConfig: decodeUlnConfig(sendUlnConfigBytes),
    receiveUlnConfig: decodeUlnConfig(receiveUlnConfigBytes),
  }),
);
