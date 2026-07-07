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
  sourceWorkerFeeClaims,
  type SourceWorkerFeeClaim,
} from "./e2e-fee-withdrawal.js";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
} from "./oft-canary-status.js";
import {
  addressToBytes32,
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
const withdrawalRecipient = getAddress(
  "0x000000000000000000000000000000000000fee1",
);

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
const openExecutorArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
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
  nonce: bigint;
  packetHeader: Hex;
  payloadHash: Hex;
  sourceReceipt: TransactionReceipt;
};

const deploymentPath = path.join(tmpDir, "deployments.json");
const deployment: LocalE2EDeployment = validateLocalE2EDeployment(
  JSON.parse(await readFile(deploymentPath, "utf8")),
);

const amountAB = 1n * 10n ** 18n;
const amountBA = amountAB / 2n;

logStep("loaded deployment", {
  path: deploymentPath,
  chain_a: `${deployment.chains.a.name}:${deployment.chains.a.eid}`,
  chain_b: `${deployment.chains.b.name}:${deployment.chains.b.eid}`,
  deployer: deployer.address,
});

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
  const direction = directionLabel(source, destination);
  logStep("direction started", {
    direction,
    src_eid: source.eid,
    dst_eid: destination.eid,
    amount_ld: amountLD,
  });
  const sourceClients = clientsFor(source);
  const destinationClients = clientsFor(destination);
  const balanceBefore = await balanceOf(
    destinationClients.publicClient,
    destination.oft,
    deployer.address,
  );
  logStep("destination balance before send", {
    direction,
    recipient: deployer.address,
    balance: balanceBefore,
  });
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
  logStep("quoted OFT send", {
    direction,
    native_fee: nativeFee,
    lz_receive_gas: deployment.parameters.lzReceiveGas,
  });
  const hash = await sourceClients.walletClient.writeContract({
    address: source.oft,
    abi: oftArtifact.abi,
    functionName: "send",
    args: [sendParam, { nativeFee, lzTokenFee: 0n }, deployer.address],
    account: deployer,
    chain: null,
    value: nativeFee,
  });
  logStep("OFT send submitted", { direction, tx: hash });
  const sourceReceipt =
    await sourceClients.publicClient.waitForTransactionReceipt({ hash });
  if (sourceReceipt.status !== "success") {
    throw new Error(`${source.name} OFT send ${hash} failed`);
  }
  logStep("OFT send confirmed", {
    direction,
    tx: hash,
    block: sourceReceipt.blockNumber,
    gas_used: sourceReceipt.gasUsed,
    logs: sourceReceipt.logs.length,
  });

  const sourceStatus = assertCanarySourceReceipt({
    logs: sourceReceipt.logs,
    endpoint: source.endpoint,
    sendLib: source.sendUln,
    expectedExecutor: source.openExecutor,
    endpointAbi: endpointArtifact.abi,
    sendLibAbi: sendUlnArtifact.abi,
  });
  logStep("source receipt assertions passed", {
    direction,
    send_library: sourceStatus.sendLibrary,
    executor: sourceStatus.executor,
    executor_fee: sourceStatus.executorFee,
  });
  const feeClaims = sourceWorkerFeeClaims({
    sourceName: source.name,
    logs: sourceReceipt.logs,
    sendLib: source.sendUln,
    sendLibAbi: sendUlnArtifact.abi,
    openExecutor: source.openExecutor,
    primaryOpenDVN: source.primaryOpenDVN,
    secondaryOpenDVN: source.secondaryOpenDVN,
    executorFee: sourceStatus.executorFee,
  });
  logStep("source worker fee claims decoded", {
    direction,
    claims: feeClaims.map((claim) => ({
      role: claim.role,
      worker: claim.worker,
      amount: claim.amount.toString(),
    })),
  });
  await withdrawSourceWorkerFees(
    sourceClients,
    source,
    feeClaims,
  );
  const packet = packetFromSourceReceipt(sourceReceipt, source);
  logStep("packet extracted", {
    direction,
    guid: packet.guid,
    nonce: packet.nonce,
    payload_hash: packet.payloadHash,
  });

  await submitSecondaryVerification(destinationClients, destination, packet);

  await waitForDelivery(
    sourceClients,
    destinationClients,
    source,
    destination,
    packet,
    balanceBefore + amountLD,
  );
  logStep("direction completed", {
    direction,
    min_balance: balanceBefore + amountLD,
  });
}

