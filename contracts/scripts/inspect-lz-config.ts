import type { Address, Hex, PublicClient } from "viem";
import {
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  decodeExecutorConfig,
  decodeUlnConfig,
  type ExecutorConfig,
  type UlnConfig,
} from "./lz-config.js";
import { loadABIArtifact } from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json"
);

export type InspectLzConfigInput = {
  endpoint: Address;
  oapp: Address;
  remoteEid: number;
  sendUln: Address;
  receiveUln: Address;
};

export type InspectLzConfigReport = {
  endpoint: Address;
  localEid: number;
  oapp: Address;
  remoteEid: number;
  activeSendLibrary: Address;
  activeReceiveLibrary: Address;
  receiveLibraryIsDefault: boolean;
  inspectedLibraries: {
    sendUln: Address;
    receiveUln: Address;
  };
  executorConfig: ExecutorConfig;
  sendUlnConfig: UlnConfig;
  receiveUlnConfig: UlnConfig;
};

export async function inspectLzConfig(
  input: InspectLzConfigInput,
  publicClient: PublicClient
): Promise<InspectLzConfigReport> {
  const localEid = (await publicClient.readContract({
    address: input.endpoint,
    abi: endpointArtifact.abi,
    functionName: "eid",
  })) as number;
  const activeSendLibrary = (await publicClient.readContract({
    address: input.endpoint,
    abi: endpointArtifact.abi,
    functionName: "getSendLibrary",
    args: [input.oapp, input.remoteEid],
  })) as Address;
  const [activeReceiveLibrary, receiveLibraryIsDefault] =
    (await publicClient.readContract({
      address: input.endpoint,
      abi: endpointArtifact.abi,
      functionName: "getReceiveLibrary",
      args: [input.oapp, input.remoteEid],
    })) as [Address, boolean];

  const executorConfigBytes = (await publicClient.readContract({
    address: input.endpoint,
    abi: endpointArtifact.abi,
    functionName: "getConfig",
    args: [input.oapp, input.sendUln, input.remoteEid, CONFIG_TYPE_EXECUTOR],
  })) as Hex;
  const sendUlnConfigBytes = (await publicClient.readContract({
    address: input.endpoint,
    abi: endpointArtifact.abi,
    functionName: "getConfig",
    args: [input.oapp, input.sendUln, input.remoteEid, CONFIG_TYPE_ULN],
  })) as Hex;
  const receiveUlnConfigBytes = (await publicClient.readContract({
    address: input.endpoint,
    abi: endpointArtifact.abi,
    functionName: "getConfig",
    args: [input.oapp, input.receiveUln, input.remoteEid, CONFIG_TYPE_ULN],
  })) as Hex;

  return {
    endpoint: input.endpoint,
    localEid,
    oapp: input.oapp,
    remoteEid: input.remoteEid,
    activeSendLibrary,
    activeReceiveLibrary,
    receiveLibraryIsDefault,
    inspectedLibraries: {
      sendUln: input.sendUln,
      receiveUln: input.receiveUln,
    },
    executorConfig: decodeExecutorConfig(executorConfigBytes),
    sendUlnConfig: decodeUlnConfig(sendUlnConfigBytes),
    receiveUlnConfig: decodeUlnConfig(receiveUlnConfigBytes),
  };
}
