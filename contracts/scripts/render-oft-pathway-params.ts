import {
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  optionalAddress,
  optionalBigInt,
  optionalUint64,
} from "./lib.js";
import type { WorkerPriceConfigInput } from "./oft-pathway-ignition.js";
import { buildTestOFTPathwayConfigParameters } from "./oft-pathway-ignition.js";

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

const parameters = buildTestOFTPathwayConfigParameters({
  testOFT: envAddress("TEST_OFT"),
  endpoint: envAddress("ENDPOINT"),
  delegate,
  remoteEid: envUint32("REMOTE_EID"),
  remoteOFT: envAddress("REMOTE_OFT"),
  sendUln: envAddress("SEND_ULN"),
  receiveUln: envAddress("RECEIVE_ULN"),
  openExecutor: envAddress("OPEN_EXECUTOR"),
  openDVN: envAddress("OPEN_DVN"),
  layerZeroLabsDVN: envAddress("LAYERZERO_LABS_DVN"),
  confirmations: envBigInt("CONFIRMATIONS"),
  maxMessageSize: Number(maxMessageSizeValue),
  minLzReceiveGas,
  maxLzReceiveGas: envBigInt("MAX_LZ_RECEIVE_GAS"),
  executorPriceConfig: workerPriceConfig("EXECUTOR", priceUpdatedAt),
  dvnPriceConfig: workerPriceConfig("DVN", priceUpdatedAt),
  dvnVerifier,
  enforcedLzReceiveGas,
});

console.log(jsonStringify(parameters));

function workerPriceConfig(
  prefix: "EXECUTOR" | "DVN",
  defaultUpdatedAt: bigint,
): WorkerPriceConfigInput {
  return {
    baseFee: envBigInt(`${prefix}_PRICE_BASE_FEE`),
    dstGasPriceInSrcToken: envBigInt(
      `${prefix}_PRICE_DST_GAS_PRICE_IN_SRC_TOKEN`,
    ),
    bufferBps: envBigInt(`${prefix}_PRICE_BUFFER_BPS`),
    updatedAt: optionalUint64(`${prefix}_PRICE_UPDATED_AT`, defaultUpdatedAt),
    staleAfter: envBigInt(`${prefix}_PRICE_STALE_AFTER`),
  };
}
