import type { Hex } from "viem";
import {
  assertDVNVerificationReceipt,
  type DVNVerificationStatus,
} from "./dvn-verification-status.js";
import {
  createPublicClientFromEnv,
  envAddress,
  envBigInt,
  jsonStringify,
  loadABIArtifact,
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
const receipt = await publicClient.getTransactionReceipt({ hash: txHash });
if (receipt.status !== "success") {
  throw new Error(`transaction ${txHash} did not succeed`);
}

const endpoint =
  process.env.ENDPOINT === undefined || process.env.ENDPOINT === ""
    ? undefined
    : envAddress("ENDPOINT");
const expectedPayloadHash =
  process.env.EXPECTED_PAYLOAD_HASH === undefined ||
  process.env.EXPECTED_PAYLOAD_HASH === ""
    ? undefined
    : (requiredEnv("EXPECTED_PAYLOAD_HASH") as Hex);

const status: DVNVerificationStatus = assertDVNVerificationReceipt({
  logs: receipt.logs,
  receiveUln: envAddress("RECEIVE_ULN"),
  requiredDVNs: [envAddress("OPEN_DVN"), envAddress("LAYERZERO_LABS_DVN")],
  minConfirmations: envBigInt("CONFIRMATIONS"),
  receiveUlnAbi: receiveUlnArtifact.abi,
  endpoint,
  endpointAbi: endpoint === undefined ? undefined : endpointArtifact.abi,
  expectedPayloadHash,
});

console.log(
  jsonStringify({
    chainId: Number(await publicClient.getChainId()),
    txHash,
    status,
  }),
);
