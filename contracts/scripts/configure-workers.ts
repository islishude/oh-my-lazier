import type { Address } from "viem";
import { loadABIArtifact, waitForTx, type ChainClients } from "./lib.js";
import { shouldSetPriceFeed } from "./worker-price-feed.js";

export type ConfigureWorkersPriceSnapshotInput = {
  dstGasPriceInSrcToken: bigint;
  dstDataFeePerByteInSrcToken: bigint;
  updatedAt?: bigint;
  staleAfter: bigint;
};

export type ConfigureWorkersFeeModelInput = {
  baseFee: bigint;
  dstGasOverhead: bigint;
  dataSizeOverheadBytes: bigint;
  marginBps: bigint;
};

export type ConfigureWorkersRateLimitInput = {
  capacity: bigint;
  refillPerSecond: bigint;
};

export type ConfigureWorkersInput = {
  testOFT?: Address;
  openExecutor: Address;
  openDVN: Address;
  priceFeed: Address;
  remoteEid: number;
  sendLib: Address;
  srcOApp?: Address;
  maxMessageSize: bigint;
  minLzReceiveGas: bigint;
  maxLzReceiveGas: bigint;
  priceSnapshot: ConfigureWorkersPriceSnapshotInput;
  executorFeeModel: ConfigureWorkersFeeModelInput;
  dvnFeeModel: ConfigureWorkersFeeModelInput;
  rateLimit?: ConfigureWorkersRateLimitInput;
};

export type ConfigureWorkersResult = {
  chainId: number;
  sender: Address;
  testOFT?: Address;
  openExecutor: Address;
  openDVN: Address;
  priceFeed: Address;
  remoteEid: number;
  sendLib: Address;
  srcOApp: Address;
};

export type ConfigureWorkersOptions = {
  now?: bigint;
};

export type ConfigureWorkersPlan = {
  srcOApp: Address;
  pathwayConfig: {
    enabled: true;
    maxMessageSize: bigint;
    minLzReceiveGas: bigint;
    maxLzReceiveGas: bigint;
  };
  priceSnapshot: Required<ConfigureWorkersPriceSnapshotInput>;
  actions: string[];
};

const uint64Max = (1n << 64n) - 1n;
const uint128Max = (1n << 128n) - 1n;
const maxStaleAfter = 86_400n;
const bpsDenominator = 10_000n;

/** Validate every static constraint before any worker transaction is sent. */
export function buildConfigureWorkersPlan(
  input: ConfigureWorkersInput,
  options: ConfigureWorkersOptions = {}
): ConfigureWorkersPlan {
  const srcOApp = input.srcOApp ?? input.testOFT;
  if (srcOApp === undefined) {
    throw new Error("input.srcOApp or input.testOFT is required");
  }
  if (input.rateLimit !== undefined && input.testOFT === undefined) {
    throw new Error("input.testOFT is required for TestOFT rate-limit changes");
  }
  assertMaximum(input.minLzReceiveGas, uint128Max, "input.minLzReceiveGas");
  assertMaximum(input.maxLzReceiveGas, uint128Max, "input.maxLzReceiveGas");
  if (input.minLzReceiveGas > input.maxLzReceiveGas) {
    throw new Error(
      "input.minLzReceiveGas must not exceed input.maxLzReceiveGas"
    );
  }

  const now = options.now ?? BigInt(Math.floor(Date.now() / 1000));
  const updatedAt = input.priceSnapshot.updatedAt ?? now;
  assertMaximum(updatedAt, uint64Max, "input.priceSnapshot.updatedAt");
  assertMaximum(
    input.priceSnapshot.staleAfter,
    uint64Max,
    "input.priceSnapshot.staleAfter"
  );
  if (input.priceSnapshot.dstGasPriceInSrcToken === 0n) {
    throw new Error(
      "input.priceSnapshot.dstGasPriceInSrcToken must be greater than zero"
    );
  }
  if (updatedAt === 0n || updatedAt > now) {
    throw new Error(
      "input.priceSnapshot.updatedAt must be positive and not in the future"
    );
  }
  if (
    input.priceSnapshot.staleAfter === 0n ||
    input.priceSnapshot.staleAfter > maxStaleAfter
  ) {
    throw new Error(
      "input.priceSnapshot.staleAfter must be between 1 and 86400"
    );
  }
  validateFeeModel(input.executorFeeModel, "input.executorFeeModel");
  validateFeeModel(input.dvnFeeModel, "input.dvnFeeModel");

  return {
    srcOApp,
    pathwayConfig: {
      enabled: true,
      maxMessageSize: input.maxMessageSize,
      minLzReceiveGas: input.minLzReceiveGas,
      maxLzReceiveGas: input.maxLzReceiveGas,
    },
    priceSnapshot: {
      ...input.priceSnapshot,
      updatedAt,
    },
    actions: [
      ...(input.rateLimit === undefined
        ? []
        : ["TestOFT.setOutboundRateLimit"]),
      "OpenPriceFeed.setPriceSnapshot",
      "OpenExecutor/OpenDVN.setPriceFeed when changed",
      "OpenExecutor/OpenDVN.setAllowedSendLib",
      "OpenExecutor/OpenDVN.setPathwayConfig",
      "OpenExecutor/OpenDVN.setFeeModel",
    ],
  };
}

