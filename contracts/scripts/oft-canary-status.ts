import { decodeEventLog, encodeEventTopics, getAddress, isAddressEqual } from "viem";
import type { Abi, Address, Hex } from "viem";

export type CanaryLog = {
  address: Address;
  topics: readonly Hex[];
  data: Hex;
};

export type SourceCanaryStatusInput = {
  logs: readonly CanaryLog[];
  endpoint: Address;
  sendLib: Address;
  expectedExecutor: Address;
  endpointAbi: Abi;
  sendLibAbi: Abi;
};

export type SourceCanaryStatus = {
  packetSent: boolean;
  sendLibrary: Address;
  executor: Address;
  executorFee: bigint;
};

export type DestinationCanaryStatusInput = {
  logs: readonly CanaryLog[];
  endpoint: Address;
  endpointAbi: Abi;
};

export type DestinationCanaryStatus = {
  packetDelivered: boolean;
};

export type CanaryRecipientBalanceStatus = {
  recipient: Address;
  balance: bigint;
  minBalance: bigint;
};

export function assertCanarySourceReceipt(
  input: SourceCanaryStatusInput,
): SourceCanaryStatus {
  const packetSent = findPacketSent(input.logs, input.endpoint, input.endpointAbi);
  if (packetSent === undefined) {
    throw new Error("source receipt is missing EndpointV2 PacketSent");
  }
  if (!isAddressEqual(packetSent.sendLibrary, input.sendLib)) {
    throw new Error(
      `PacketSent sendLibrary ${packetSent.sendLibrary} does not match expected ${input.sendLib}`,
    );
  }

  const feePaid = findExecutorFeePaid(input.logs, input.sendLib, input.sendLibAbi);
  if (feePaid === undefined) {
    throw new Error("source receipt is missing SendLib ExecutorFeePaid");
  }
  if (!isAddressEqual(feePaid.executor, input.expectedExecutor)) {
    throw new Error(
      `ExecutorFeePaid executor ${feePaid.executor} does not match expected ${input.expectedExecutor}`,
    );
  }

  return {
    packetSent: true,
    sendLibrary: packetSent.sendLibrary,
    executor: feePaid.executor,
    executorFee: feePaid.fee,
  };
}

export function assertCanaryDestinationReceipt(
  input: DestinationCanaryStatusInput,
): DestinationCanaryStatus {
  if (hasLzReceiveAlert(input.logs, input.endpoint, input.endpointAbi)) {
    throw new Error("destination receipt contains EndpointV2 LzReceiveAlert");
  }
  if (!hasPacketDelivered(input.logs, input.endpoint, input.endpointAbi)) {
    throw new Error("destination receipt is missing EndpointV2 PacketDelivered");
  }
  return { packetDelivered: true };
}

export function assertCanaryRecipientBalance(
  status: CanaryRecipientBalanceStatus,
): CanaryRecipientBalanceStatus {
  if (status.balance < status.minBalance) {
    throw new Error(
      `recipient ${status.recipient} TestOFT balance ${status.balance} is below expected minimum ${status.minBalance}`,
    );
  }
  return status;
}

function findPacketSent(
  logs: readonly CanaryLog[],
  endpoint: Address,
  endpointAbi: Abi,
): { sendLibrary: Address } | undefined {
  for (const log of logs) {
    if (!isAddressEqual(log.address, endpoint)) {
      continue;
    }
    if (log.topics[0] !== eventTopic(endpointAbi, "PacketSent")) {
      continue;
    }
    try {
      const decoded = decodeEventLog({
        abi: endpointAbi,
        data: log.data,
        topics: mutableTopics(log.topics),
        eventName: "PacketSent",
      });
      const args = decoded.args as unknown as { sendLibrary: Address };
      return { sendLibrary: getAddress(args.sendLibrary) };
    } catch {
      continue;
    }
  }
  return undefined;
}

function findExecutorFeePaid(
  logs: readonly CanaryLog[],
  sendLib: Address,
  sendLibAbi: Abi,
): { executor: Address; fee: bigint } | undefined {
  for (const log of logs) {
    if (!isAddressEqual(log.address, sendLib)) {
      continue;
    }
    if (log.topics[0] !== eventTopic(sendLibAbi, "ExecutorFeePaid")) {
      continue;
    }
    try {
      const decoded = decodeEventLog({
        abi: sendLibAbi,
        data: log.data,
        topics: mutableTopics(log.topics),
        eventName: "ExecutorFeePaid",
      });
      const args = decoded.args as unknown as { executor: Address; fee: bigint };
      return { executor: getAddress(args.executor), fee: args.fee };
    } catch {
      continue;
    }
  }
  return undefined;
}

function hasPacketDelivered(
  logs: readonly CanaryLog[],
  endpoint: Address,
  endpointAbi: Abi,
): boolean {
  return logs.some((log) => {
    if (!isAddressEqual(log.address, endpoint)) {
      return false;
    }
    if (log.topics[0] !== eventTopic(endpointAbi, "PacketDelivered")) {
      return false;
    }
    try {
      decodeEventLog({
        abi: endpointAbi,
        data: log.data,
        topics: mutableTopics(log.topics),
        eventName: "PacketDelivered",
      });
      return true;
    } catch {
      return false;
    }
  });
}

function hasLzReceiveAlert(
  logs: readonly CanaryLog[],
  endpoint: Address,
  endpointAbi: Abi,
): boolean {
  return logs.some((log) => {
    if (!isAddressEqual(log.address, endpoint)) {
      return false;
    }
    if (log.topics[0] !== eventTopic(endpointAbi, "LzReceiveAlert")) {
      return false;
    }
    try {
      decodeEventLog({
        abi: endpointAbi,
        data: log.data,
        topics: mutableTopics(log.topics),
        eventName: "LzReceiveAlert",
      });
      return true;
    } catch {
      return false;
    }
  });
}

function eventTopic(abi: Abi, eventName: string): Hex {
  const [topic] = encodeEventTopics({ abi, eventName });
  if (topic === null || Array.isArray(topic)) {
    throw new Error(`${eventName} topic0 is not a single topic`);
  }
  return topic;
}

function mutableTopics(topics: readonly Hex[]): [Hex, ...Hex[]] {
  if (topics.length === 0) {
    throw new Error("event log is missing topic0");
  }
  return [...topics] as [Hex, ...Hex[]];
}
