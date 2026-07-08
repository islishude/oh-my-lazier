import {
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  optionalAddress,
  optionalBigInt,
  optionalUint64,
} from "./lib.js";
import type {
  PriceSnapshotInput,
  WorkerFeeModelInput,
} from "./oft-pathway-ignition.js";
import {
  buildOAppEndpointConfigParameters,
  buildOpenWorkersPathwayConfigParameters,
} from "./oft-pathway-ignition.js";

const maxMessageSizeValue = envBigInt("EXECUTOR_MAX_MESSAGE_SIZE");
if (maxMessageSizeValue > 0xffffffffn) {
  throw new Error("EXECUTOR_MAX_MESSAGE_SIZE exceeds uint32");
}

const delegate = optionalAddress("DELEGATE");
const dvnVerifier = optionalAddress("DVN_VERIFIER");
const enforcedLzReceiveGas = envBigInt("ENFORCED_LZ_RECEIVE_GAS");
const minLzReceiveGas =
  optionalBigInt("MIN_LZ_RECEIVE_GAS") ?? enforcedLzReceiveGas;
const priceUpdatedAt = BigInt(Math.floor(Date.now() / 1000));

const input = {
  oapp: envAddress("OAPP"),
  endpoint: envAddress("ENDPOINT"),
  delegate,
  remoteEid: envUint32("REMOTE_EID"),
  remoteOApp: envAddress("REMOTE_OAPP"),
  sendUln: envAddress("SEND_ULN"),
  receiveUln: envAddress("RECEIVE_ULN"),
  openExecutor: envAddress("OPEN_EXECUTOR"),
  openDVN: envAddress("OPEN_DVN"),
  priceFeed: envAddress("PRICE_FEED"),
  bootstrapPriceSubmitter: envAddress("BOOTSTRAP_PRICE_SUBMITTER"),
  layerZeroLabsDVN: envAddress("LAYERZERO_LABS_DVN"),
  confirmations: envBigInt("CONFIRMATIONS"),
  maxMessageSize: Number(maxMessageSizeValue),
  minLzReceiveGas,
  maxLzReceiveGas: envBigInt("MAX_LZ_RECEIVE_GAS"),
  priceSnapshot: priceSnapshot(priceUpdatedAt),
  executorFeeModel: workerFeeModel("EXECUTOR"),
  dvnFeeModel: workerFeeModel("DVN"),
  dvnVerifier,
  enforcedLzReceiveGas,
};

const parameters = {
  ...buildOAppEndpointConfigParameters(input),
  ...buildOpenWorkersPathwayConfigParameters(input),
};

console.log(jsonStringify(parameters));

function priceSnapshot(defaultUpdatedAt: bigint): PriceSnapshotInput {
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

function workerFeeModel(prefix: "EXECUTOR" | "DVN"): WorkerFeeModelInput {
  return {
    baseFee: envBigInt(`${prefix}_FEE_FIXED_FEE_WEI`),
    dstGasOverhead: envBigInt(`${prefix}_FEE_DST_GAS_OVERHEAD`),
    dataSizeOverheadBytes: envBigInt(`${prefix}_FEE_DATA_SIZE_OVERHEAD_BYTES`),
    marginBps: envBigInt(`${prefix}_FEE_MARGIN_BPS`),
  };
}