/** Configure the repeatable worker and optional TestOFT pathway settings. */
export async function configureWorkers(
  input: ConfigureWorkersInput,
  clients: ChainClients,
  options: ConfigureWorkersOptions = {}
): Promise<ConfigureWorkersResult> {
  const plan = buildConfigureWorkersPlan(input, options);

  const testOFTArtifact = loadABIArtifact(
    "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json"
  );
  const openExecutorArtifact = loadABIArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json"
  );
  const openDVNArtifact = loadABIArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json"
  );
  const openPriceFeedArtifact = loadABIArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenPriceFeed.sol/OpenPriceFeed.json"
  );

  if (input.rateLimit !== undefined && input.testOFT !== undefined) {
    await waitForTx(
      clients.publicClient,
      "TestOFT.setOutboundRateLimit",
      await clients.walletClient.writeContract({
        address: input.testOFT,
        abi: testOFTArtifact.abi,
        functionName: "setOutboundRateLimit",
        args: [input.remoteEid, input.rateLimit],
        account: clients.account,
        chain: clients.walletClient.chain,
      })
    );
  }

  await waitForTx(
    clients.publicClient,
    "OpenPriceFeed.setPriceSnapshot",
    await clients.walletClient.writeContract({
      address: input.priceFeed,
      abi: openPriceFeedArtifact.abi,
      functionName: "setPriceSnapshot",
      args: [[{ dstEid: input.remoteEid, snapshot: plan.priceSnapshot }]],
      account: clients.account,
      chain: clients.walletClient.chain,
    })
  );

  for (const [label, address, abi, feeModel] of [
    [
      "OpenExecutor",
      input.openExecutor,
      openExecutorArtifact.abi,
      input.executorFeeModel,
    ],
    ["OpenDVN", input.openDVN, openDVNArtifact.abi, input.dvnFeeModel],
  ] as const) {
    const currentPriceFeed = (await clients.publicClient.readContract({
      address,
      abi,
      functionName: "priceFeed",
    })) as string;
    if (shouldSetPriceFeed(currentPriceFeed, input.priceFeed)) {
      await waitForTx(
        clients.publicClient,
        `${label}.setPriceFeed`,
        await clients.walletClient.writeContract({
          address,
          abi,
          functionName: "setPriceFeed",
          args: [input.priceFeed],
          account: clients.account,
          chain: clients.walletClient.chain,
        })
      );
    }

    await waitForTx(
      clients.publicClient,
      `${label}.setAllowedSendLib`,
      await clients.walletClient.writeContract({
        address,
        abi,
        functionName: "setAllowedSendLib",
        args: [input.sendLib, true],
        account: clients.account,
        chain: clients.walletClient.chain,
      })
    );

    await waitForTx(
      clients.publicClient,
      `${label}.setPathwayConfig`,
      await clients.walletClient.writeContract({
        address,
        abi,
        functionName: "setPathwayConfig",
        args: [input.remoteEid, plan.srcOApp, plan.pathwayConfig],
        account: clients.account,
        chain: clients.walletClient.chain,
      })
    );

    await waitForTx(
      clients.publicClient,
      `${label}.setFeeModel`,
      await clients.walletClient.writeContract({
        address,
        abi,
        functionName: "setFeeModel",
        args: [input.remoteEid, feeModel],
        account: clients.account,
        chain: clients.walletClient.chain,
      })
    );
  }

  return {
    chainId: Number(await clients.publicClient.getChainId()),
    sender: clients.account.address,
    ...(input.testOFT === undefined ? {} : { testOFT: input.testOFT }),
    openExecutor: input.openExecutor,
    openDVN: input.openDVN,
    priceFeed: input.priceFeed,
    remoteEid: input.remoteEid,
    sendLib: input.sendLib,
    srcOApp: plan.srcOApp,
  };
}

function validateFeeModel(
  model: ConfigureWorkersFeeModelInput,
  label: string
): void {
  assertMaximum(model.dstGasOverhead, uint64Max, `${label}.dstGasOverhead`);
  assertMaximum(
    model.dataSizeOverheadBytes,
    uint64Max,
    `${label}.dataSizeOverheadBytes`
  );
  if (model.marginBps > bpsDenominator) {
    throw new Error(`${label}.marginBps must not exceed 10000`);
  }
}

function assertMaximum(value: bigint, maximum: bigint, label: string): void {
  if (value > maximum) {
    throw new Error(`${label} exceeds ${maximum.toString()}`);
  }
}