async function submitSecondaryVerification(
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails,
) {
  logStep("secondary OpenDVN verification submitting", {
    chain: destination.name,
    dvn: destination.secondaryOpenDVN,
    receive_uln: destination.receiveUln,
    payload_hash: packet.payloadHash,
  });
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
  logStep("secondary OpenDVN verification submitted", {
    chain: destination.name,
    tx: hash,
  });
  const receipt = await clients.publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`secondary OpenDVN verification ${hash} failed`);
  }
  logStep("secondary OpenDVN verification confirmed", {
    chain: destination.name,
    tx: hash,
    block: receipt.blockNumber,
    gas_used: receipt.gasUsed,
  });
}

async function waitForDelivery(
  sourceClients: Clients,
  destinationClients: Clients,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails,
  minBalance: bigint,
) {
  const direction = directionLabel(source, destination);
  const started = Date.now();
  let attempts = 0;
  let lastProgressAt = 0;
  let loggedPayloadVerified = false;
  let loggedPrimaryVerifier = false;
  let loggedDeliveryReceipt = false;
  let lastError: unknown;
  logStep("waiting for delivery", {
    direction,
    nonce: packet.nonce,
    payload_hash: packet.payloadHash,
    min_balance: minBalance,
  });
  while (Date.now() - started < 180_000) {
    attempts++;
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
      if (!loggedPayloadVerified) {
        logStep("payload verified by required DVNs", {
          direction,
          dvns: [...verified].sort(),
        });
        loggedPayloadVerified = true;
      }
      const primaryVerifier = await assertPrimaryDVNVerifier(
        destinationClients.publicClient,
        destination,
        packet,
      );
      if (!loggedPrimaryVerifier) {
        logStep("primary OpenDVN verifier assertion passed", {
          direction,
          verifier: primaryVerifier,
        });
        loggedPrimaryVerifier = true;
      }
      const deliveryReceipt = await matchingPacketDeliveredReceipt(
        destinationClients.publicClient,
        source,
        destination,
        packet,
      );
      if (!loggedDeliveryReceipt) {
        logStep("PacketDelivered receipt found", {
          direction,
          tx: deliveryReceipt.transactionHash,
          block: deliveryReceipt.blockNumber,
          logs: deliveryReceipt.logs.length,
        });
        loggedDeliveryReceipt = true;
      }
      await assertTransactionFrom(
        destinationClients.publicClient,
        deliveryReceipt.transactionHash,
        destination.executorSigner,
        `${destination.name} PacketDelivered`,
      );
      logStep("delivery transaction signer assertion passed", {
        direction,
        signer: destination.executorSigner,
      });
      assertCanaryDestinationReceipt({
        logs: deliveryReceipt.logs,
        endpoint: destination.endpoint,
        endpointAbi: endpointArtifact.abi,
      });
      logStep("destination receipt assertions passed", { direction });
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
      logStep("recipient balance assertion passed", {
        direction,
        recipient: deployer.address,
        balance,
        min_balance: minBalance,
      });
      return;
    } catch (err) {
      lastError = err;
      const now = Date.now();
      if (now - lastProgressAt >= 10_000) {
        logStep("delivery pending", {
          direction,
          attempts,
          elapsed_ms: now - started,
          last_error: err instanceof Error ? err.message : String(err),
        });
        lastProgressAt = now;
      }
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
    const nonce = BigInt(sliceHex(packetHeader, 1, 9));
    return { guid, nonce, packetHeader, payloadHash, sourceReceipt: receipt };
  }
  throw new Error("source receipt is missing PacketSent");
}

async function withdrawSourceWorkerFees(
  clients: Clients,
  source: ChainDeployment,
  claims: readonly SourceWorkerFeeClaim[],
) {
  for (const claim of claims) {
    await withdrawSourceWorkerFee(clients, source, claim);
  }
}

