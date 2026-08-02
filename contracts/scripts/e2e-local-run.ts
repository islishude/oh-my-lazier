import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { EthereumProvider } from "hardhat/types/providers";
import {
  decodeEventLog,
  encodeFunctionData,
  encodeEventTopics,
  getAddress,
  isAddressEqual,
  keccak256,
  type Account,
  type Abi,
  type Address,
  type Hex,
  type Log,
  type PublicClient,
  type TransactionReceipt,
  type WalletClient,
} from "viem";
import {
  type ApplyGate,
  type WriteNetworkContext,
  withWriteConnection,
} from "./command-harness.js";
import { buildCanarySendParam } from "./oft-canary.js";
import {
  sourceExecutorFeeTotal,
  sourceWorkerFeeClaims,
  type SourceWorkerFeeClaim,
} from "./e2e-fee-withdrawal.js";
import {
  destinationReplayEvidence,
  multiSendIndexerEvidence,
  packetsFromSourceReceipt,
  requirePacketCount,
  type DestinationReplayObservation,
  type PacketDetails,
} from "./e2e-local-indexer-evidence.js";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
} from "./oft-canary-status.js";
import {
  addressToBytes32,
  jsonStringify,
  loadArtifact,
  type Artifact,
} from "./lib.js";
import {
  validateLocalE2EDeployment,
  type LocalChainDeployment as ChainDeployment,
  type LocalE2EDeployment,
} from "./e2e-local-artifacts.js";

const withdrawalRecipient = getAddress(
  "0x000000000000000000000000000000000000fee1"
);

const endpointArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/EndpointV2.sol/EndpointV2.json"
);
const sendUlnArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json"
);
const receiveUlnArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json"
);
const oftArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json"
);
const openExecutorArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json"
);
const openDVNArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json"
);

type Clients = {
  provider: EthereumProvider;
  publicClient: PublicClient;
  walletClient: WalletClient;
  account: Account;
};

type LocalE2ERunState = {
  deployment: LocalE2EDeployment;
  deployerAddress: Address;
  clients: Record<"a" | "b", Clients>;
  multiSendIndexerEvidencePath: string;
  destinationReplayEvidencePath: string;
};

type RunDirectionOptions = {
  exerciseRBF?: boolean;
};

type PendingRPCTransaction = {
  hash: Hex;
  from: Address;
  to: Address | null;
  input: Hex;
  nonce: Hex;
  gasPrice?: Hex;
  maxFeePerGas?: Hex;
  maxPriorityFeePerGas?: Hex;
};

type PendingRPCBlock = {
  transactions: readonly PendingRPCTransaction[];
};

type PendingCommitVerificationTx = {
  hash: Hex;
  nonce: bigint;
  gasPrice?: bigint;
  maxFeePerGas?: bigint;
  maxPriorityFeePerGas?: bigint;
};

const amountAB = 1n * 10n ** 18n;
const amountBA = amountAB / 2n;
const multiSendAmountsAB = [amountAB / 4n, amountAB / 5n] as const;

export const LOCAL_E2E_RUN_NETWORKS = {
  chainA: "local-anvil-a",
  chainB: "local-anvil-b",
} as const;

const defaultWorkerReadyUrl = "http://127.0.0.1:19090/readyz";

export type LocalE2ERunBusinessInput = {
  tmpDir: string;
  workerReadyUrl?: string;
};

export type LocalE2ERunInput = {
  tmpDir: string;
  workerReadyUrl: string;
};

export type LocalE2ERunContext = {
  hre: HardhatRuntimeEnvironment;
  gate: Pick<ApplyGate, "authorize">;
  fetch?: typeof fetch;
};

export type LocalE2ERunResult =
  | {
      applied: false;
      directions: readonly string[];
    }
  | {
      applied: true;
      ok: true;
      directions: readonly string[];
    };

/** Resolve and validate the non-secret input for the local E2E scenarios. */
export function resolveLocalE2ERunInput(
  input: LocalE2ERunBusinessInput
): LocalE2ERunInput {
  if (input.tmpDir.trim() === "") {
    throw new Error("tmpDir must not be empty");
  }
  const workerReadyUrl = input.workerReadyUrl ?? defaultWorkerReadyUrl;
  let parsedWorkerReadyUrl: URL;
  try {
    parsedWorkerReadyUrl = new URL(workerReadyUrl);
  } catch {
    throw new Error("workerReadyUrl must be a valid URL");
  }
  if (
    parsedWorkerReadyUrl.protocol !== "http:" &&
    parsedWorkerReadyUrl.protocol !== "https:"
  ) {
    throw new Error("workerReadyUrl must use http or https");
  }
  return { tmpDir: input.tmpDir, workerReadyUrl };
}

