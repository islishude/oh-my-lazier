import { readFile } from "node:fs/promises";
import path from "node:path";
import {
  createPublicClient,
  createWalletClient,
  decodeEventLog,
  defineChain,
  encodeEventTopics,
  getAddress,
  http,
  isAddressEqual,
  keccak256,
  type Abi,
  type Address,
  type Hex,
  type Log,
  type PublicClient,
  type TransactionReceipt,
  type WalletClient,
} from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { buildCanarySendParam } from "./oft-canary.js";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
} from "./oft-canary-status.js";
import {
  jsonStringify,
  loadArtifact,
  optionalEnv,
  type Artifact,
} from "./lib.js";
import {
  validateLocalE2EDeployment,
  type LocalChainDeployment as ChainDeployment,
  type LocalE2EDeployment,
} from "./e2e-local-artifacts.js";

const tmpDir = optionalEnv("E2E_TMP_DIR", "tmp/e2e");
const deployerPrivateKey = normalizePrivateKey(
  optionalEnv(
    "E2E_DEPLOYER_PRIVATE_KEY",
    "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
  ),
);
const deployer = privateKeyToAccount(deployerPrivateKey);

const endpointArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/EndpointV2.sol/EndpointV2.json",
);
const sendUlnArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json",
);
const receiveUlnArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
);
const oftArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);
const openDVNArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
);

type Clients = {
  publicClient: PublicClient;
  walletClient: WalletClient;
};

type PacketDetails = {
  guid: Hex;
  packetHeader: Hex;
  payloadHash: Hex;
  sourceReceipt: TransactionReceipt;
};

const deployment: LocalE2EDeployment = validateLocalE2EDeployment(
  JSON.parse(await readFile(path.join(tmpDir, "deployments.json"), "utf8")),
);

const amountAB = 1n * 10n ** 18n;
const amountBA = amountAB / 2n;

await waitForWorkerReady();
await runDirection(deployment.chains.a, deployment.chains.b, amountAB);
await runDirection(deployment.chains.b, deployment.chains.a, amountBA);

console.log(
  jsonStringify({
    ok: true,
    directions: [
      `${deployment.chains.a.name}->${deployment.chains.b.name}`,
      `${deployment.chains.b.name}->${deployment.chains.a.name}`,
    ],
  }),
);

async function runDirection(
  source: ChainDeployment,
  destination: ChainDeployment,
  amountLD: bigint,
) {
  const sourceClients = clientsFor(source);
  const destinationClients = clientsFor(destination);
  const balanceBefore = await balanceOf(destinationClients.publicClient, destination.oft, deployer.address);
  const sendParam = buildCanarySendParam({
    dstEid: destination.eid,
    recipient: deployer.address,
    amountLD,
    minAmountLD: amountLD,
    lzReceiveGas: BigInt(deployment.parameters.lzReceiveGas),
  });
  const fee = await sourceClients.publicClient.readContract({
    address: source.oft,
    abi: oftArtifact.abi,
    functionName: "quoteSend",
    args: [sendParam, false],
  });
  const nativeFee = nativeFeeFromQuote(fee);
  const hash = await sourceClients.walletClient.writeContract({
    address: source.oft,
    abi: oftArtifact.abi,
    functionName: "send",
    args: [sendParam, { nativeFee, lzTokenFee: 0n }, deployer.address],
    account: deployer,
    chain: null,
    value: nativeFee,
  });
  const sourceReceipt =
    await sourceClients.publicClient.waitForTransactionReceipt({ hash });
  if (sourceReceipt.status !== "success") {
    throw new Error(`${source.name} OFT send ${hash} failed`);
  }

  assertCanarySourceReceipt({
    logs: sourceReceipt.logs,
    endpoint: source.endpoint,
    sendLib: source.sendUln,
    expectedExecutor: source.openExecutor,
    endpointAbi: endpointArtifact.abi,
    sendLibAbi: sendUlnArtifact.abi,
  });
  assertSourceDVNFees(sourceReceipt.logs, source);
  const packet = packetFromSourceReceipt(sourceReceipt, source);

  await submitSecondaryVerification(destinationClients, destination, packet);

  await waitForDelivery(
    sourceClients,
    destinationClients,
    source,
    destination,
    packet,
    balanceBefore + amountLD,
  );
}

