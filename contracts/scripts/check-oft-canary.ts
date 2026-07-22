import type { Address, Hex, PublicClient } from "viem";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
} from "./oft-canary-status.js";
import { jsonStringify, loadABIArtifact, loadArtifact } from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json"
);
const sendLibArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/SendLibBase.sol/SendLibBase.json"
);
const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json"
);

export type CheckOFTCanaryInput = {
  endpoint: Address;
  sourceTxHash?: Hex;
  destinationTxHash?: Hex;
  sendLib?: Address;
  openExecutor?: Address;
  destinationEndpoint?: Address;
  destinationTestOFT?: Address;
  recipient?: Address;
  minRecipientBalance?: bigint;
};

export async function checkOFTCanary(
  input: CheckOFTCanaryInput,
  publicClient: PublicClient
): Promise<void> {
  const output: {
    chainId: number;
    sourceTxHash?: Hex;
    source?: ReturnType<typeof assertCanarySourceReceipt>;
    destinationTxHash?: Hex;
    destination?: { packetDelivered: boolean };
    recipientBalance?: {
      recipient: string;
      balance: bigint;
      minBalance: bigint;
    };
  } = {
    chainId: Number(await publicClient.getChainId()),
  };

  if (input.sourceTxHash !== undefined) {
    if (input.sendLib === undefined || input.openExecutor === undefined) {
      throw new Error(
        "input.sendLib and input.openExecutor are required with input.sourceTxHash"
      );
    }
    const sourceReceipt = await publicClient.getTransactionReceipt({
      hash: input.sourceTxHash,
    });
    if (sourceReceipt.status !== "success") {
      throw new Error(
        `source transaction ${input.sourceTxHash} did not succeed`
      );
    }
    output.sourceTxHash = input.sourceTxHash;
    output.source = assertCanarySourceReceipt({
      logs: sourceReceipt.logs,
      endpoint: input.endpoint,
      sendLib: input.sendLib,
      expectedExecutor: input.openExecutor,
      endpointAbi: endpointArtifact.abi,
      sendLibAbi: sendLibArtifact.abi,
    });
  }

  if (input.destinationTxHash !== undefined) {
    const destinationReceipt = await publicClient.getTransactionReceipt({
      hash: input.destinationTxHash,
    });
    if (destinationReceipt.status !== "success") {
      throw new Error(
        `destination transaction ${input.destinationTxHash} did not succeed`
      );
    }
    output.destinationTxHash = input.destinationTxHash;
    output.destination = assertCanaryDestinationReceipt({
      logs: destinationReceipt.logs,
      endpoint: input.destinationEndpoint ?? input.endpoint,
      endpointAbi: endpointArtifact.abi,
    });
  }

  if (
    input.destinationTestOFT !== undefined ||
    input.recipient !== undefined ||
    input.minRecipientBalance !== undefined
  ) {
    if (
      input.destinationTestOFT === undefined ||
      input.recipient === undefined ||
      input.minRecipientBalance === undefined
    ) {
      throw new Error(
        "DESTINATION_TEST_OFT, RECIPIENT, and MIN_RECIPIENT_BALANCE must be set together"
      );
    }
    output.recipientBalance = assertCanaryRecipientBalance({
      recipient: input.recipient,
      balance: (await publicClient.readContract({
        address: input.destinationTestOFT,
        abi: testOFTArtifact.abi,
        functionName: "balanceOf",
        args: [input.recipient],
      })) as bigint,
      minBalance: input.minRecipientBalance,
    });
  }

  if (
    output.source === undefined &&
    output.destination === undefined &&
    output.recipientBalance === undefined
  ) {
    throw new Error(
      "set SOURCE_TX_HASH, DESTINATION_TX_HASH, or DESTINATION_TEST_OFT/RECIPIENT/MIN_RECIPIENT_BALANCE"
    );
  }

  console.log(jsonStringify(output));
}
