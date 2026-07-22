import { jsonStringify, loadArtifact } from "./lib.js";
import type { Abi, Address, PublicClient } from "viem";

export type PriceSnapshot = {
  dstGasPriceInSrcToken: bigint;
  dstDataFeePerByteInSrcToken: bigint;
  updatedAt: bigint;
  staleAfter: bigint;
};

export type FeeModel = {
  baseFee: bigint;
  dstGasOverhead: bigint;
  dataSizeOverheadBytes: bigint;
  marginBps: number;
};

export type PriceFeedSnapshot = {
  address: Address;
  priceSnapshot: PriceSnapshot;
};

export type WorkerFeeModel = {
  label: "OpenExecutor" | "OpenDVN";
  address: Address;
  priceFeed: Address;
  feeModel: FeeModel;
};

export type PriceConfigReport = {
  chainId: number;
  dstEid: number;
  checkedAt: bigint;
  maxAgeSeconds: bigint;
  expectedStaleAfter?: bigint;
  priceFeed: PriceFeedSnapshot;
  workers: WorkerFeeModel[];
};

export async function readPriceConfigReport(input: {
  publicClient: PublicClient;
  dstEid: number;
  checkedAt: bigint;
  maxAgeSeconds: bigint;
  expectedStaleAfter?: bigint;
  priceFeed: Address;
  openExecutor: Address;
  openDVN: Address;
  priceFeedAbi: Abi;
  openExecutorAbi: Abi;
  openDVNAbi: Abi;
}): Promise<PriceConfigReport> {
  const [chainId, snapshot, executor, dvn] = await Promise.all([
    input.publicClient.getChainId(),
    readPriceSnapshot(
      input.publicClient,
      input.priceFeed,
      input.priceFeedAbi,
      input.dstEid
    ),
    readWorkerFeeModel(
      input.publicClient,
      "OpenExecutor",
      input.openExecutor,
      input.openExecutorAbi,
      input.dstEid
    ),
    readWorkerFeeModel(
      input.publicClient,
      "OpenDVN",
      input.openDVN,
      input.openDVNAbi,
      input.dstEid
    ),
  ]);
  return {
    chainId,
    dstEid: input.dstEid,
    checkedAt: input.checkedAt,
    maxAgeSeconds: input.maxAgeSeconds,
    expectedStaleAfter: input.expectedStaleAfter,
    priceFeed: {
      address: input.priceFeed,
      priceSnapshot: snapshot,
    },
    workers: [executor, dvn],
  };
}

export function validatePriceConfigReport(report: PriceConfigReport): string[] {
  const errors: string[] = [];
  const snapshot = report.priceFeed.priceSnapshot;
  if (snapshot.dstGasPriceInSrcToken <= 0n) {
    errors.push("priceFeed dstGasPriceInSrcToken must be non-zero");
  }
  if (snapshot.dstDataFeePerByteInSrcToken < 0n) {
    errors.push("priceFeed dstDataFeePerByteInSrcToken must be non-negative");
  }
  if (snapshot.updatedAt === 0n) {
    errors.push("priceFeed updatedAt is zero");
  } else if (snapshot.updatedAt > report.checkedAt) {
    errors.push(`priceFeed updatedAt ${snapshot.updatedAt} is in the future`);
  } else if (report.checkedAt - snapshot.updatedAt > report.maxAgeSeconds) {
    errors.push(
      `priceFeed priceSnapshot age ${
        report.checkedAt - snapshot.updatedAt
      }s exceeds ${report.maxAgeSeconds}s`
    );
  }
  if (snapshot.staleAfter === 0n) {
    errors.push("priceFeed staleAfter is zero");
  }
  if (
    report.expectedStaleAfter !== undefined &&
    snapshot.staleAfter !== report.expectedStaleAfter
  ) {
    errors.push(
      `priceFeed staleAfter ${snapshot.staleAfter} does not match expected ${report.expectedStaleAfter}`
    );
  }

  for (const worker of report.workers) {
    if (
      worker.priceFeed.toLowerCase() !== report.priceFeed.address.toLowerCase()
    ) {
      errors.push(
        `${worker.label} priceFeed ${worker.priceFeed} does not match expected ${report.priceFeed.address}`
      );
    }
    if (worker.feeModel.baseFee < 0n) {
      errors.push(`${worker.label} baseFee must be non-negative`);
    }
    if (worker.feeModel.dstGasOverhead < 0n) {
      errors.push(`${worker.label} dstGasOverhead must be non-negative`);
    }
    if (worker.feeModel.dataSizeOverheadBytes < 0n) {
      errors.push(`${worker.label} dataSizeOverheadBytes must be non-negative`);
    }
    if (worker.feeModel.marginBps > 10_000) {
      errors.push(`${worker.label} marginBps exceeds 10000`);
    }
  }
  return errors;
}