async function submitSecondaryVerification(
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails,
) {
  const hash = await clients.walletClient.writeContract({
    address: destination.secondaryOpenDVN,
    abi: openDVNArtifact.abi,
    functionName: "submitVerification",
    args: [
      destination.receiveUln,
      packet.packetHeader,
      packet.payloadHash,
      BigInt(deployment.parameters.confirmations),
    ],
    account: deployer,
    chain: null,
  });
  const receipt = await clients.publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`secondary OpenDVN verification ${hash} failed`);
  }
}

async function waitForDelivery(
  sourceClients: Clients,
  destinationClients: Clients,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails,
  minBalance: bigint,
) {
  const started = Date.now();
  let lastError: unknown;
  while (Date.now() - started < 180_000) {
    await mine(sourceClients.publicClient);
    await mine(destinationClients.publicClient);
    try {
      const verified = await verifiedDVNs(
        destinationClients.publicClient,
        destination,
        packet,
      );
      if (
        !verified.has(destination.primaryOpenDVN.toLowerCase()) ||
        !verified.has(destination.secondaryOpenDVN.toLowerCase())
      ) {
        throw new Error(
          `${destination.name} missing PayloadVerified logs; observed ${[
            ...verified,
          ].join(",")}`,
        );
      }
      const deliveredLogs = await matchingPacketDeliveredLogs(
        destinationClients.publicClient,
        destination,
      );
      assertCanaryDestinationReceipt({
        logs: deliveredLogs,
        endpoint: destination.endpoint,
        endpointAbi: endpointArtifact.abi,
      });
      const balance = await balanceOf(
        destinationClients.publicClient,
        destination.oft,
        deployer.address,
      );
      assertCanaryRecipientBalance({
        recipient: deployer.address,
        balance,
        minBalance,
      });
      return;
    } catch (err) {
      lastError = err;
      await sleep(1_000);
    }
  }
  throw new Error(
    `timed out waiting for ${source.name}->${destination.name} delivery: ${String(
      lastError instanceof Error ? lastError.message : lastError,
    )}`,
  );
}

function packetFromSourceReceipt(
  receipt: TransactionReceipt,
  source: ChainDeployment,
): PacketDetails {
  for (const log of receipt.logs) {
    if (!isAddressEqual(log.address, source.endpoint)) {
      continue;
    }
    if (log.topics[0] !== eventTopic(endpointArtifact, "PacketSent")) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: endpointArtifact.abi,
      eventName: "PacketSent",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as { encodedPayload: Hex };
    const encodedPayload = args.encodedPayload;
    if ((encodedPayload.length - 2) / 2 < 113) {
      throw new Error("PacketSent encodedPayload is shorter than PacketV1");
    }
    const packetHeader = sliceHex(encodedPayload, 0, 81);
    const payloadHash = keccak256(sliceHex(encodedPayload, 81));
    const guid = sliceHex(encodedPayload, 81, 113);
    return { guid, packetHeader, payloadHash, sourceReceipt: receipt };
  }
  throw new Error("source receipt is missing PacketSent");
}

function assertSourceDVNFees(logs: readonly Log[], source: ChainDeployment) {
  for (const log of logs) {
    if (!isAddressEqual(log.address, source.sendUln)) {
      continue;
    }
    if (log.topics[0] !== eventTopic(sendUlnArtifact, "DVNFeePaid")) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: sendUlnArtifact.abi,
      eventName: "DVNFeePaid",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      requiredDVNs: Address[];
      fees: bigint[];
    };
    const requiredDVNs = args.requiredDVNs.map((address) =>
      getAddress(address).toLowerCase(),
    );
    for (const required of [source.primaryOpenDVN, source.secondaryOpenDVN]) {
      if (!requiredDVNs.includes(required.toLowerCase())) {
        throw new Error(
          `DVNFeePaid missing source DVN ${required} on ${source.name}`,
        );
      }
    }
    if (args.fees.length < 2) {
      throw new Error("DVNFeePaid has fewer than two fees");
    }
    return;
  }
  throw new Error("source receipt is missing SendUln302 DVNFeePaid");
}

