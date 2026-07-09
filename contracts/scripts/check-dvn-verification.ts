import type { Hex } from "viem";
import {
  assertDVNVerificationReceipt,
  type DVNVerificationStatus,
} from "./dvn-verification-status.js";
import {
  assertConfiguredChain,
  createPublicClientFromEnv,
  envAddress,
  envAddressList,
  envBigInt,
  jsonStringify,
  loadABIArtifact,
  optionalAddress,
  optionalBigInt,
  optionalParam,
  requiredEnv,
} from "./lib.js";

const receiveUlnArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
);
const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);

const txHash = requiredEnv("TX_HASH") as Hex;
const publicClient = createPublicClientFromEnv();
await assertConfiguredChain(publicClient);
const receipt = await publicClient.getTransactionReceipt({ hash: txHash });
if (receipt.status !== "success") {
  throw new Error(`transaction ${txHash} did not succeed`);
}

const endpoint = optionalAddress("ENDPOINT");
const expectedPayloadHashParam = optionalParam("EXPECTED_PAYLOAD_HASH");
const expectedPayloadHash =
  expectedPayloadHashParam === undefined || expectedPayloadHashParam === ""
    ? undefined
    : (expectedPayloadHashParam as Hex);
const expectedSrcEid = optionalUint32("EXPECTED_SRC_EID");
const expectedDstEid = optionalUint32("EXPECTED_DST_EID");
const expectedNonce = optionalBigInt("EXPECTED_NONCE");
const expectedSender = optionalAddress("EXPECTED_SENDER");
const expectedReceiver = optionalAddress("EXPECTED_RECEIVER");

const status: DVNVerificationStatus = assertDVNVerificationReceipt({
  logs: receipt.logs,
  receiveUln: envAddress("RECEIVE_ULN"),
  requiredDVNs: envAddressList("REQUIRED_DVNS"),
  minConfirmations: envBigInt("CONFIRMATIONS"),
  receiveUlnAbi: receiveUlnArtifact.abi,
  endpoint,
  endpointAbi: endpoint === undefined ? undefined : endpointArtifact.abi,
  expectedPayloadHash,
  expectedPacket:
    expectedSrcEid === undefined &&
    expectedDstEid === undefined &&
    expectedNonce === undefined &&
    expectedSender === undefined &&
    expectedReceiver === undefined
      ? undefined
      : {
          srcEid: expectedSrcEid,
          dstEid: expectedDstEid,
          nonce: expectedNonce,
          sender: expectedSender,
          receiver: expectedReceiver,
        },
});

console.log(
  jsonStringify({
    chainId: Number(await publicClient.getChainId()),
    txHash,
    status,
  }),
);

function optionalUint32(name: string): number | undefined {
  const value = optionalBigInt(name);
  if (value === undefined) {
    return undefined;
  }
  if (value > 0xffffffffn) {
    throw new Error(`${name} exceeds uint32`);
  }
  return Number(value);
}