async function withdrawSourceWorkerFee(
  clients: Clients,
  source: ChainDeployment,
  claim: SourceWorkerFeeClaim,
) {
  const ledgerBefore = await sendLibFeeBalance(
    clients.publicClient,
    source.sendUln,
    claim.worker,
  );
  logStep("source worker fee ledger checked", {
    chain: source.name,
    role: claim.role,
    worker: claim.worker,
    amount: claim.amount,
    ledger: ledgerBefore,
  });
  if (ledgerBefore !== claim.amount) {
    throw new Error(
      `${source.name} ${claim.role} fee ledger ${ledgerBefore} does not match expected ${claim.amount}`,
    );
  }
  const recipientBefore = await clients.publicClient.getBalance({
    address: withdrawalRecipient,
  });
  const sendLibBefore = await clients.publicClient.getBalance({
    address: source.sendUln,
  });

  const hash = await clients.walletClient.writeContract({
    address: claim.worker,
    abi: workerArtifactForClaim(claim).abi,
    functionName: "withdrawFee",
    args: [source.sendUln, withdrawalRecipient, claim.amount],
    account: deployer,
    chain: null,
  });
  logStep("source worker fee withdrawal submitted", {
    chain: source.name,
    role: claim.role,
    tx: hash,
  });
  const receipt = await clients.publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`${source.name} ${claim.role} fee withdrawal ${hash} failed`);
  }
  assertFeeWithdrawalReceipt(source, claim, receipt.logs);
  logStep("source worker fee withdrawal confirmed", {
    chain: source.name,
    role: claim.role,
    tx: hash,
    block: receipt.blockNumber,
    gas_used: receipt.gasUsed,
  });

  const ledgerAfter = await sendLibFeeBalance(
    clients.publicClient,
    source.sendUln,
    claim.worker,
  );
  if (ledgerAfter !== 0n) {
    throw new Error(
      `${source.name} ${claim.role} fee ledger ${ledgerAfter} after withdrawal, want 0`,
    );
  }
  const recipientAfter = await clients.publicClient.getBalance({
    address: withdrawalRecipient,
  });
  if (recipientAfter - recipientBefore !== claim.amount) {
    throw new Error(
      `${source.name} withdrawal recipient balance delta ${
        recipientAfter - recipientBefore
      } does not match ${claim.amount}`,
    );
  }
  const sendLibAfter = await clients.publicClient.getBalance({
    address: source.sendUln,
  });
  if (sendLibBefore - sendLibAfter !== claim.amount) {
    throw new Error(
      `${source.name} SendUln302 balance delta ${
        sendLibBefore - sendLibAfter
      } does not match ${claim.amount}`,
    );
  }
  logStep("source worker fee withdrawal assertions passed", {
    chain: source.name,
    role: claim.role,
    ledger_after: ledgerAfter,
    recipient_delta: recipientAfter - recipientBefore,
    send_lib_delta: sendLibBefore - sendLibAfter,
  });
}

async function sendLibFeeBalance(
  publicClient: PublicClient,
  sendLib: Address,
  worker: Address,
): Promise<bigint> {
  return (await publicClient.readContract({
    address: sendLib,
    abi: sendUlnArtifact.abi,
    functionName: "fees",
    args: [worker],
  })) as bigint;
}

function assertFeeWithdrawalReceipt(
  source: ChainDeployment,
  claim: SourceWorkerFeeClaim,
  logs: readonly Log[],
) {
  let sawWorkerEvent = false;
  let sawSendLibEvent = false;
  for (const log of logs) {
    if (
      isAddressEqual(log.address, claim.worker) &&
      log.topics[0] ===
        eventTopic(workerArtifactForClaim(claim), "SendLibFeeWithdrawn")
    ) {
      const decoded = decodeEventLog({
        abi: workerArtifactForClaim(claim).abi,
        eventName: "SendLibFeeWithdrawn",
        data: log.data,
        topics: mutableTopics(log.topics),
      });
      const args = decoded.args as unknown as {
        sendLib: Address;
        recipient: Address;
        amount: bigint;
      };
      sawWorkerEvent =
        isAddressEqual(args.sendLib, source.sendUln) &&
        isAddressEqual(args.recipient, withdrawalRecipient) &&
        args.amount === claim.amount;
    }
    if (
      isAddressEqual(log.address, source.sendUln) &&
      log.topics[0] === eventTopic(sendUlnArtifact, "NativeFeeWithdrawn")
    ) {
      const decoded = decodeEventLog({
        abi: sendUlnArtifact.abi,
        eventName: "NativeFeeWithdrawn",
        data: log.data,
        topics: mutableTopics(log.topics),
      });
      const args = decoded.args as unknown as {
        worker: Address;
        receiver: Address;
        amount: bigint;
      };
      sawSendLibEvent =
        isAddressEqual(args.worker, claim.worker) &&
        isAddressEqual(args.receiver, withdrawalRecipient) &&
        args.amount === claim.amount;
    }
  }
  if (!sawWorkerEvent) {
    throw new Error(`${source.name} ${claim.role} withdrawal missing worker event`);
  }
  if (!sawSendLibEvent) {
    throw new Error(
      `${source.name} ${claim.role} withdrawal missing SendUln302 event`,
    );
  }
}

