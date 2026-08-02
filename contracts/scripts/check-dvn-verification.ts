import type { Address, Hex, PublicClient } from "viem";
import {
  assertDVNVerificationReceipt,
  type DVNVerificationStatus,
} from "./dvn-verification-status.js";
import { jsonStringify, loadABIArtifact } from "./lib.js";

const receiveUlnArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json"
);
const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json"
);

export type CheckDVNVerificationInput = {
  txHash: Hex;
  receiveUln: Address;
  requiredDVNs: Address[];
  confirmations: bigint;
  endpoint?: Address;
  expectedPayloadHash?: Hex;
  expectedSrcEid?: number;
  expectedDstEid?: number;
  expectedNonce?: bigint;
  expectedSender?: Address;
  expectedReceiver?: Address;
};

export async function checkDVNVerification(
  input: CheckDVNVerificationInput,
  publicClient: PublicClient
): Promise<void> {
  const receipt = await publicClient.getTransactionReceipt({
    hash: input.txHash,
  });
  if (receipt.status !== "success") {
    throw new Error(`transaction ${input.txHash} did not succeed`);
  }
  const hasExpectedPacket =
    input.expectedSrcEid !== undefined ||
    input.expectedDstEid !== undefined ||
    input.expectedNonce !== undefined ||
    input.expectedSender !== undefined ||
    input.expectedReceiver !== undefined;
  const status: DVNVerificationStatus = assertDVNVerificationReceipt({
    logs: receipt.logs,
    receiveUln: input.receiveUln,
    requiredDVNs: input.requiredDVNs,
    minConfirmations: input.confirmations,
    receiveUlnAbi: receiveUlnArtifact.abi,
    endpoint: input.endpoint,
    endpointAbi:
      input.endpoint === undefined ? undefined : endpointArtifact.abi,
    expectedPayloadHash: input.expectedPayloadHash,
    expectedPacket: hasExpectedPacket
      ? {
          srcEid: input.expectedSrcEid,
          dstEid: input.expectedDstEid,
          nonce: input.expectedNonce,
          sender: input.expectedSender,
          receiver: input.expectedReceiver,
        }
      : undefined,
  });

  console.log(
    jsonStringify({
      chainId: Number(await publicClient.getChainId()),
      txHash: input.txHash,
      status,
    })
  );
}
