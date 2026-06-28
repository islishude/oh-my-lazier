import {
  decodeEventLog,
  encodeEventTopics,
  getAddress,
  isAddressEqual,
} from "viem";
import type { Abi, Address, Hex } from "viem";

export type VerificationLog = {
  address: Address;
  topics: readonly Hex[];
  data: Hex;
};

export type DVNVerificationStatusInput = {
  logs: readonly VerificationLog[];
  receiveUln: Address;
  requiredDVNs: readonly Address[];
  minConfirmations: bigint;
  receiveUlnAbi: Abi;
  endpoint?: Address;
  endpointAbi?: Abi;
  expectedPayloadHash?: Hex;
  expectedPacket?: ExpectedPacketHeader;
};

export type DVNVerificationStatus = {
  payloadVerified: Array<{
    dvn: Address;
    confirmations: bigint;
    payloadHash: Hex;
  }>;
  packetVerified: boolean;
};

export type ExpectedPacketHeader = {
  srcEid?: number;
  dstEid?: number;
  nonce?: bigint;
  sender?: Address;
  receiver?: Address;
};

export function assertDVNVerificationReceipt(
  input: DVNVerificationStatusInput,
): DVNVerificationStatus {
  const verified = findPayloadVerified(input);
  const errors: string[] = [];
  for (const requiredDVN of input.requiredDVNs) {
    const match = verified.find((item) =>
      isAddressEqual(item.dvn, requiredDVN),
    );
    if (match === undefined) {
      errors.push(`missing ReceiveUln302 PayloadVerified for ${requiredDVN}`);
      continue;
    }
    if (match.confirmations < input.minConfirmations) {
      errors.push(
        `PayloadVerified confirmations ${match.confirmations} for ${requiredDVN} below ${input.minConfirmations}`,
      );
    }
  }
  const packetVerified =
    input.endpoint === undefined || input.endpointAbi === undefined
      ? false
      : hasPacketVerified(input.logs, input.endpoint, input.endpointAbi);
  if ((input.endpoint === undefined) !== (input.endpointAbi === undefined)) {
    errors.push("endpoint and endpointAbi must be provided together");
  }
  if (input.endpoint !== undefined && !packetVerified) {
    errors.push("receipt is missing EndpointV2 PacketVerified");
  }
  if (errors.length > 0) {
    throw new Error(errors.join("; "));
  }
  return { payloadVerified: verified, packetVerified };
}

function findPayloadVerified(
  input: DVNVerificationStatusInput,
): DVNVerificationStatus["payloadVerified"] {
  const topic = eventTopic(input.receiveUlnAbi, "PayloadVerified");
  const out: DVNVerificationStatus["payloadVerified"] = [];
  for (const log of input.logs) {
    if (!isAddressEqual(log.address, input.receiveUln)) {
      continue;
    }
    if (log.topics[0] !== topic) {
      continue;
    }
    try {
      const decoded = decodeEventLog({
        abi: input.receiveUlnAbi,
        data: log.data,
        topics: mutableTopics(log.topics),
        eventName: "PayloadVerified",
      });
      const args = decoded.args as unknown as {
        dvn: Address;
        header: Hex;
        confirmations: bigint;
        proofHash: Hex;
      };
      if (
        input.expectedPayloadHash !== undefined &&
        args.proofHash.toLowerCase() !== input.expectedPayloadHash.toLowerCase()
      ) {
        continue;
      }
      if (
        input.expectedPacket !== undefined &&
        !packetHeaderMatches(args.header, input.expectedPacket)
      ) {
        continue;
      }
      out.push({
        dvn: getAddress(args.dvn),
        confirmations: args.confirmations,
        payloadHash: args.proofHash,
      });
    } catch {
      continue;
    }
  }
  return out;
}

function packetHeaderMatches(
  header: Hex,
  expected: ExpectedPacketHeader,
): boolean {
  const decoded = decodePacketHeader(header);
  if (decoded === undefined) {
    return false;
  }
  if (expected.srcEid !== undefined && decoded.srcEid !== expected.srcEid) {
    return false;
  }
  if (expected.dstEid !== undefined && decoded.dstEid !== expected.dstEid) {
    return false;
  }
  if (expected.nonce !== undefined && decoded.nonce !== expected.nonce) {
    return false;
  }
  if (
    expected.sender !== undefined &&
    !isAddressEqual(decoded.sender, expected.sender)
  ) {
    return false;
  }
  if (
    expected.receiver !== undefined &&
    !isAddressEqual(decoded.receiver, expected.receiver)
  ) {
    return false;
  }
  return true;
}

function decodePacketHeader(header: Hex):
  | {
      nonce: bigint;
      srcEid: number;
      sender: Address;
      dstEid: number;
      receiver: Address;
    }
  | undefined {
  if (!/^0x[0-9a-fA-F]*$/.test(header) || header.length !== 164) {
    return undefined;
  }
  if (byteAt(header, 0) !== 1) {
    return undefined;
  }
  return {
    nonce: BigInt(`0x${hexSlice(header, 1, 9)}`),
    srcEid: Number.parseInt(hexSlice(header, 9, 13), 16),
    sender: getAddress(`0x${hexSlice(header, 25, 45)}`),
    dstEid: Number.parseInt(hexSlice(header, 45, 49), 16),
    receiver: getAddress(`0x${hexSlice(header, 61, 81)}`),
  };
}

function byteAt(hex: Hex, offset: number): number {
  return Number.parseInt(hex.slice(2 + offset * 2, 4 + offset * 2), 16);
}

function hexSlice(hex: Hex, start: number, end: number): string {
  return hex.slice(2 + start * 2, 2 + end * 2);
}

function hasPacketVerified(
  logs: readonly VerificationLog[],
  endpoint: Address,
  endpointAbi: Abi,
): boolean {
  const topic = eventTopic(endpointAbi, "PacketVerified");
  return logs.some((log) => {
    if (!isAddressEqual(log.address, endpoint)) {
      return false;
    }
    if (log.topics[0] !== topic) {
      return false;
    }
    try {
      decodeEventLog({
        abi: endpointAbi,
        data: log.data,
        topics: mutableTopics(log.topics),
        eventName: "PacketVerified",
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
