import {
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  optionalAddress,
  optionalAddressList,
  optionalBigInt,
  optionalBool,
  optionalUint64,
} from "./lib.js";
import { requireLayerZeroLabsDVNForLibraries } from "./lz-addresses.js";
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
const openDVN = envAddress("OPEN_DVN");
const endpoint = envAddress("ENDPOINT");
const sendUln = envAddress("SEND_ULN");
const receiveUln = envAddress("RECEIVE_ULN");
const includeLayerZeroLabsDVN =
  optionalBool("INCLUDE_LAYERZERO_LABS_DVN") ?? false;
const requiredDVNs = resolveRequiredDVNs({
  openDVN,
  endpoint,
  sendUln,
  receiveUln,
  explicit: optionalAddressList("REQUIRED_DVNS"),
  includeLayerZeroLabsDVN,
});

const input = {
  oapp: envAddress("OAPP"),
  endpoint,
  delegate,
  remoteEid: envUint32("REMOTE_EID"),
  remoteOApp: envAddress("REMOTE_OAPP"),
  sendUln,
  receiveUln,
  openExecutor: envAddress("OPEN_EXECUTOR"),
  openDVN,
  priceFeed: envAddress("PRICE_FEED"),
  bootstrapPriceSubmitter: envAddress("BOOTSTRAP_PRICE_SUBMITTER"),
  requiredDVNs,
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

function resolveRequiredDVNs(input: {
  openDVN: `0x${string}`;
  endpoint: `0x${string}`;
  sendUln: `0x${string}`;
  receiveUln: `0x${string}`;
  explicit?: `0x${string}`[];
  includeLayerZeroLabsDVN: boolean;
}) {
  const dvns = input.explicit ?? [input.openDVN];
  if (!input.includeLayerZeroLabsDVN) {
    return dvns;
  }
  return [
    ...dvns,
    requireLayerZeroLabsDVNForLibraries(
      {
        endpointV2: input.endpoint,
        sendUln302: input.sendUln,
        receiveUln302: input.receiveUln,
      },
      "INCLUDE_LAYERZERO_LABS_DVN",
    ),
  ];
}

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
