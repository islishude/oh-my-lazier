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

export type PriceSnapshotParam = {
  dstGasPriceInSrcToken: string;
  dstDataFeePerByteInSrcToken: string;
  updatedAt: string;
  staleAfter: string;
};

export type WorkerFeeModelParam = {
  baseFee: string;
  dstGasOverhead: string;
  dataSizeOverheadBytes: string;
  marginBps: number;
};

export type OpenWorkersParameters = {
  owner: Address;
  priceFeedSubmitters: Address[];
};

export type OpenWorkersParameterFile = {
  OpenWorkers: OpenWorkersParameters;
};

export type TestOFTParameters = {
  tokenName: string;
  tokenSymbol: string;
  endpoint: Address;
  delegate: Address;
  initialRecipient: Address;
  initialSupply: string;
};

export type TestOFTParameterFile = {
  TestOFT: TestOFTParameters;
};

export type OAppEndpointConfigParameters = {
  oapp: Address;
  endpoint: Address;
  delegate?: Address;
  remoteEid: number;
  remotePeer: Hex;
  sendUln: Address;
  receiveUln: Address;
  receiveLibraryGracePeriod: number;
  sendConfig: SetConfigEntry[];
  receiveConfig: SetConfigEntry[];
  enforcedOptions: EnforcedOptionParam[];
};

export type OAppEndpointConfigParameterFile = {
  OAppEndpointConfig: OAppEndpointConfigParameters;
};

export type OpenWorkersPathwayConfigParameters = {
  oapp: Address;
  remoteEid: number;
  sendUln: Address;
  openExecutor: Address;
  openDVN: Address;
  priceFeed: Address;
  bootstrapPriceSubmitter: Address;
  dvnVerifier?: Address;
  workerPathwayConfig: WorkerPathwayConfigParam;
  priceSnapshot: PriceSnapshotParam;
  executorFeeModel: WorkerFeeModelParam;
  dvnFeeModel: WorkerFeeModelParam;
};

export type OpenWorkersPathwayConfigParameterFile = {
  OpenWorkersPathwayConfig: OpenWorkersPathwayConfigParameters;
};

export type PriceSnapshotInput = {
  dstGasPriceInSrcToken: bigint;
  dstDataFeePerByteInSrcToken: bigint;
  updatedAt: bigint;
  staleAfter: bigint;
};

export type WorkerFeeModelInput = {
  baseFee: bigint;
  dstGasOverhead: bigint;
  dataSizeOverheadBytes: bigint;
  marginBps: number | bigint;
};

export type PathwayConfigInput = {
  oapp: Address;
  endpoint: Address;
  delegate?: Address;
  remoteEid: number;
  remoteOApp: Address;
  sendUln: Address;
  receiveUln: Address;
  openExecutor: Address;
  openDVN: Address;
  priceFeed: Address;
  bootstrapPriceSubmitter: Address;
  layerZeroLabsDVN: Address;
  confirmations: bigint;
  maxMessageSize: number;
  minLzReceiveGas: bigint;
  maxLzReceiveGas: bigint;
  priceSnapshot: PriceSnapshotInput;
  executorFeeModel: WorkerFeeModelInput;
  dvnFeeModel: WorkerFeeModelInput;
  dvnVerifier?: Address;
  enforcedLzReceiveGas: bigint;
  receiveLibraryGracePeriod?: number;
};

export function buildOpenWorkersParameters(input: {
  owner: Address;
  priceFeedSubmitters: Address[];
}): OpenWorkersParameterFile {
  return {
    OpenWorkers: {
      owner: input.owner,
      priceFeedSubmitters: input.priceFeedSubmitters,
    },
  };
}

export function buildTestOFTParameters(input: {
  tokenName: string;
  tokenSymbol: string;
  endpoint: Address;
  delegate: Address;
  initialRecipient: Address;
  initialSupply: string;
}): TestOFTParameterFile {
  return {
    TestOFT: {
      tokenName: input.tokenName,
      tokenSymbol: input.tokenSymbol,
      endpoint: input.endpoint,
      delegate: input.delegate,
      initialRecipient: input.initialRecipient,
      initialSupply: `${input.initialSupply}n`,
    },
  };
}

