import { type Address, type Hex } from "viem";
import {
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  encodeExecutorConfig,
  encodeUlnConfig,
  requiredDVNsConfig,
  type SetConfigEntry,
} from "./lz-config.js";
import { addressToBytes32 } from "./lib.js";
import { buildLzReceiveOption } from "./oft-canary.js";

export const OFT_MSG_TYPE_SEND = 1;

export type EnforcedOptionParam = {
  eid: number;
  msgType: number;
  options: Hex;
};

export type WorkerPathwayConfigParam = {
  enabled: boolean;
  maxMessageSize: string;
  minLzReceiveGas: string;
  maxLzReceiveGas: string;
};

export type WorkerPriceConfigParam = {
  baseFee: string;
  dstGasPriceInSrcToken: string;
  dstGasOverhead: string;
  marginBps: number;
  updatedAt: string;
  staleAfter: string;
};

export type TestOFTPathwayConfigParameters = {
  testOFT: Address;
  endpoint: Address;
  delegate?: Address;
  remoteEid: number;
  remotePeer: Hex;
  sendUln: Address;
  receiveUln: Address;
  openExecutor: Address;
  openDVN: Address;
  dvnVerifier?: Address;
  workerPathwayConfig: WorkerPathwayConfigParam;
  executorPriceConfig: WorkerPriceConfigParam;
  dvnPriceConfig: WorkerPriceConfigParam;
  receiveLibraryGracePeriod: number;
  sendConfig: SetConfigEntry[];
  receiveConfig: SetConfigEntry[];
  enforcedOptions: EnforcedOptionParam[];
};

export type TestOFTPathwayConfigParameterFile = {
  TestOFTPathwayConfig: TestOFTPathwayConfigParameters;
};

export type WorkerPriceConfigInput = {
  baseFee: bigint;
  dstGasPriceInSrcToken: bigint;
  dstGasOverhead: bigint;
  marginBps: number | bigint;
  updatedAt: bigint;
  staleAfter: bigint;
};

export function buildTestOFTPathwayConfigParameters(input: {
  testOFT: Address;
  endpoint: Address;
  delegate?: Address;
  remoteEid: number;
  remoteOFT: Address;
  sendUln: Address;
  receiveUln: Address;
  openExecutor: Address;
  openDVN: Address;
  layerZeroLabsDVN: Address;
  confirmations: bigint;
  maxMessageSize: number;
  minLzReceiveGas: bigint;
  maxLzReceiveGas: bigint;
  executorPriceConfig: WorkerPriceConfigInput;
  dvnPriceConfig: WorkerPriceConfigInput;
  dvnVerifier?: Address;
  enforcedLzReceiveGas: bigint;
  receiveLibraryGracePeriod?: number;
}): TestOFTPathwayConfigParameterFile {
  const remoteEid = normalizeUint32(input.remoteEid, "remoteEid");
  const maxMessageSize = normalizeUint32(
    input.maxMessageSize,
    "maxMessageSize",
  );
  const receiveLibraryGracePeriod = normalizeUint32(
    input.receiveLibraryGracePeriod ?? 0,
    "receiveLibraryGracePeriod",
  );
  const minLzReceiveGas = normalizeUint128(
    input.minLzReceiveGas,
    "minLzReceiveGas",
  );
  const maxLzReceiveGas = normalizeUint128(
    input.maxLzReceiveGas,
    "maxLzReceiveGas",
  );
  if (input.minLzReceiveGas > input.maxLzReceiveGas) {
    throw new Error("minLzReceiveGas must not exceed maxLzReceiveGas");
  }
  const ulnConfig = requiredDVNsConfig(input.confirmations, [
    input.openDVN,
    input.layerZeroLabsDVN,
  ]);
  const encodedUlnConfig = encodeUlnConfig(ulnConfig);
  const params: TestOFTPathwayConfigParameters = {
    testOFT: input.testOFT,
    endpoint: input.endpoint,
    remoteEid,
    remotePeer: addressToBytes32(input.remoteOFT),
    sendUln: input.sendUln,
    receiveUln: input.receiveUln,
    openExecutor: input.openExecutor,
    openDVN: input.openDVN,
    workerPathwayConfig: {
      enabled: true,
      maxMessageSize: maxMessageSize.toString(),
      minLzReceiveGas,
      maxLzReceiveGas,
    },
    executorPriceConfig: normalizeWorkerPriceConfig(
      input.executorPriceConfig,
      "executorPriceConfig",
    ),
    dvnPriceConfig: normalizeWorkerPriceConfig(
      input.dvnPriceConfig,
      "dvnPriceConfig",
    ),
    receiveLibraryGracePeriod,
    sendConfig: [
      {
        eid: remoteEid,
        configType: CONFIG_TYPE_EXECUTOR,
        config: encodeExecutorConfig({
          maxMessageSize,
          executor: input.openExecutor,
        }),
      },
      {
        eid: remoteEid,
        configType: CONFIG_TYPE_ULN,
        config: encodedUlnConfig,
      },
    ],
    receiveConfig: [
      {
        eid: remoteEid,
        configType: CONFIG_TYPE_ULN,
        config: encodedUlnConfig,
      },
    ],
    enforcedOptions: [
      {
        eid: remoteEid,
        msgType: OFT_MSG_TYPE_SEND,
        options: buildLzReceiveOption(input.enforcedLzReceiveGas),
      },
    ],
  };
  if (input.delegate !== undefined) {
    params.delegate = input.delegate;
  }
  if (input.dvnVerifier !== undefined) {
    params.dvnVerifier = input.dvnVerifier;
  }
  return { TestOFTPathwayConfig: params };
}

