import {
  createClients,
  envAddress,
  envBigInt,
  envUint32,
  loadArtifact,
  optionalAddress,
  optionalBigInt,
  optionalUint64,
  waitForTx,
} from "./lib.js";

const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);
const openExecutorArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
);
const openDVNArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
);

const { account, publicClient, walletClient } = createClients();

const testOFT = envAddress("TEST_OFT");
const openExecutor = envAddress("OPEN_EXECUTOR");
const openDVN = envAddress("OPEN_DVN");
const remoteEid = envUint32("REMOTE_EID");
const sendLib = envAddress("SEND_LIB");
const srcOApp = optionalAddress("SRC_OAPP") ?? testOFT;

const pathwayConfig = {
  enabled: true,
  maxMessageSize: envBigInt("MAX_MESSAGE_SIZE"),
  minLzReceiveGas: envBigInt("MIN_LZ_RECEIVE_GAS"),
  maxLzReceiveGas: envBigInt("MAX_LZ_RECEIVE_GAS"),
};

const now = BigInt(Math.floor(Date.now() / 1000));
const executorPriceConfig = workerPriceConfig("EXECUTOR", now);
const dvnPriceConfig = workerPriceConfig("DVN", now);

const rateLimitCapacity = optionalBigInt("RATE_LIMIT_CAPACITY");
const rateLimitRefillPerSecond = optionalBigInt("RATE_LIMIT_REFILL_PER_SECOND");
if (rateLimitCapacity !== undefined || rateLimitRefillPerSecond !== undefined) {
  if (
    rateLimitCapacity === undefined ||
    rateLimitRefillPerSecond === undefined
  ) {
    throw new Error(
      "RATE_LIMIT_CAPACITY and RATE_LIMIT_REFILL_PER_SECOND must be set together",
    );
  }
  await waitForTx(
    publicClient,
    "TestOFT.setOutboundRateLimit",
    await walletClient.writeContract({
      address: testOFT,
      abi: testOFTArtifact.abi,
      functionName: "setOutboundRateLimit",
      args: [
        remoteEid,
        {
          capacity: rateLimitCapacity,
          refillPerSecond: rateLimitRefillPerSecond,
        },
      ],
      account,
      chain: null,
    }),
  );
}

for (const [label, address, abi] of [
  ["OpenExecutor", openExecutor, openExecutorArtifact.abi],
  ["OpenDVN", openDVN, openDVNArtifact.abi],
] as const) {
  await waitForTx(
    publicClient,
    `${label}.setAllowedSendLib`,
    await walletClient.writeContract({
      address,
      abi,
      functionName: "setAllowedSendLib",
      args: [sendLib, true],
      account,
      chain: null,
    }),
  );

  await waitForTx(
    publicClient,
    `${label}.setPathwayConfig`,
    await walletClient.writeContract({
      address,
      abi,
      functionName: "setPathwayConfig",
      args: [remoteEid, srcOApp, pathwayConfig],
      account,
      chain: null,
    }),
  );

  await waitForTx(
    publicClient,
    `${label}.setPriceConfig`,
    await walletClient.writeContract({
      address,
      abi,
      functionName: "setPriceConfig",
      args: [
        remoteEid,
        label === "OpenExecutor" ? executorPriceConfig : dvnPriceConfig,
      ],
      account,
      chain: null,
    }),
  );
}

console.log(
  JSON.stringify(
    {
      chainId: Number(await publicClient.getChainId()),
      sender: account.address,
      testOFT,
      openExecutor,
      openDVN,
      remoteEid,
      sendLib,
      srcOApp,
    },
    null,
    2,
  ),
);

function workerPriceConfig(prefix: "EXECUTOR" | "DVN", defaultUpdatedAt: bigint) {
  return {
    baseFee: envBigInt(`${prefix}_PRICE_BASE_FEE`),
    dstGasPriceInSrcToken: envBigInt(
      `${prefix}_PRICE_DST_GAS_PRICE_IN_SRC_TOKEN`,
    ),
    dstGasOverhead: envBigInt(`${prefix}_PRICE_DST_GAS_OVERHEAD`),
    marginBps: envBigInt(`${prefix}_PRICE_MARGIN_BPS`),
    updatedAt: optionalUint64(`${prefix}_PRICE_UPDATED_AT`, defaultUpdatedAt),
    staleAfter: envBigInt(`${prefix}_PRICE_STALE_AFTER`),
  };
}