async function readPriceSnapshot(
  publicClient: PublicClient,
  address: Address,
  abi: Abi,
  dstEid: number
): Promise<PriceSnapshot> {
  return normalizePriceSnapshot(
    await publicClient.readContract({
      address,
      abi,
      functionName: "priceSnapshot",
      args: [dstEid],
    })
  );
}

async function readWorkerFeeModel(
  publicClient: PublicClient,
  label: "OpenExecutor" | "OpenDVN",
  address: Address,
  abi: Abi,
  dstEid: number
): Promise<WorkerFeeModel> {
  const [priceFeed, feeModel] = await Promise.all([
    publicClient.readContract({
      address,
      abi,
      functionName: "priceFeed",
    }) as Promise<Address>,
    publicClient.readContract({
      address,
      abi,
      functionName: "feeModel",
      args: [dstEid],
    }),
  ]);
  return {
    label,
    address,
    priceFeed,
    feeModel: normalizeFeeModel(feeModel),
  };
}

function normalizePriceSnapshot(value: unknown): PriceSnapshot {
  if (Array.isArray(value)) {
    return {
      dstGasPriceInSrcToken: value[0] as bigint,
      dstDataFeePerByteInSrcToken: value[1] as bigint,
      updatedAt: value[2] as bigint,
      staleAfter: value[3] as bigint,
    };
  }
  const snapshot = value as {
    dstGasPriceInSrcToken: bigint;
    dstDataFeePerByteInSrcToken: bigint;
    updatedAt: bigint;
    staleAfter: bigint;
  };
  return {
    dstGasPriceInSrcToken: snapshot.dstGasPriceInSrcToken,
    dstDataFeePerByteInSrcToken: snapshot.dstDataFeePerByteInSrcToken,
    updatedAt: snapshot.updatedAt,
    staleAfter: snapshot.staleAfter,
  };
}

function normalizeFeeModel(value: unknown): FeeModel {
  if (Array.isArray(value)) {
    return {
      baseFee: value[0] as bigint,
      dstGasOverhead: value[1] as bigint,
      dataSizeOverheadBytes: value[2] as bigint,
      marginBps: Number(value[3]),
    };
  }
  const model = value as {
    baseFee: bigint;
    dstGasOverhead: bigint;
    dataSizeOverheadBytes: bigint;
    marginBps: number;
  };
  return {
    baseFee: model.baseFee,
    dstGasOverhead: model.dstGasOverhead,
    dataSizeOverheadBytes: model.dataSizeOverheadBytes,
    marginBps: Number(model.marginBps),
  };
}

export type RunPriceConfigCheckInput = {
  dstEid: number;
  maxPriceAgeSeconds: bigint;
  expectedStaleAfter?: bigint;
  priceFeed: Address;
  openExecutor: Address;
  openDVN: Address;
};

export async function runPriceConfigCheck(
  input: RunPriceConfigCheckInput,
  publicClient: PublicClient
): Promise<void> {
  const priceFeedArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenPriceFeed.sol/OpenPriceFeed.json"
  );
  const openExecutorArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json"
  );
  const openDVNArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json"
  );
  const report = await readPriceConfigReport({
    publicClient,
    dstEid: input.dstEid,
    checkedAt: BigInt(Math.floor(Date.now() / 1000)),
    maxAgeSeconds: input.maxPriceAgeSeconds,
    expectedStaleAfter: input.expectedStaleAfter,
    priceFeed: input.priceFeed,
    openExecutor: input.openExecutor,
    openDVN: input.openDVN,
    priceFeedAbi: priceFeedArtifact.abi,
    openExecutorAbi: openExecutorArtifact.abi,
    openDVNAbi: openDVNArtifact.abi,
  });
  const errors = validatePriceConfigReport(report);
  console.log(jsonStringify({ ok: errors.length === 0, ...report, errors }));
  if (errors.length > 0) {
    throw new Error(`price config check failed with ${errors.length} error(s)`);
  }
}
