import {
  createPublicClientFromEnv,
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  loadArtifact,
  optionalBigInt,
} from "./lib.js";
import type { Abi, Address, PublicClient } from "viem";

export type PriceConfig = {
  baseFee: bigint;
  dstGasPriceInSrcToken: bigint;
  bufferBps: number;
  updatedAt: bigint;
  staleAfter: bigint;
};

export type WorkerPriceConfig = {
  label: "OpenExecutor" | "OpenDVN";
  address: Address;
  priceConfig: PriceConfig;
};

export type PriceConfigReport = {
  chainId: number;
  dstEid: number;
  checkedAt: bigint;
  maxAgeSeconds: bigint;
  expectedStaleAfter?: bigint;
  workers: WorkerPriceConfig[];
};

export async function readPriceConfigReport(input: {
  publicClient: PublicClient;
  dstEid: number;
  checkedAt: bigint;
  maxAgeSeconds: bigint;
  expectedStaleAfter?: bigint;
  openExecutor: Address;
  openDVN: Address;
  openExecutorAbi: Abi;
  openDVNAbi: Abi;
}): Promise<PriceConfigReport> {
  const [chainId, executorConfig, dvnConfig] = await Promise.all([
    input.publicClient.getChainId(),
    readPriceConfig(
      input.publicClient,
      input.openExecutor,
      input.openExecutorAbi,
      input.dstEid,
    ),
    readPriceConfig(
      input.publicClient,
      input.openDVN,
      input.openDVNAbi,
      input.dstEid,
    ),
  ]);
  return {
    chainId,
    dstEid: input.dstEid,
    checkedAt: input.checkedAt,
    maxAgeSeconds: input.maxAgeSeconds,
    expectedStaleAfter: input.expectedStaleAfter,
    workers: [
      {
        label: "OpenExecutor",
        address: input.openExecutor,
        priceConfig: executorConfig,
      },
      { label: "OpenDVN", address: input.openDVN, priceConfig: dvnConfig },
    ],
  };
}

export function validatePriceConfigReport(report: PriceConfigReport): string[] {
  const errors: string[] = [];
  for (const worker of report.workers) {
    const config = worker.priceConfig;
    if (config.dstGasPriceInSrcToken <= 0n) {
      errors.push(`${worker.label} dstGasPriceInSrcToken must be non-zero`);
    }
    if (config.updatedAt === 0n) {
      errors.push(`${worker.label} updatedAt is zero`);
    } else if (config.updatedAt > report.checkedAt) {
      errors.push(
        `${worker.label} updatedAt ${config.updatedAt} is in the future`,
      );
    } else if (report.checkedAt - config.updatedAt > report.maxAgeSeconds) {
      errors.push(
        `${worker.label} priceConfig age ${report.checkedAt - config.updatedAt}s exceeds ${report.maxAgeSeconds}s`,
      );
    }
    if (config.staleAfter === 0n) {
      errors.push(`${worker.label} staleAfter is zero`);
    }
    if (
      report.expectedStaleAfter !== undefined &&
      config.staleAfter !== report.expectedStaleAfter
    ) {
      errors.push(
        `${worker.label} staleAfter ${config.staleAfter} does not match expected ${report.expectedStaleAfter}`,
      );
    }
  }
  return errors;
}

async function readPriceConfig(
  publicClient: PublicClient,
  address: Address,
  abi: Abi,
  dstEid: number,
): Promise<PriceConfig> {
  return normalizePriceConfig(
    await publicClient.readContract({
      address,
      abi,
      functionName: "priceConfig",
      args: [dstEid],
    }),
  );
}

function normalizePriceConfig(value: unknown): PriceConfig {
  if (Array.isArray(value)) {
    return {
      baseFee: value[0] as bigint,
      dstGasPriceInSrcToken: value[1] as bigint,
      bufferBps: Number(value[2]),
      updatedAt: value[3] as bigint,
      staleAfter: value[4] as bigint,
    };
  }
  const config = value as {
    baseFee: bigint;
    dstGasPriceInSrcToken: bigint;
    bufferBps: number;
    updatedAt: bigint;
    staleAfter: bigint;
  };
  return {
    baseFee: config.baseFee,
    dstGasPriceInSrcToken: config.dstGasPriceInSrcToken,
    bufferBps: Number(config.bufferBps),
    updatedAt: config.updatedAt,
    staleAfter: config.staleAfter,
  };
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const openExecutorArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
  );
  const openDVNArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
  );
  const report = await readPriceConfigReport({
    publicClient: createPublicClientFromEnv(),
    dstEid: envUint32("DST_EID"),
    checkedAt: BigInt(Math.floor(Date.now() / 1000)),
    maxAgeSeconds: envBigInt("MAX_PRICE_AGE_SECONDS"),
    expectedStaleAfter: optionalBigInt("EXPECTED_STALE_AFTER"),
    openExecutor: envAddress("OPEN_EXECUTOR"),
    openDVN: envAddress("OPEN_DVN"),
    openExecutorAbi: openExecutorArtifact.abi,
    openDVNAbi: openDVNArtifact.abi,
  });
  const errors = validatePriceConfigReport(report);
  console.log(jsonStringify({ ok: errors.length === 0, ...report, errors }));
  if (errors.length > 0) {
    process.exitCode = 1;
  }
}