function normalizeWorkerPriceConfig(
  config: WorkerPriceConfigInput,
  label: string,
): WorkerPriceConfigParam {
  return {
    baseFee: normalizeUint256(config.baseFee, `${label}.baseFee`),
    dstGasPriceInSrcToken: normalizeUint256(
      config.dstGasPriceInSrcToken,
      `${label}.dstGasPriceInSrcToken`,
    ),
    dstGasOverhead: normalizeUint64(
      config.dstGasOverhead,
      `${label}.dstGasOverhead`,
    ),
    marginBps: normalizeBps(config.marginBps, `${label}.marginBps`),
    updatedAt: normalizeUint64(config.updatedAt, `${label}.updatedAt`),
    staleAfter: normalizeUint64(config.staleAfter, `${label}.staleAfter`),
  };
}

function normalizeUint32(value: number, label: string): number {
  if (!Number.isInteger(value) || value < 0 || value > 0xffffffff) {
    throw new Error(`${label} must be a uint32`);
  }
  return value;
}

function normalizeUint128(value: bigint, label: string): string {
  if (value < 0n || value > (1n << 128n) - 1n) {
    throw new Error(`${label} must be a uint128`);
  }
  return value.toString();
}

function normalizeUint64(value: bigint, label: string): string {
  if (value < 0n || value > (1n << 64n) - 1n) {
    throw new Error(`${label} must be a uint64`);
  }
  return value.toString();
}

function normalizeUint256(value: bigint, label: string): string {
  if (value < 0n || value > (1n << 256n) - 1n) {
    throw new Error(`${label} must be a uint256`);
  }
  return value.toString();
}

function normalizeBps(value: number | bigint, label: string): number {
  if (typeof value === "number" && !Number.isInteger(value)) {
    throw new Error(`${label} must be between 0 and 10000 bps`);
  }
  const bigintValue = typeof value === "bigint" ? value : BigInt(value);
  if (bigintValue < 0n || bigintValue > 10_000n) {
    throw new Error(`${label} must be between 0 and 10000 bps`);
  }
  return Number(bigintValue);
}
