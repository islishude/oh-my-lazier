import {
  assertConfiguredChain,
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
import { shouldSetPriceFeed } from "./worker-price-feed.js";

const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);
const openExecutorArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
);
const openDVNArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
);
const openPriceFeedArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenPriceFeed.sol/OpenPriceFeed.json",
);

const { account, publicClient, walletClient } = createClients();
await assertConfiguredChain(publicClient);

const testOFT = optionalAddress("TEST_OFT");
const openExecutor = envAddress("OPEN_EXECUTOR");
const openDVN = envAddress("OPEN_DVN");
const priceFeed = envAddress("PRICE_FEED");
const remoteEid = envUint32("REMOTE_EID");
const sendLib = envAddress("SEND_LIB");
const srcOApp = optionalAddress("SRC_OAPP") ?? testOFT;
if (srcOApp === undefined) {
  throw new Error("SRC_OAPP or TEST_OFT is required");
}

const pathwayConfig = {
  enabled: true,
  maxMessageSize: envBigInt("MAX_MESSAGE_SIZE"),
  minLzReceiveGas: envBigInt("MIN_LZ_RECEIVE_GAS"),
  maxLzReceiveGas: envBigInt("MAX_LZ_RECEIVE_GAS"),
};

const now = BigInt(Math.floor(Date.now() / 1000));
const sharedPriceSnapshot = priceSnapshot(now);
const executorFeeModel = workerFeeModel("EXECUTOR");
const dvnFeeModel = workerFeeModel("DVN");

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
  if (testOFT === undefined) {
    throw new Error("TEST_OFT is required for TestOFT rate-limit changes");
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
      chain: walletClient.chain,
    }),
  );
}

await waitForTx(
  publicClient,
  "OpenPriceFeed.setPriceSnapshot",
  await walletClient.writeContract({
    address: priceFeed,
    abi: openPriceFeedArtifact.abi,
    functionName: "setPriceSnapshot",
    args: [[{ dstEid: remoteEid, snapshot: sharedPriceSnapshot }]],
    account,
    chain: walletClient.chain,
  }),
);

for (const [label, address, abi] of [
  ["OpenExecutor", openExecutor, openExecutorArtifact.abi],
  ["OpenDVN", openDVN, openDVNArtifact.abi],
] as const) {
  const currentPriceFeed = (await publicClient.readContract({
    address,
    abi,
    functionName: "priceFeed",
  })) as string;
  if (shouldSetPriceFeed(currentPriceFeed, priceFeed)) {
    await waitForTx(
      publicClient,
      `${label}.setPriceFeed`,
      await walletClient.writeContract({
        address,
        abi,
        functionName: "setPriceFeed",
        args: [priceFeed],
        account,
        chain: walletClient.chain,
      }),
    );
  }

  await waitForTx(
    publicClient,
    `${label}.setAllowedSendLib`,
    await walletClient.writeContract({
      address,
      abi,
      functionName: "setAllowedSendLib",
      args: [sendLib, true],
      account,
      chain: walletClient.chain,
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
      chain: walletClient.chain,
    }),
  );

  await waitForTx(
    publicClient,
    `${label}.setFeeModel`,
    await walletClient.writeContract({
      address,
      abi,
      functionName: "setFeeModel",
      args: [remoteEid, label === "OpenExecutor" ? executorFeeModel : dvnFeeModel],
      account,
      chain: walletClient.chain,
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
      priceFeed,
      remoteEid,
      sendLib,
      srcOApp,
    },
    null,
    2,
  ),
);

function priceSnapshot(defaultUpdatedAt: bigint) {
  return {
    dstGasPriceInSrcToken: envBigInt(
      "PRICE_SNAPSHOT_DST_GAS_PRICE_IN_SRC_TOKEN",
    ),
    dstDataFeePerByteInSrcToken: envBigInt(
      "PRICE_SNAPSHOT_DST_DATA_FEE_PER_BYTE_IN_SRC_TOKEN",
    ),
    updatedAt: optionalUint64("PRICE_SNAPSHOT_UPDATED_AT", defaultUpdatedAt),
    staleAfter: envBigInt("PRICE_SNAPSHOT_STALE_AFTER"),
  };
}

function workerFeeModel(prefix: "EXECUTOR" | "DVN") {
  return {
    baseFee: envBigInt(`${prefix}_FEE_FIXED_FEE_WEI`),
    dstGasOverhead: envBigInt(`${prefix}_FEE_DST_GAS_OVERHEAD`),
    dataSizeOverheadBytes: envBigInt(`${prefix}_FEE_DATA_SIZE_OVERHEAD_BYTES`),
    marginBps: envBigInt(`${prefix}_FEE_MARGIN_BPS`),
  };
}