function workerArtifactForClaim(claim: SourceWorkerFeeClaim): Artifact {
  return claim.role === "open_executor" ? openExecutorArtifact : openDVNArtifact;
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

async function assertPrimaryDVNVerifier(
  publicClient: PublicClient,
  destination: ChainDeployment,
  packet: PacketDetails,
): Promise<Address> {
  const logs = await publicClient.getLogs({
    address: destination.primaryOpenDVN,
    fromBlock: 0n,
    toBlock: "latest",
  });
  const packetHeaderHash = keccak256(packet.packetHeader);
  for (const log of logs) {
    if (
      log.topics[0] !==
      eventTopic(openDVNArtifact, "DVNVerificationSubmitted")
    ) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: openDVNArtifact.abi,
      eventName: "DVNVerificationSubmitted",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      verifier: Address;
      receiveLib: Address;
      payloadHash: Hex;
      packetHeaderHash: Hex;
    };
    if (
      isAddressEqual(args.receiveLib, destination.receiveUln) &&
      args.payloadHash.toLowerCase() === packet.payloadHash.toLowerCase() &&
      args.packetHeaderHash.toLowerCase() === packetHeaderHash.toLowerCase()
    ) {
      if (!isAddressEqual(args.verifier, destination.dvnSigner)) {
        throw new Error(
          `${destination.name} primary OpenDVN verifier ${args.verifier} does not match configured DVN signer ${destination.dvnSigner}`,
        );
      }
      return getAddress(args.verifier);
    }
  }
  throw new Error(
    `${destination.name} primary OpenDVN is missing DVNVerificationSubmitted for ${packet.payloadHash}`,
  );
}

async function matchingPacketDeliveredReceipt(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails,
): Promise<TransactionReceipt> {
  const logs = await matchingPacketDeliveredLogs(
    publicClient,
    source,
    destination,
    packet,
  );
  const txHash = logs[0]?.transactionHash;
  if (txHash === undefined || txHash === null) {
    throw new Error(
      `${destination.name} PacketDelivered log is missing transaction hash`,
    );
  }
  return publicClient.getTransactionReceipt({ hash: txHash });
}

async function matchingPacketDeliveredLogs(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails,
): Promise<Log[]> {
  const logs = await publicClient.getLogs({
    address: destination.endpoint,
    fromBlock: 0n,
    toBlock: "latest",
  });
  return logs.filter((log) => {
    if (log.topics[0] !== eventTopic(endpointArtifact, "PacketDelivered")) {
      return false;
    }
    const decoded = decodeEventLog({
      abi: endpointArtifact.abi,
      eventName: "PacketDelivered",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      origin: { srcEid: number; sender: Hex; nonce: bigint };
      receiver: Address;
    };
    return (
      args.origin.srcEid === source.eid &&
      args.origin.nonce === packet.nonce &&
      args.origin.sender.toLowerCase() ===
        addressToBytes32(source.oft).toLowerCase() &&
      isAddressEqual(args.receiver, destination.oft)
    );
  });
}

async function assertTransactionFrom(
  publicClient: PublicClient,
  txHash: Hex,
  expected: Address,
  label: string,
): Promise<void> {
  const tx = await publicClient.getTransaction({ hash: txHash });
  if (!isAddressEqual(tx.from, expected)) {
    throw new Error(
      `${label} transaction sender ${tx.from} does not match expected signer ${expected}`,
    );
  }
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
  logStep("waiting for worker readiness", { url });
  while (Date.now() - started < 60_000) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        logStep("worker readiness healthy", {
          url,
          elapsed_ms: Date.now() - started,
        });
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

function directionLabel(source: ChainDeployment, destination: ChainDeployment): string {
  return `${source.name}->${destination.name}`;
}

function logStep(message: string, fields: Record<string, unknown> = {}) {
  const suffix = Object.entries(fields)
    .map(([key, value]) => `${key}=${formatLogValue(value)}`)
    .join(" ");
  console.error(`[e2e-local-run] ${message}${suffix === "" ? "" : ` ${suffix}`}`);
}

function formatLogValue(value: unknown): string {
  if (typeof value === "bigint") {
    return value.toString();
  }
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return String(value);
  }
  if (value === null) {
    return "null";
  }
  if (value === undefined) {
    return "undefined";
  }
  return jsonStringify(value);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
