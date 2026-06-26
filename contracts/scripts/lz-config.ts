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
