import {
  decodeEventLog,
  encodeEventTopics,
  getAddress,
  isAddressEqual,
  type Abi,
  type Address,
  type Hex,
} from "viem";

export type FeeEventLog = {
  address: Address;
  topics: readonly Hex[];
  data: Hex;
};

export type SourceWorkerFeeClaimRole =
  | "open_executor"
  | "primary_open_dvn"
  | "secondary_open_dvn";

export type SourceWorkerFeeClaim = {
  role: SourceWorkerFeeClaimRole;
  worker: Address;
  amount: bigint;
};

export type SourceWorkerFeeClaimInput = {
  sourceName: string;
  logs: readonly FeeEventLog[];
  sendLib: Address;
  sendLibAbi: Abi;
  openExecutor: Address;
  primaryOpenDVN: Address;
  secondaryOpenDVN: Address;
  executorFee: bigint;
};

export type SourceExecutorFeeTotalInput = {
  sourceName: string;
  logs: readonly FeeEventLog[];
  sendLib: Address;
  sendLibAbi: Abi;
  openExecutor: Address;
};

export function sourceExecutorFeeTotal(
  input: SourceExecutorFeeTotalInput,
): bigint {
  let total = 0n;
  let found = false;
  for (const log of input.logs) {
    if (!isAddressEqual(log.address, input.sendLib)) {
      continue;
    }
    if (log.topics[0] !== eventTopic(input.sendLibAbi, "ExecutorFeePaid")) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: input.sendLibAbi,
      eventName: "ExecutorFeePaid",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      executor: Address;
      fee: bigint;
    };
    if (!isAddressEqual(args.executor, input.openExecutor)) {
      throw new Error(
        `${input.sourceName} ExecutorFeePaid executor ${args.executor} does not match expected ${input.openExecutor}`,
      );
    }
    total += args.fee;
    found = true;
  }
  if (!found) {
    throw new Error(`${input.sourceName} source receipt is missing SendUln302 ExecutorFeePaid`);
  }
  return total;
}

export function sourceWorkerFeeClaims(
  input: SourceWorkerFeeClaimInput,
): SourceWorkerFeeClaim[] {
  const dvnFees = sourceDVNFeeMap(input);
  return [
    positiveClaim(
      "open_executor",
      input.openExecutor,
      input.executorFee,
      `${input.sourceName} ExecutorFeePaid`,
    ),
    positiveClaim(
      "primary_open_dvn",
      input.primaryOpenDVN,
      requiredDVNFee(input, dvnFees, "primary_open_dvn", input.primaryOpenDVN),
      `${input.sourceName} DVNFeePaid`,
    ),
    positiveClaim(
      "secondary_open_dvn",
      input.secondaryOpenDVN,
      requiredDVNFee(input, dvnFees, "secondary_open_dvn", input.secondaryOpenDVN),
      `${input.sourceName} DVNFeePaid`,
    ),
  ];
}

function sourceDVNFeeMap(
  input: SourceWorkerFeeClaimInput,
): Map<string, bigint> {
  const fees = new Map<string, bigint>();
  let found = false;
  for (const log of input.logs) {
    if (!isAddressEqual(log.address, input.sendLib)) {
      continue;
    }
    if (log.topics[0] !== eventTopic(input.sendLibAbi, "DVNFeePaid")) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: input.sendLibAbi,
      eventName: "DVNFeePaid",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as {
      requiredDVNs: Address[];
      optionalDVNs: Address[];
      fees: bigint[];
    };
    const expectedFeeCount = args.requiredDVNs.length + args.optionalDVNs.length;
    if (args.fees.length < expectedFeeCount) {
      throw new Error(
        `${input.sourceName} DVNFeePaid has ${args.fees.length} fees for ${expectedFeeCount} DVNs`,
      );
    }
    const dvns = [...args.requiredDVNs, ...args.optionalDVNs];
    for (let index = 0; index < dvns.length; index++) {
      const fee = args.fees[index];
      if (fee === undefined) {
        throw new Error(`${input.sourceName} DVNFeePaid fee ${index} missing`);
      }
      const key = getAddress(dvns[index]).toLowerCase();
      fees.set(key, (fees.get(key) ?? 0n) + fee);
    }
    found = true;
  }
  if (!found) {
    throw new Error(`${input.sourceName} source receipt is missing SendUln302 DVNFeePaid`);
  }
  return fees;
}

function requiredDVNFee(
  input: SourceWorkerFeeClaimInput,
  fees: Map<string, bigint>,
  role: SourceWorkerFeeClaimRole,
  worker: Address,
): bigint {
  const fee = fees.get(getAddress(worker).toLowerCase());
  if (fee === undefined) {
    throw new Error(
      `${input.sourceName} DVNFeePaid missing source DVN ${role} ${worker}`,
    );
  }
  return fee;
}

function positiveClaim(
  role: SourceWorkerFeeClaimRole,
  worker: Address,
  amount: bigint,
  label: string,
): SourceWorkerFeeClaim {
  if (amount <= 0n) {
    throw new Error(`${label} ${role} fee must be positive`);
  }
  return { role, worker: getAddress(worker), amount };
}

function eventTopic(abi: Abi, eventName: string): Hex {
  const topic = encodeEventTopics({ abi, eventName })[0];
  if (topic === null || Array.isArray(topic)) {
    throw new Error(`event ${eventName} did not produce a single topic`);
  }
  return topic;
}

function mutableTopics(topics: readonly Hex[]): [Hex, ...Hex[]] {
  if (topics.length === 0) {
    throw new Error("log is missing topics");
  }
  return [...topics] as [Hex, ...Hex[]];
}
