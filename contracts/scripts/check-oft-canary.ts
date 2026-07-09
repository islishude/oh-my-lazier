import type { Hex } from "viem";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
} from "./oft-canary-status.js";
import {
  assertConfiguredChain,
  createPublicClientFromEnv,
  envAddress,
  jsonStringify,
  loadABIArtifact,
  loadArtifact,
  optionalAddress,
  optionalBigInt,
  optionalParam,
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
await assertConfiguredChain(publicClient);
const endpoint = envAddress("ENDPOINT");
const sourceTxHash = optionalParam("SOURCE_TX_HASH");
const destinationTxHash = optionalParam("DESTINATION_TX_HASH");

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
    endpoint: optionalAddress("DESTINATION_ENDPOINT") ?? endpoint,
    endpointAbi: endpointArtifact.abi,
  });
}

const destinationTestOFT = optionalAddress("DESTINATION_TEST_OFT");
const recipient = optionalAddress("RECIPIENT");
const minRecipientBalance = optionalBigInt("MIN_RECIPIENT_BALANCE");
if (
  destinationTestOFT !== undefined ||
  recipient !== undefined ||
  minRecipientBalance !== undefined
) {
  if (
    destinationTestOFT === undefined ||
    recipient === undefined ||
    minRecipientBalance === undefined
  ) {
    throw new Error(
      "DESTINATION_TEST_OFT, RECIPIENT, and MIN_RECIPIENT_BALANCE must be set together",
    );
  }
  output.recipientBalance = assertCanaryRecipientBalance({
    recipient,
    balance: (await publicClient.readContract({
      address: destinationTestOFT,
      abi: testOFTArtifact.abi,
      functionName: "balanceOf",
      args: [recipient],
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
