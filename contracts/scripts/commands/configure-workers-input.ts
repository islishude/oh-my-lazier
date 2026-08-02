import {
  optionalField,
  requiredField,
  type ValueParser,
} from "../command-harness.js";
import type {
  ConfigureWorkersFeeModelInput,
  ConfigureWorkersInput,
  ConfigureWorkersPriceSnapshotInput,
  ConfigureWorkersRateLimitInput,
} from "../configure-workers.js";
import {
  addressField,
  bigintField,
  optionalAddressField,
  optionalBigintField,
  parseInputObject,
  uint32Field,
} from "./input-parsers.js";
import type { Address } from "viem";

export type ConfigureWorkersCommandInput = ConfigureWorkersInput & {
  expectedSigner: Address;
};

export function parseConfigureWorkersCommandInput(
  value: unknown,
  label: string
): ConfigureWorkersCommandInput {
  const input = parseInputObject(value, label, [
    "testOFT",
    "openExecutor",
    "openDVN",
    "priceFeed",
    "remoteEid",
    "sendLib",
    "srcOApp",
    "maxMessageSize",
    "minLzReceiveGas",
    "maxLzReceiveGas",
    "priceSnapshot",
    "executorFeeModel",
    "dvnFeeModel",
    "rateLimit",
    "expectedSigner",
  ]);
  return {
    testOFT: optionalAddressField(input, "testOFT", label),
    openExecutor: addressField(input, "openExecutor", label),
    openDVN: addressField(input, "openDVN", label),
    priceFeed: addressField(input, "priceFeed", label),
    remoteEid: uint32Field(input, "remoteEid", label),
    sendLib: addressField(input, "sendLib", label),
    srcOApp: optionalAddressField(input, "srcOApp", label),
    maxMessageSize: bigintField(input, "maxMessageSize", label),
    minLzReceiveGas: bigintField(input, "minLzReceiveGas", label),
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
    rateLimit: optionalField(input, "rateLimit", parseRateLimit, label),
    expectedSigner: addressField(input, "expectedSigner", label),
  };
}

const parsePriceSnapshot: ValueParser<ConfigureWorkersPriceSnapshotInput> = (
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

const parseFeeModel: ValueParser<ConfigureWorkersFeeModelInput> = (
  value,
  label
) => {
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

const parseRateLimit: ValueParser<ConfigureWorkersRateLimitInput> = (
  value,
  label
) => {
  const input = parseInputObject(value, label, ["capacity", "refillPerSecond"]);
  return {
    capacity: bigintField(input, "capacity", label),
    refillPerSecond: bigintField(input, "refillPerSecond", label),
  };
};
