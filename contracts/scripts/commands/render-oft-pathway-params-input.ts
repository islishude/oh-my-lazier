import {
  parseAddress,
  parseArray,
  requiredField,
  type ValueParser,
} from "../command-harness.js";
import type { WorkerFeeModelInput } from "../oft-pathway-ignition.js";
import type {
  RenderOftPathwayParamsInput,
  RenderOftPathwayPriceSnapshotInput,
} from "../render-oft-pathway-params.js";
import {
  addressField,
  bigintField,
  optionalAddressField,
  optionalBigintField,
  optionalBooleanField,
  optionalParsedField,
  parseInputObject,
  uint32Field,
} from "./input-parsers.js";

export function parseRenderOftPathwayParamsInput(
  value: unknown,
  label: string
): RenderOftPathwayParamsInput {
  const input = parseInputObject(value, label, [
    "oapp",
    "endpoint",
    "delegate",
    "remoteEid",
    "remoteOApp",
    "sendUln",
    "receiveUln",
    "openExecutor",
    "openDVN",
    "priceFeed",
    "bootstrapPriceSubmitter",
    "requiredDVNs",
    "includeLayerZeroLabsDVN",
    "confirmations",
    "maxMessageSize",
    "minLzReceiveGas",
    "maxLzReceiveGas",
    "priceSnapshot",
    "executorFeeModel",
    "dvnFeeModel",
    "dvnVerifier",
    "enforcedLzReceiveGas",
  ]);
  return {
    oapp: addressField(input, "oapp", label),
    endpoint: addressField(input, "endpoint", label),
    delegate: optionalAddressField(input, "delegate", label),
    remoteEid: uint32Field(input, "remoteEid", label),
    remoteOApp: addressField(input, "remoteOApp", label),
    sendUln: addressField(input, "sendUln", label),
    receiveUln: addressField(input, "receiveUln", label),
    openExecutor: addressField(input, "openExecutor", label),
    openDVN: addressField(input, "openDVN", label),
    priceFeed: addressField(input, "priceFeed", label),
    bootstrapPriceSubmitter: addressField(
      input,
      "bootstrapPriceSubmitter",
      label
    ),
    requiredDVNs: optionalParsedField(
      input,
      "requiredDVNs",
      (item, fieldLabel) =>
        parseArray(item, fieldLabel, parseAddress, { minLength: 1 }),
      label
    ),
    includeLayerZeroLabsDVN: optionalBooleanField(
      input,
      "includeLayerZeroLabsDVN",
      label
    ),
    confirmations: bigintField(input, "confirmations", label),
    maxMessageSize: uint32Field(input, "maxMessageSize", label),
    minLzReceiveGas: optionalBigintField(input, "minLzReceiveGas", label),
    maxLzReceiveGas: bigintField(input, "maxLzReceiveGas", label),
    priceSnapshot: requiredField(
      input,
      "priceSnapshot",
      parsePriceSnapshot,
      label
    ),
    executorFeeModel: requiredField(
      input,
      "executorFeeModel",
      parseFeeModel,
      label
    ),
    dvnFeeModel: requiredField(input, "dvnFeeModel", parseFeeModel, label),
    dvnVerifier: optionalAddressField(input, "dvnVerifier", label),
    enforcedLzReceiveGas: bigintField(input, "enforcedLzReceiveGas", label),
  };
}

const parsePriceSnapshot: ValueParser<RenderOftPathwayPriceSnapshotInput> = (
  value,
  label
) => {
  const input = parseInputObject(value, label, [
    "dstGasPriceInSrcToken",
    "dstDataFeePerByteInSrcToken",
    "updatedAt",
    "staleAfter",
  ]);
  return {
    dstGasPriceInSrcToken: bigintField(input, "dstGasPriceInSrcToken", label),
    dstDataFeePerByteInSrcToken: bigintField(
      input,
      "dstDataFeePerByteInSrcToken",
      label
    ),
    updatedAt: optionalBigintField(input, "updatedAt", label),
    staleAfter: bigintField(input, "staleAfter", label),
  };
};

const parseFeeModel: ValueParser<WorkerFeeModelInput> = (value, label) => {
  const input = parseInputObject(value, label, [
    "baseFee",
    "dstGasOverhead",
    "dataSizeOverheadBytes",
    "marginBps",
  ]);
  return {
    baseFee: bigintField(input, "baseFee", label),
    dstGasOverhead: bigintField(input, "dstGasOverhead", label),
    dataSizeOverheadBytes: bigintField(input, "dataSizeOverheadBytes", label),
    marginBps: bigintField(input, "marginBps", label),
  };
};
