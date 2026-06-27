import { getAddress } from "viem";
import type { Hex } from "viem";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
} from "./oft-canary-status.js";
import {
  createPublicClientFromEnv,
  envAddress,
  jsonStringify,
  loadABIArtifact,
  loadArtifact,
  optionalBigInt,
} from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);
const sendLibArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/SendLibBase.sol/SendLibBase.json",
);
const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);

const publicClient = createPublicClientFromEnv();
const endpoint = envAddress("ENDPOINT");
const sourceTxHash = process.env.SOURCE_TX_HASH;
const destinationTxHash = process.env.DESTINATION_TX_HASH;

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

if (sourceTxHash !== undefined && sourceTxHash !== "") {
  const sourceReceipt = await publicClient.getTransactionReceipt({
    hash: sourceTxHash as Hex,
  });
  if (sourceReceipt.status !== "success") {
    throw new Error(`source transaction ${sourceTxHash} did not succeed`);
  }
  output.sourceTxHash = sourceTxHash as Hex;
  output.source = assertCanarySourceReceipt({
    logs: sourceReceipt.logs,
    endpoint,
    sendLib: envAddress("SEND_LIB"),
    expectedExecutor: envAddress("OPEN_EXECUTOR"),
    endpointAbi: endpointArtifact.abi,
    sendLibAbi: sendLibArtifact.abi,
  });
}

if (destinationTxHash !== undefined && destinationTxHash !== "") {
  const destinationReceipt = await publicClient.getTransactionReceipt({
    hash: destinationTxHash as Hex,
  });
  if (destinationReceipt.status !== "success") {
    throw new Error(
      `destination transaction ${destinationTxHash} did not succeed`,
    );
  }
  output.destinationTxHash = destinationTxHash as Hex;
  output.destination = assertCanaryDestinationReceipt({
    logs: destinationReceipt.logs,
    endpoint: getAddress(process.env.DESTINATION_ENDPOINT ?? endpoint),
    endpointAbi: endpointArtifact.abi,
  });
}

const destinationTestOFT = process.env.DESTINATION_TEST_OFT;
const recipient = process.env.RECIPIENT;
const minRecipientBalance = optionalBigInt("MIN_RECIPIENT_BALANCE");
if (
  destinationTestOFT !== undefined ||
  recipient !== undefined ||
  minRecipientBalance !== undefined
) {
  if (
    destinationTestOFT === undefined ||
    destinationTestOFT === "" ||
    recipient === undefined ||
    recipient === "" ||
    minRecipientBalance === undefined
  ) {
    throw new Error(
      "DESTINATION_TEST_OFT, RECIPIENT, and MIN_RECIPIENT_BALANCE must be set together",
    );
  }
  const recipientAddress = getAddress(recipient);
  output.recipientBalance = assertCanaryRecipientBalance({
    recipient: recipientAddress,
    balance: (await publicClient.readContract({
      address: getAddress(destinationTestOFT),
      abi: testOFTArtifact.abi,
      functionName: "balanceOf",
      args: [recipientAddress],
    })) as bigint,
    minBalance: minRecipientBalance,
  });
}

if (
  output.source === undefined &&
  output.destination === undefined &&
  output.recipientBalance === undefined
) {
  throw new Error(
    "set SOURCE_TX_HASH, DESTINATION_TX_HASH, or DESTINATION_TEST_OFT/RECIPIENT/MIN_RECIPIENT_BALANCE",
  );
}

console.log(jsonStringify(output));