/** Run every stateful local E2E scenario over the two named Hardhat networks. */
export async function runLocalE2E(
  input: LocalE2ERunInput,
  context: LocalE2ERunContext
): Promise<LocalE2ERunResult> {
  const deploymentPath = path.join(input.tmpDir, "deployments.json");
  const deployment = validateLocalE2ERunDeployment(
    validateLocalE2EDeployment(
      JSON.parse(await readFile(deploymentPath, "utf8")) as unknown
    )
  );
  const directions = [
    directionLabel(deployment.chains.a, deployment.chains.b),
    directionLabel(deployment.chains.b, deployment.chains.a),
  ] as const;

  logStep("loaded deployment", {
    path: deploymentPath,
    chain_a: `${deployment.chains.a.name}:${deployment.chains.a.eid}`,
    chain_b: `${deployment.chains.b.name}:${deployment.chains.b.eid}`,
    deployer: deployment.deployer,
  });

  const applied = await context.gate.authorize({
    command: "e2e:run-local",
    targets: [
      {
        network: LOCAL_E2E_RUN_NETWORKS.chainA,
        chainId: deployment.chains.a.chainId,
      },
      {
        network: LOCAL_E2E_RUN_NETWORKS.chainB,
        chainId: deployment.chains.b.chainId,
      },
    ],
    actions: [
      "run bidirectional OFT sends and delivery assertions",
      "exercise worker transaction replacement and fee withdrawals",
      "run the multi-send indexer and destination replay scenarios",
    ],
  });
  if (!applied) {
    return { applied: false, directions };
  }

  await waitForWorkerReady(input.workerReadyUrl, context.fetch ?? fetch);
  return await withWriteConnection(
    context.hre,
    {
      network: LOCAL_E2E_RUN_NETWORKS.chainA,
      expectedChainId: deployment.chains.a.chainId,
    },
    async (chainAContext) =>
      await withWriteConnection(
        context.hre,
        {
          network: LOCAL_E2E_RUN_NETWORKS.chainB,
          expectedChainId: deployment.chains.b.chainId,
        },
        async (chainBContext) => {
          const state = createRunState(
            input,
            deployment,
            chainAContext,
            chainBContext
          );
          await runDirection(
            state,
            deployment.chains.a,
            deployment.chains.b,
            amountAB,
            { exerciseRBF: true }
          );
          await runDirection(
            state,
            deployment.chains.b,
            deployment.chains.a,
            amountBA
          );
          await runMultiSendIndexerScenario(
            state,
            deployment.chains.a,
            deployment.chains.b,
            multiSendAmountsAB
          );
          return { applied: true, ok: true, directions };
        }
      )
  );
}

export function validateLocalE2ERunDeployment(
  deployment: LocalE2EDeployment
): LocalE2EDeployment {
  const expected = [
    [deployment.chains.a, "a", LOCAL_E2E_RUN_NETWORKS.chainA, 31337],
    [deployment.chains.b, "b", LOCAL_E2E_RUN_NETWORKS.chainB, 31338],
  ] as const;
  for (const [chain, key, name, chainId] of expected) {
    if (chain.key !== key || chain.name !== name || chain.chainId !== chainId) {
      throw new Error(
        `deployment chain ${key} must use Hardhat network ${name} with chain id ${chainId}`
      );
    }
  }
  return deployment;
}

function createRunState(
  input: LocalE2ERunInput,
  deployment: LocalE2EDeployment,
  chainAContext: WriteNetworkContext,
  chainBContext: WriteNetworkContext
): LocalE2ERunState {
  const chainAClients = clientsFromContext(chainAContext);
  const chainBClients = clientsFromContext(chainBContext);
  if (
    !isAddressEqual(
      chainAClients.account.address,
      chainBClients.account.address
    )
  ) {
    throw new Error(
      `local E2E networks use different deployers: ${chainAClients.account.address} and ${chainBClients.account.address}`
    );
  }
  if (!isAddressEqual(chainAClients.account.address, deployment.deployer)) {
    throw new Error(
      `configured deployer ${chainAClients.account.address} does not match deployment deployer ${deployment.deployer}`
    );
  }
  return {
    deployment,
    deployerAddress: getAddress(chainAClients.account.address),
    clients: { a: chainAClients, b: chainBClients },
    multiSendIndexerEvidencePath: path.join(
      input.tmpDir,
      "multi-oft-send-indexer.json"
    ),
    destinationReplayEvidencePath: path.join(
      input.tmpDir,
      "destination-replay.json"
    ),
  };
}

function clientsFromContext(context: WriteNetworkContext): Clients {
  const account = context.walletClient.account;
  if (account === undefined) {
    throw new Error(
      `Hardhat network ${context.networkName} has no signer account`
    );
  }
  return {
    provider: context.connection.provider,
    publicClient: context.publicClient,
    walletClient: context.walletClient,
    account,
  };
}

