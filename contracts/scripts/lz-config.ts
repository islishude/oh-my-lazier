import {
  decodeAbiParameters,
  encodeAbiParameters,
  type Address,
  type Hex,
} from "viem";

export const CONFIG_TYPE_EXECUTOR = 1;
export const CONFIG_TYPE_ULN = 2;
export const NIL_DVN_COUNT = 255;

const executorConfigParameters = [
  {
    type: "tuple",
    components: [
      { name: "maxMessageSize", type: "uint32" },
      { name: "executor", type: "address" },
    ],
  },
] as const;

const ulnConfigParameters = [
  {
    type: "tuple",
    components: [
      { name: "confirmations", type: "uint64" },
      { name: "requiredDVNCount", type: "uint8" },
      { name: "optionalDVNCount", type: "uint8" },
      { name: "optionalDVNThreshold", type: "uint8" },
      { name: "requiredDVNs", type: "address[]" },
      { name: "optionalDVNs", type: "address[]" },
    ],
  },
] as const;

export type ExecutorConfig = {
  maxMessageSize: number;
  executor: Address;
};

export type UlnConfig = {
  confirmations: bigint;
  requiredDVNCount: number;
  optionalDVNCount: number;
  optionalDVNThreshold: number;
  requiredDVNs: Address[];
  optionalDVNs: Address[];
};

export type LzConfigSnapshot = {
  endpoint: Address;
  oapp: Address;
  remoteEid: number;
  inspectedLibraries: {
    sendUln: Address;
    receiveUln: Address;
  };
  executorConfig: ExecutorConfig;
  sendUlnConfig: UlnConfig;
  receiveUlnConfig: UlnConfig;
};

export type SetConfigEntry = {
  eid: number;
  configType: number;
  config: Hex;
};

export type SetConfigBatch = {
  label: string;
  library: Address;
  configs: SetConfigEntry[];
};

export function encodeExecutorConfig(config: ExecutorConfig): Hex {
  return encodeAbiParameters(executorConfigParameters, [config]);
}

export function decodeExecutorConfig(config: Hex): ExecutorConfig {
  const [decoded] = decodeAbiParameters(executorConfigParameters, config);
  return {
    maxMessageSize: decoded.maxMessageSize,
    executor: decoded.executor,
  };
}

export function encodeUlnConfig(config: UlnConfig): Hex {
  return encodeAbiParameters(ulnConfigParameters, [config]);
}

export function decodeUlnConfig(config: Hex): UlnConfig {
  const [decoded] = decodeAbiParameters(ulnConfigParameters, config);
  return {
    confirmations: decoded.confirmations,
    requiredDVNCount: decoded.requiredDVNCount,
    optionalDVNCount: decoded.optionalDVNCount,
    optionalDVNThreshold: decoded.optionalDVNThreshold,
    requiredDVNs: [...decoded.requiredDVNs],
    optionalDVNs: [...decoded.optionalDVNs],
  };
}

export function rollbackConfigBatches(
  snapshot: LzConfigSnapshot,
): SetConfigBatch[] {
  const normalized = normalizeLzConfigSnapshot(snapshot);
  return [
    {
      label: "Endpoint.setConfig SendUln302 rollback",
      library: normalized.inspectedLibraries.sendUln,
      configs: [
        {
          eid: normalized.remoteEid,
          configType: CONFIG_TYPE_EXECUTOR,
          config: encodeExecutorConfig(normalized.executorConfig),
        },
        {
          eid: normalized.remoteEid,
          configType: CONFIG_TYPE_ULN,
          config: encodeUlnConfig(normalized.sendUlnConfig),
        },
      ],
    },
    {
      label: "Endpoint.setConfig ReceiveUln302 rollback",
      library: normalized.inspectedLibraries.receiveUln,
      configs: [
        {
          eid: normalized.remoteEid,
          configType: CONFIG_TYPE_ULN,
          config: encodeUlnConfig(normalized.receiveUlnConfig),
        },
      ],
    },
  ];
}

export function requiredDVNsConfig(
  confirmations: bigint,
  dvns: Address[],
): UlnConfig {
  const requiredDVNs = sortUniqueAddresses(dvns);
  return {
    confirmations,
    requiredDVNCount: requiredDVNs.length,
    optionalDVNCount: NIL_DVN_COUNT,
    optionalDVNThreshold: 0,
    requiredDVNs,
    optionalDVNs: [],
  };
}

function sortUniqueAddresses(addresses: Address[]): Address[] {
  const sorted = [...addresses].sort((left, right) =>
    left.toLowerCase().localeCompare(right.toLowerCase()),
  );
  for (let index = 1; index < sorted.length; index++) {
    if (sorted[index - 1].toLowerCase() === sorted[index].toLowerCase()) {
      throw new Error(`duplicate DVN address ${sorted[index]}`);
    }
  }
  return sorted;
}

function normalizeLzConfigSnapshot(
  snapshot: LzConfigSnapshot,
): LzConfigSnapshot {
  return {
    ...snapshot,
    remoteEid: normalizeUint32(snapshot.remoteEid, "snapshot.remoteEid"),
    executorConfig: {
      maxMessageSize: normalizeUint32(
        snapshot.executorConfig.maxMessageSize,
        "snapshot.executorConfig.maxMessageSize",
      ),
      executor: snapshot.executorConfig.executor,
    },
    sendUlnConfig: normalizeUlnConfig(
      snapshot.sendUlnConfig,
      "snapshot.sendUlnConfig",
    ),
    receiveUlnConfig: normalizeUlnConfig(
      snapshot.receiveUlnConfig,
      "snapshot.receiveUlnConfig",
    ),
  };
}

function normalizeUlnConfig(config: UlnConfig, label: string): UlnConfig {
  const requiredDVNCount = normalizeUint8(
    config.requiredDVNCount,
    `${label}.requiredDVNCount`,
  );
  const optionalDVNCount = normalizeUint8(
    config.optionalDVNCount,
    `${label}.optionalDVNCount`,
  );
  const requiredDVNs = [...config.requiredDVNs];
  const optionalDVNs = [...config.optionalDVNs];
  if (
    requiredDVNCount !== NIL_DVN_COUNT &&
    requiredDVNCount !== requiredDVNs.length
  ) {
    throw new Error(`${label}.requiredDVNCount does not match requiredDVNs`);
  }
  if (
    optionalDVNCount !== NIL_DVN_COUNT &&
    optionalDVNCount !== optionalDVNs.length
  ) {
    throw new Error(`${label}.optionalDVNCount does not match optionalDVNs`);
  }
  return {
    confirmations: BigInt(config.confirmations),
    requiredDVNCount,
    optionalDVNCount,
    optionalDVNThreshold: normalizeUint8(
      config.optionalDVNThreshold,
      `${label}.optionalDVNThreshold`,
    ),
    requiredDVNs,
    optionalDVNs,
  };
}

function normalizeUint8(value: number, label: string): number {
  if (!Number.isInteger(value) || value < 0 || value > 0xff) {
    throw new Error(`${label} must be a uint8`);
  }
  return value;
}

function normalizeUint32(value: number, label: string): number {
  if (!Number.isInteger(value) || value < 0 || value > 0xffffffff) {
    throw new Error(`${label} must be a uint32`);
  }
  return value;
}