async function verifiedDVNs(
  publicClient: PublicClient,
  destination: ChainDeployment,
  packet: PacketDetails,
): Promise<Set<string>> {
  const logs = await publicClient.getLogs({
    address: destination.receiveUln,
    fromBlock: 0n,
    toBlock: "latest",
  });
  const out = new Set<string>();
  for (const log of logs) {
    if (log.topics[0] !== eventTopic(receiveUlnArtifact, "PayloadVerified")) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: receiveUlnArtifact.abi,
      eventName: "PayloadVerified",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      dvn: Address;
      header: Hex;
      proofHash: Hex;
    };
    if (
      args.header.toLowerCase() === packet.packetHeader.toLowerCase() &&
      args.proofHash.toLowerCase() === packet.payloadHash.toLowerCase()
    ) {
      out.add(getAddress(args.dvn).toLowerCase());
    }
  }
  return out;
}

async function matchingPacketDeliveredLogs(
  publicClient: PublicClient,
  destination: ChainDeployment,
): Promise<Log[]> {
  const logs = await publicClient.getLogs({
    address: destination.endpoint,
    fromBlock: 0n,
    toBlock: "latest",
  });
  return logs.filter(
    (log) => log.topics[0] === eventTopic(endpointArtifact, "PacketDelivered"),
  );
}

async function balanceOf(
  publicClient: PublicClient,
  token: Address,
  account: Address,
): Promise<bigint> {
  return (await publicClient.readContract({
    address: token,
    abi: oftArtifact.abi,
    functionName: "balanceOf",
    args: [account],
  })) as bigint;
}

function clientsFor(chain: ChainDeployment): Clients {
  const viemChain = defineChain({
    id: chain.chainId,
    name: chain.name,
    nativeCurrency: { name: "Ether", symbol: "ETH", decimals: 18 },
    rpcUrls: { default: { http: [chain.hostRpcUrl] } },
  });
  const transport = http(chain.hostRpcUrl);
  return {
    publicClient: createPublicClient({ chain: viemChain, transport }),
    walletClient: createWalletClient({
      account: deployer,
      chain: viemChain,
      transport,
    }),
  };
}

async function waitForWorkerReady() {
  const url = optionalEnv(
    "E2E_WORKER_READY_URL",
    "http://127.0.0.1:19090/readyz",
  );
  const started = Date.now();
  while (Date.now() - started < 60_000) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // keep polling
    }
    await sleep(1_000);
  }
  throw new Error(`worker readiness endpoint did not become healthy at ${url}`);
}

async function mine(publicClient: PublicClient) {
  await publicClient.request({
    method: "anvil_mine" as never,
    params: ["0x1"] as never,
  });
}

function nativeFeeFromQuote(value: unknown): bigint {
  if (Array.isArray(value)) {
    return BigInt(value[0]);
  }
  const record = value as { nativeFee?: bigint; 0?: bigint };
  if (record.nativeFee !== undefined) {
    return record.nativeFee;
  }
  if (record[0] !== undefined) {
    return record[0];
  }
  throw new Error(`unexpected quoteSend return: ${jsonStringify(value)}`);
}

function eventTopic(artifact: Artifact, eventName: string): Hex {
  const topic = encodeEventTopics({
    abi: artifact.abi,
    eventName,
  })[0];
  if (topic === null || Array.isArray(topic)) {
    throw new Error(`event ${eventName} did not produce a single topic`);
  }
  return topic;
}

function sliceHex(value: Hex, start: number, end?: number): Hex {
  const body = value.slice(2);
  return `0x${body.slice(start * 2, end === undefined ? undefined : end * 2)}` as Hex;
}

function mutableTopics(topics: readonly Hex[]): [Hex, ...Hex[]] {
  if (topics.length === 0) {
    throw new Error("log is missing topics");
  }
  return [...topics] as [Hex, ...Hex[]];
}

function normalizePrivateKey(value: string): Hex {
  const normalized = value.startsWith("0x") ? value : `0x${value}`;
  if (!/^0x[0-9a-fA-F]{64}$/.test(normalized)) {
    throw new Error("private key must be a 32-byte hex value");
  }
  return normalized as Hex;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