async function runDirection(
  state: LocalE2ERunState,
  source: ChainDeployment,
  destination: ChainDeployment,
  amountLD: bigint,
  options: RunDirectionOptions = {}
) {
  const direction = directionLabel(source, destination);
  logStep("direction started", {
    direction,
    src_eid: source.eid,
    dst_eid: destination.eid,
    amount_ld: amountLD,
  });
  const sourceClients = clientsFor(state, source);
  const destinationClients = clientsFor(state, destination);
  const balanceBefore = await balanceOf(
    destinationClients.publicClient,
    destination.oft,
    state.deployerAddress
  );
  logStep("destination balance before send", {
    direction,
    recipient: state.deployerAddress,
    balance: balanceBefore,
  });
  const sendParam = buildCanarySendParam({
    dstEid: destination.eid,
    recipient: state.deployerAddress,
    amountLD,
    minAmountLD: amountLD,
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
    lz_receive_gas: state.deployment.parameters.lzReceiveGas,
  });
  const hash = await sourceClients.walletClient.writeContract({
    address: source.oft,
    abi: oftArtifact.abi,
    functionName: "send",
    args: [sendParam, { nativeFee, lzTokenFee: 0n }, state.deployerAddress],
    account: sourceClients.account,
    chain: sourceClients.walletClient.chain,
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
  await withdrawSourceWorkerFees(sourceClients, source, feeClaims);
  const [packet] = requirePacketCount(
    packetsFromSourceReceipt({
      receipt: sourceReceipt,
      endpoint: source.endpoint,
      endpointAbi: endpointArtifact.abi,
    }),
    1,
    "source receipt"
  );
  logStep("packet extracted", {
    direction,
    guid: packet.guid,
    nonce: packet.nonce,
    payload_hash: packet.payloadHash,
  });

  if (options.exerciseRBF) {
    await submitSecondaryVerificationAndExerciseRBF(
      state,
      destinationClients,
      destination,
      packet
    );
  } else {
    await submitSecondaryVerification(
      state,
      destinationClients,
      destination,
      packet
    );
  }

  await waitForDelivery(
    state,
    sourceClients,
    destinationClients,
    source,
    destination,
    packet,
    balanceBefore + amountLD
  );
  logStep("direction completed", {
    direction,
    min_balance: balanceBefore + amountLD,
  });
}

async function runMultiSendIndexerScenario(
  state: LocalE2ERunState,
  source: ChainDeployment,
  destination: ChainDeployment,
  amountsLD: readonly bigint[]
) {
  const direction = directionLabel(source, destination);
  logStep("multi-send indexer scenario started", {
    direction,
    src_eid: source.eid,
    dst_eid: destination.eid,
    amounts_ld: amountsLD,
  });
  const sourceClients = clientsFor(state, source);
  const destinationClients = clientsFor(state, destination);
  const balanceBefore = await balanceOf(
    destinationClients.publicClient,
    destination.oft,
    state.deployerAddress
  );
  const sendParams = amountsLD.map((amountLD) =>
    buildCanarySendParam({
      dstEid: destination.eid,
      recipient: state.deployerAddress,
      amountLD,
      minAmountLD: amountLD,
    })
  );
  const quote = multiSendQuoteFromReturn(
    await sourceClients.publicClient.readContract({
      address: source.oft,
      abi: oftArtifact.abi,
      functionName: "quoteMultiSend",
      args: [sendParams, false],
    })
  );
  logStep("quoted TestOFT multiSend", {
    direction,
    total_native_fee: quote.totalFee.nativeFee,
    per_send_native_fees: quote.fees.map((fee) => fee.nativeFee),
  });
  const hash = await sourceClients.walletClient.writeContract({
    address: source.oft,
    abi: oftArtifact.abi,
    functionName: "multiSend",
    args: [sendParams, false, state.deployerAddress],
    account: sourceClients.account,
    chain: sourceClients.walletClient.chain,
    value: quote.totalFee.nativeFee,
  });
  logStep("TestOFT multiSend submitted", { direction, tx: hash });
  const sourceReceipt =
    await sourceClients.publicClient.waitForTransactionReceipt({ hash });
  if (sourceReceipt.status !== "success") {
    throw new Error(`${source.name} TestOFT multiSend ${hash} failed`);
  }
  const packets = requirePacketCount(
    packetsFromSourceReceipt({
      receipt: sourceReceipt,
      endpoint: source.endpoint,
      endpointAbi: endpointArtifact.abi,
    }),
    amountsLD.length,
    "multi-send source receipt"
  );
  logStep("TestOFT multiSend confirmed", {
    direction,
    tx: hash,
    block: sourceReceipt.blockNumber,
    gas_used: sourceReceipt.gasUsed,
    packet_guids: packets.map((packet) => packet.guid),
  });

  const executorFee = sourceExecutorFeeTotal({
    sourceName: source.name,
    logs: sourceReceipt.logs,
    sendLib: source.sendUln,
    sendLibAbi: sendUlnArtifact.abi,
    openExecutor: source.openExecutor,
  });
  const feeClaims = sourceWorkerFeeClaims({
    sourceName: source.name,
    logs: sourceReceipt.logs,
    sendLib: source.sendUln,
    sendLibAbi: sendUlnArtifact.abi,
    openExecutor: source.openExecutor,
    primaryOpenDVN: source.primaryOpenDVN,
    secondaryOpenDVN: source.secondaryOpenDVN,
    executorFee,
  });
  logStep("multi-send source worker fee claims decoded", {
    direction,
    claims: feeClaims.map((claim) => ({
      role: claim.role,
      worker: claim.worker,
      amount: claim.amount.toString(),
    })),
  });
  await withdrawSourceWorkerFees(sourceClients, source, feeClaims);

  const evidence = multiSendIndexerEvidence({
    srcEid: source.eid,
    dstEid: destination.eid,
    packets,
  });
  await writeFile(
    state.multiSendIndexerEvidencePath,
    `${jsonStringify(evidence)}\n`
  );
  logStep("multi-send indexer evidence written", {
    path: state.multiSendIndexerEvidencePath,
    tx: evidence.sourceTxHash,
    packets: evidence.expectedPackets.length,
  });

  for (const packet of packets) {
    await submitSecondaryVerification(
      state,
      destinationClients,
      destination,
      packet
    );
  }
  let minBalance = balanceBefore;
  const replayObservations: DestinationReplayObservation[] = [];
  for (let index = 0; index < packets.length; index++) {
    const amountLD = amountsLD[index];
    const packet = packets[index];
    if (amountLD === undefined || packet === undefined) {
      throw new Error("multi-send amount or packet missing");
    }
    minBalance += amountLD;
    await waitForDelivery(
      state,
      sourceClients,
      destinationClients,
      source,
      destination,
      packet,
      minBalance
    );
    replayObservations.push(
      await destinationReplayObservation(
        destinationClients.publicClient,
        source,
        destination,
        packet
      )
    );
  }
  const replayEvidence = destinationReplayEvidence({
    srcEid: source.eid,
    dstEid: destination.eid,
    packets,
    observations: replayObservations,
  });
  await writeFile(
    state.destinationReplayEvidencePath,
    `${jsonStringify(replayEvidence)}\n`
  );
  logStep("destination replay evidence written", {
    path: state.destinationReplayEvidencePath,
    tx: replayEvidence.sourceTxHash,
    packets: replayEvidence.expectedPackets.length,
  });
  logStep("multi-send indexer scenario completed", {
    direction,
    min_balance: minBalance,
  });
}

async function submitSecondaryVerification(
  state: LocalE2ERunState,
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails
) {
  const hash = await sendSecondaryVerification(
    state,
    clients,
    destination,
    packet
  );
  const receipt = await clients.publicClient.waitForTransactionReceipt({
    hash,
  });
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

async function sendSecondaryVerification(
  state: LocalE2ERunState,
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<Hex> {
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
      BigInt(state.deployment.parameters.confirmations),
    ],
    account: clients.account,
    chain: clients.walletClient.chain,
  });
  logStep("secondary OpenDVN verification submitted", {
    chain: destination.name,
    tx: hash,
  });
  return hash;
}

async function submitSecondaryVerificationAndExerciseRBF(
  state: LocalE2ERunState,
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails
) {
  logStep("rbf exercise started", {
    chain: destination.name,
    receive_uln: destination.receiveUln,
    executor_signer: destination.executorSigner,
    payload_hash: packet.payloadHash,
  });
  await setAutomine(clients, false);
  try {
    const secondaryHash = await sendSecondaryVerification(
      state,
      clients,
      destination,
      packet
    );
    await waitForSecondaryAndPrimaryVerifications(
      clients,
      destination,
      packet,
      secondaryHash
    );
    const original = await waitForPendingCommitVerification(
      clients,
      destination,
      packet
    );
    const replacement = await waitForPendingCommitVerificationReplacement(
      clients,
      destination,
      packet,
      original
    );
    assertReplacementBumped(original, replacement, destination.name);
    logStep("rbf replacement observed", {
      chain: destination.name,
      original_tx: original.hash,
      replacement_tx: replacement.hash,
      nonce: replacement.nonce,
      original_max_fee_per_gas: original.maxFeePerGas,
      replacement_max_fee_per_gas: replacement.maxFeePerGas,
      original_max_priority_fee_per_gas: original.maxPriorityFeePerGas,
      replacement_max_priority_fee_per_gas: replacement.maxPriorityFeePerGas,
    });
  } finally {
    await setAutomine(clients, true);
  }
  await mine(clients);
}

async function waitForSecondaryAndPrimaryVerifications(
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails,
  secondaryHash: Hex
) {
  const started = Date.now();
  let secondaryConfirmed = false;
  while (Date.now() - started < 60_000) {
    await mine(clients);
    if (!secondaryConfirmed) {
      let receipt: TransactionReceipt | undefined;
      try {
        receipt = await clients.publicClient.getTransactionReceipt({
          hash: secondaryHash,
        });
      } catch {
        // The next mined block may include the script-submitted secondary tx.
      }
      if (receipt !== undefined) {
        if (receipt.status !== "success") {
          throw new Error(
            `secondary OpenDVN verification ${secondaryHash} failed`
          );
        }
        secondaryConfirmed = true;
        logStep("secondary OpenDVN verification confirmed", {
          chain: destination.name,
          tx: secondaryHash,
          block: receipt.blockNumber,
          gas_used: receipt.gasUsed,
        });
      }
    }
    const verified = await verifiedDVNs(
      clients.publicClient,
      destination,
      packet
    );
    if (
      secondaryConfirmed &&
      verified.has(destination.primaryOpenDVN.toLowerCase()) &&
      verified.has(destination.secondaryOpenDVN.toLowerCase())
    ) {
      logStep("rbf prerequisite verifications mined", {
        chain: destination.name,
        dvns: [...verified].sort(),
      });
      return;
    }
    await sleep(500);
  }
  throw new Error(
    `${destination.name} timed out waiting for primary and secondary verification before RBF exercise`
  );
}

async function waitForPendingCommitVerification(
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<PendingCommitVerificationTx> {
  const started = Date.now();
  while (Date.now() - started < 60_000) {
    const pending = await pendingCommitVerificationTxs(
      clients,
      destination,
      packet
    );
    const tx = pending[0];
    if (tx !== undefined) {
      logStep("rbf original commitVerification pending", {
        chain: destination.name,
        tx: tx.hash,
        nonce: tx.nonce,
        max_fee_per_gas: tx.maxFeePerGas,
        max_priority_fee_per_gas: tx.maxPriorityFeePerGas,
        gas_price: tx.gasPrice,
      });
      return tx;
    }
    await sleep(500);
  }
  throw new Error(
    `${destination.name} timed out waiting for pending worker commitVerification tx`
  );
}

async function waitForPendingCommitVerificationReplacement(
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails,
  original: PendingCommitVerificationTx
): Promise<PendingCommitVerificationTx> {
  const started = Date.now();
  while (Date.now() - started < 90_000) {
    const pending = await pendingCommitVerificationTxs(
      clients,
      destination,
      packet
    );
    const replacement = pending.find(
      (tx) => tx.nonce === original.nonce && tx.hash !== original.hash
    );
    if (replacement !== undefined) {
      return replacement;
    }
    await sleep(500);
  }
  throw new Error(
    `${destination.name} timed out waiting for pending worker commitVerification replacement`
  );
}

async function pendingCommitVerificationTxs(
  clients: Clients,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<PendingCommitVerificationTx[]> {
  const block = (await clients.provider.request({
    method: "eth_getBlockByNumber",
    params: ["pending", true],
  })) as PendingRPCBlock | null;
  const calldata = commitVerificationCalldata(packet).toLowerCase();
  return (block?.transactions ?? [])
    .filter((tx) => {
      return (
        tx.to !== null &&
        isAddressEqual(tx.from, destination.executorSigner) &&
        isAddressEqual(tx.to, destination.receiveUln) &&
        tx.input.toLowerCase() === calldata
      );
    })
    .map((tx) => ({
      hash: tx.hash,
      nonce: BigInt(tx.nonce),
      gasPrice: optionalHexBigInt(tx.gasPrice),
      maxFeePerGas: optionalHexBigInt(tx.maxFeePerGas),
      maxPriorityFeePerGas: optionalHexBigInt(tx.maxPriorityFeePerGas),
    }));
}

function commitVerificationCalldata(packet: PacketDetails): Hex {
  return encodeFunctionData({
    abi: receiveUlnArtifact.abi,
    functionName: "commitVerification",
    args: [packet.packetHeader, packet.payloadHash],
  });
}

function assertReplacementBumped(
  original: PendingCommitVerificationTx,
  replacement: PendingCommitVerificationTx,
  chainName: string
) {
  if (replacement.nonce !== original.nonce) {
    throw new Error(
      `${chainName} replacement nonce ${replacement.nonce} does not match original nonce ${original.nonce}`
    );
  }
  if (replacement.hash === original.hash) {
    throw new Error(
      `${chainName} replacement hash matches original ${original.hash}`
    );
  }
  if (
    original.maxFeePerGas !== undefined &&
    original.maxPriorityFeePerGas !== undefined &&
    replacement.maxFeePerGas !== undefined &&
    replacement.maxPriorityFeePerGas !== undefined
  ) {
    if (replacement.maxFeePerGas <= original.maxFeePerGas) {
      throw new Error(
        `${chainName} replacement max fee ${replacement.maxFeePerGas} is not above original ${original.maxFeePerGas}`
      );
    }
    if (replacement.maxPriorityFeePerGas <= original.maxPriorityFeePerGas) {
      throw new Error(
        `${chainName} replacement priority fee ${replacement.maxPriorityFeePerGas} is not above original ${original.maxPriorityFeePerGas}`
      );
    }
    return;
  }
  if (original.gasPrice !== undefined && replacement.gasPrice !== undefined) {
    if (replacement.gasPrice <= original.gasPrice) {
      throw new Error(
        `${chainName} replacement gas price ${replacement.gasPrice} is not above original ${original.gasPrice}`
      );
    }
    return;
  }
  throw new Error(
    `${chainName} replacement transaction is missing comparable fee fields`
  );
}

function optionalHexBigInt(value: Hex | undefined): bigint | undefined {
  return value === undefined ? undefined : BigInt(value);
}

async function waitForDelivery(
  state: LocalE2ERunState,
  sourceClients: Clients,
  destinationClients: Clients,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails,
  minBalance: bigint
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
    await mine(sourceClients);
    await mine(destinationClients);
    try {
      const verified = await verifiedDVNs(
        destinationClients.publicClient,
        destination,
        packet
      );
      if (
        !verified.has(destination.primaryOpenDVN.toLowerCase()) ||
        !verified.has(destination.secondaryOpenDVN.toLowerCase())
      ) {
        throw new Error(
          `${destination.name} missing PayloadVerified logs; observed ${[
            ...verified,
          ].join(",")}`
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
        packet
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
        packet
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
        `${destination.name} PacketDelivered`
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
        state.deployerAddress
      );
      assertCanaryRecipientBalance({
        recipient: state.deployerAddress,
        balance,
        minBalance,
      });
      logStep("recipient balance assertion passed", {
        direction,
        recipient: state.deployerAddress,
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
    `timed out waiting for ${source.name}->${
      destination.name
    } delivery: ${String(
      lastError instanceof Error ? lastError.message : lastError
    )}`
  );
}

async function destinationReplayObservation(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<DestinationReplayObservation> {
  const commitReceipt = await matchingPacketVerifiedReceipt(
    publicClient,
    source,
    destination,
    packet
  );
  const deliveryReceipt = await matchingPacketDeliveredReceipt(
    publicClient,
    source,
    destination,
    packet
  );
  const verifyReceipt = await matchingPayloadVerifiedReceipt(
    publicClient,
    destination,
    packet,
    destination.primaryOpenDVN
  );
  logStep("destination replay observation captured", {
    direction: directionLabel(source, destination),
    guid: packet.guid,
    commit_tx: commitReceipt.transactionHash,
    receive_tx: deliveryReceipt.transactionHash,
    verify_tx: verifyReceipt.transactionHash,
  });
  return {
    guid: packet.guid,
    commitTxHash: commitReceipt.transactionHash,
    receiveTxHash: deliveryReceipt.transactionHash,
    verifyTxHash: verifyReceipt.transactionHash,
  };
}

async function withdrawSourceWorkerFees(
  clients: Clients,
  source: ChainDeployment,
  claims: readonly SourceWorkerFeeClaim[]
) {
  for (const claim of claims) {
    await withdrawSourceWorkerFee(clients, source, claim);
  }
}

async function withdrawSourceWorkerFee(
  clients: Clients,
  source: ChainDeployment,
  claim: SourceWorkerFeeClaim
) {
  const ledgerBefore = await sendLibFeeBalance(
    clients.publicClient,
    source.sendUln,
    claim.worker
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
      `${source.name} ${claim.role} fee ledger ${ledgerBefore} does not match expected ${claim.amount}`
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
    account: clients.account,
    chain: clients.walletClient.chain,
  });
  logStep("source worker fee withdrawal submitted", {
    chain: source.name,
    role: claim.role,
    tx: hash,
  });
  const receipt = await clients.publicClient.waitForTransactionReceipt({
    hash,
  });
  if (receipt.status !== "success") {
    throw new Error(
      `${source.name} ${claim.role} fee withdrawal ${hash} failed`
    );
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
    claim.worker
  );
  if (ledgerAfter !== 0n) {
    throw new Error(
      `${source.name} ${claim.role} fee ledger ${ledgerAfter} after withdrawal, want 0`
    );
  }
  const recipientAfter = await clients.publicClient.getBalance({
    address: withdrawalRecipient,
  });
  if (recipientAfter - recipientBefore !== claim.amount) {
    throw new Error(
      `${source.name} withdrawal recipient balance delta ${
        recipientAfter - recipientBefore
      } does not match ${claim.amount}`
    );
  }
  const sendLibAfter = await clients.publicClient.getBalance({
    address: source.sendUln,
  });
  if (sendLibBefore - sendLibAfter !== claim.amount) {
    throw new Error(
      `${source.name} SendUln302 balance delta ${
        sendLibBefore - sendLibAfter
      } does not match ${claim.amount}`
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
  worker: Address
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
  logs: readonly Log[]
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
    throw new Error(
      `${source.name} ${claim.role} withdrawal missing worker event`
    );
  }
  if (!sawSendLibEvent) {
    throw new Error(
      `${source.name} ${claim.role} withdrawal missing SendUln302 event`
    );
  }
}

function workerArtifactForClaim(claim: SourceWorkerFeeClaim): Artifact {
  return claim.role === "open_executor"
    ? openExecutorArtifact
    : openDVNArtifact;
}

async function verifiedDVNs(
  publicClient: PublicClient,
  destination: ChainDeployment,
  packet: PacketDetails
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
  packet: PacketDetails
): Promise<Address> {
  const logs = await publicClient.getLogs({
    address: destination.primaryOpenDVN,
    fromBlock: 0n,
    toBlock: "latest",
  });
  const packetHeaderHash = keccak256(packet.packetHeader);
  for (const log of logs) {
    if (
      log.topics[0] !== eventTopic(openDVNArtifact, "DVNVerificationSubmitted")
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
          `${destination.name} primary OpenDVN verifier ${args.verifier} does not match configured DVN signer ${destination.dvnSigner}`
        );
      }
      return getAddress(args.verifier);
    }
  }
  throw new Error(
    `${destination.name} primary OpenDVN is missing DVNVerificationSubmitted for ${packet.payloadHash}`
  );
}

async function matchingPacketVerifiedReceipt(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<TransactionReceipt> {
  const logs = await matchingPacketVerifiedLogs(
    publicClient,
    source,
    destination,
    packet
  );
  const txHash = logs[0]?.transactionHash;
  if (txHash === undefined || txHash === null) {
    throw new Error(
      `${destination.name} PacketVerified log is missing transaction hash`
    );
  }
  return publicClient.getTransactionReceipt({ hash: txHash });
}

async function matchingPacketVerifiedLogs(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<Log[]> {
  const logs = await publicClient.getLogs({
    address: destination.endpoint,
    fromBlock: 0n,
    toBlock: "latest",
  });
  return logs.filter((log) => {
    if (log.topics[0] !== eventTopic(endpointArtifact, "PacketVerified")) {
      return false;
    }
    const decoded = decodeEventLog({
      abi: endpointArtifact.abi,
      eventName: "PacketVerified",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      origin: { srcEid: number; sender: Hex; nonce: bigint };
      receiver: Address;
      payloadHash: Hex;
    };
    return (
      args.origin.srcEid === source.eid &&
      args.origin.nonce === packet.nonce &&
      args.origin.sender.toLowerCase() ===
        addressToBytes32(source.oft).toLowerCase() &&
      isAddressEqual(args.receiver, destination.oft) &&
      args.payloadHash.toLowerCase() === packet.payloadHash.toLowerCase()
    );
  });
}

async function matchingPacketDeliveredReceipt(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails
): Promise<TransactionReceipt> {
  const logs = await matchingPacketDeliveredLogs(
    publicClient,
    source,
    destination,
    packet
  );
  const txHash = logs[0]?.transactionHash;
  if (txHash === undefined || txHash === null) {
    throw new Error(
      `${destination.name} PacketDelivered log is missing transaction hash`
    );
  }
  return publicClient.getTransactionReceipt({ hash: txHash });
}

async function matchingPacketDeliveredLogs(
  publicClient: PublicClient,
  source: ChainDeployment,
  destination: ChainDeployment,
  packet: PacketDetails
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

async function matchingPayloadVerifiedReceipt(
  publicClient: PublicClient,
  destination: ChainDeployment,
  packet: PacketDetails,
  dvn: Address
): Promise<TransactionReceipt> {
  const logs = await matchingPayloadVerifiedLogs(
    publicClient,
    destination,
    packet,
    dvn
  );
  const txHash = logs[0]?.transactionHash;
  if (txHash === undefined || txHash === null) {
    throw new Error(
      `${destination.name} PayloadVerified log is missing transaction hash`
    );
  }
  return publicClient.getTransactionReceipt({ hash: txHash });
}

async function matchingPayloadVerifiedLogs(
  publicClient: PublicClient,
  destination: ChainDeployment,
  packet: PacketDetails,
  dvn: Address
): Promise<Log[]> {
  const logs = await publicClient.getLogs({
    address: destination.receiveUln,
    fromBlock: 0n,
    toBlock: "latest",
  });
  return logs.filter((log) => {
    if (log.topics[0] !== eventTopic(receiveUlnArtifact, "PayloadVerified")) {
      return false;
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
    return (
      isAddressEqual(args.dvn, dvn) &&
      args.header.toLowerCase() === packet.packetHeader.toLowerCase() &&
      args.proofHash.toLowerCase() === packet.payloadHash.toLowerCase()
    );
  });
}

async function assertTransactionFrom(
  publicClient: PublicClient,
  txHash: Hex,
  expected: Address,
  label: string
): Promise<void> {
  const tx = await publicClient.getTransaction({ hash: txHash });
  if (!isAddressEqual(tx.from, expected)) {
    throw new Error(
      `${label} transaction sender ${tx.from} does not match expected signer ${expected}`
    );
  }
}

async function balanceOf(
  publicClient: PublicClient,
  token: Address,
  account: Address
): Promise<bigint> {
  return (await publicClient.readContract({
    address: token,
    abi: oftArtifact.abi,
    functionName: "balanceOf",
    args: [account],
  })) as bigint;
}

function clientsFor(state: LocalE2ERunState, chain: ChainDeployment): Clients {
  const clients = state.clients[chain.key];
  if (chain !== state.deployment.chains[chain.key]) {
    throw new Error(`chain ${chain.key} is not the loaded deployment chain`);
  }
  return clients;
}

async function waitForWorkerReady(url: string, fetchImpl: typeof fetch) {
  const started = Date.now();
  logStep("waiting for worker readiness", { url });
  while (Date.now() - started < 60_000) {
    try {
      const response = await fetchImpl(url);
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

async function mine(clients: Clients) {
  await clients.provider.request({
    method: "anvil_mine",
    params: ["0x1"],
  });
}

async function setAutomine(clients: Clients, enabled: boolean) {
  await clients.provider.request({
    method: "evm_setAutomine",
    params: [enabled],
  });
  logStep("anvil automine set", { enabled });
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

type MessagingFee = {
  nativeFee: bigint;
  lzTokenFee: bigint;
};

function multiSendQuoteFromReturn(value: unknown): {
  totalFee: MessagingFee;
  fees: MessagingFee[];
} {
  const record = value as {
    totalFee?: unknown;
    fees?: unknown;
    0?: unknown;
    1?: unknown;
  };
  const totalFee = messagingFeeFromUnknown(
    Array.isArray(value) ? value[0] : record.totalFee ?? record[0]
  );
  const feesValue = Array.isArray(value) ? value[1] : record.fees ?? record[1];
  if (!Array.isArray(feesValue)) {
    throw new Error(
      `unexpected quoteMultiSend fees return: ${jsonStringify(value)}`
    );
  }
  return {
    totalFee,
    fees: feesValue.map((fee) => messagingFeeFromUnknown(fee)),
  };
}

function messagingFeeFromUnknown(value: unknown): MessagingFee {
  if (Array.isArray(value)) {
    return { nativeFee: BigInt(value[0]), lzTokenFee: BigInt(value[1]) };
  }
  const record = value as {
    nativeFee?: bigint;
    lzTokenFee?: bigint;
    0?: bigint;
    1?: bigint;
  };
  const nativeFee = record.nativeFee ?? record[0];
  const lzTokenFee = record.lzTokenFee ?? record[1];
  if (nativeFee === undefined || lzTokenFee === undefined) {
    throw new Error(`unexpected MessagingFee return: ${jsonStringify(value)}`);
  }
  return { nativeFee, lzTokenFee };
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

function mutableTopics(topics: readonly Hex[]): [Hex, ...Hex[]] {
  if (topics.length === 0) {
    throw new Error("log is missing topics");
  }
  return [...topics] as [Hex, ...Hex[]];
}

function directionLabel(
  source: ChainDeployment,
  destination: ChainDeployment
): string {
  return `${source.name}->${destination.name}`;
}

function logStep(message: string, fields: Record<string, unknown> = {}) {
  const suffix = Object.entries(fields)
    .map(([key, value]) => `${key}=${formatLogValue(value)}`)
    .join(" ");
  console.error(
    `[e2e-local-run] ${message}${suffix === "" ? "" : ` ${suffix}`}`
  );
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