export function buildOAppEndpointConfigParameters(
  input: PathwayConfigInput,
): OAppEndpointConfigParameterFile {
  const common = buildCommonPathwayParameters(input);
  const params: OAppEndpointConfigParameters = {
    oapp: input.oapp,
    endpoint: input.endpoint,
    remoteEid: common.remoteEid,
    remotePeer: addressToBytes32(input.remoteOApp),
    sendUln: input.sendUln,
    receiveUln: input.receiveUln,
    receiveLibraryGracePeriod: common.receiveLibraryGracePeriod,
    sendConfig: common.sendConfig,
    receiveConfig: common.receiveConfig,
    enforcedOptions: common.enforcedOptions,
  };
  if (input.delegate !== undefined) {
    params.delegate = input.delegate;
  }
  return { OAppEndpointConfig: params };
}

export function buildOpenWorkersPathwayConfigParameters(
  input: PathwayConfigInput,
): OpenWorkersPathwayConfigParameterFile {
  const common = buildCommonPathwayParameters(input);
  const params: OpenWorkersPathwayConfigParameters = {
    oapp: input.oapp,
    remoteEid: common.remoteEid,
    sendUln: input.sendUln,
    openExecutor: input.openExecutor,
    openDVN: input.openDVN,
    priceFeed: input.priceFeed,
    bootstrapPriceSubmitter: input.bootstrapPriceSubmitter,
    workerPathwayConfig: common.workerPathwayConfig,
    priceSnapshot: common.priceSnapshot,
    executorFeeModel: common.executorFeeModel,
    dvnFeeModel: common.dvnFeeModel,
  };
  if (input.dvnVerifier !== undefined) {
    params.dvnVerifier = input.dvnVerifier;
  }
  return { OpenWorkersPathwayConfig: params };
}

function buildCommonPathwayParameters(input: PathwayConfigInput) {
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
  return {
    remoteEid,
    receiveLibraryGracePeriod,
    workerPathwayConfig: {
      enabled: true,
      maxMessageSize: maxMessageSize.toString(),
      minLzReceiveGas,
      maxLzReceiveGas,
    },
    priceSnapshot: normalizePriceSnapshot(input.priceSnapshot, "priceSnapshot"),
    executorFeeModel: normalizeWorkerFeeModel(
      input.executorFeeModel,
      "executorFeeModel",
    ),
    dvnFeeModel: normalizeWorkerFeeModel(input.dvnFeeModel, "dvnFeeModel"),
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
}

function normalizePriceSnapshot(
  config: PriceSnapshotInput,
  label: string,
): PriceSnapshotParam {
  return {
    dstGasPriceInSrcToken: normalizeUint256(
      config.dstGasPriceInSrcToken,
      `${label}.dstGasPriceInSrcToken`,
    ),
    dstDataFeePerByteInSrcToken: normalizeUint256(
      config.dstDataFeePerByteInSrcToken,
      `${label}.dstDataFeePerByteInSrcToken`,
    ),
    updatedAt: normalizeUint64(config.updatedAt, `${label}.updatedAt`),
    staleAfter: normalizeUint64(config.staleAfter, `${label}.staleAfter`),
  };
}

function normalizeWorkerFeeModel(
  model: WorkerFeeModelInput,
  label: string,
): WorkerFeeModelParam {
  return {
    baseFee: normalizeUint256(model.baseFee, `${label}.baseFee`),
    dstGasOverhead: normalizeUint64(
      model.dstGasOverhead,
      `${label}.dstGasOverhead`,
    ),
    dataSizeOverheadBytes: normalizeUint64(
      model.dataSizeOverheadBytes,
      `${label}.dataSizeOverheadBytes`,
    ),
    marginBps: normalizeBps(model.marginBps, `${label}.marginBps`),
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
