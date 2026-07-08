import {
  decodeEventLog,
  encodeEventTopics,
  isAddressEqual,
  keccak256,
  type Abi,
  type Address,
  type Hex,
  type TransactionReceipt,
} from "viem";

export type PacketDetails = {
  guid: Hex;
  nonce: bigint;
  packetHeader: Hex;
  payloadHash: Hex;
  sourceTxHash: Hex;
  srcLogIndex: number;
  sourceReceipt: TransactionReceipt;
};

export type MultiSendIndexerEvidence = {
  srcEid: number;
  dstEid: number;
  sourceTxHash: Hex;
  expectedPackets: {
    guid: Hex;
    nonce: bigint;
    srcLogIndex: number;
  }[];
};

export function packetsFromSourceReceipt(input: {
  receipt: TransactionReceipt;
  endpoint: Address;
  endpointAbi: Abi;
}): PacketDetails[] {
  const packetSentTopic = eventTopic(input.endpointAbi, "PacketSent");
  const packets: PacketDetails[] = [];
  for (const log of input.receipt.logs) {
    if (!isAddressEqual(log.address, input.endpoint)) {
      continue;
    }
    if (log.topics[0] !== packetSentTopic) {
      continue;
    }
    const decoded = decodeEventLog({
      abi: input.endpointAbi,
      eventName: "PacketSent",
      data: log.data,
      topics: mutableTopics(log.topics),
    });
    const args = decoded.args as unknown as { encodedPayload: Hex };
    const encodedPayload = args.encodedPayload;
    if ((encodedPayload.length - 2) / 2 < 113) {
      throw new Error("PacketSent encodedPayload is shorter than PacketV1");
    }
    if (log.logIndex === null) {
      throw new Error("PacketSent log is missing logIndex");
    }
    const packetHeader = sliceHex(encodedPayload, 0, 81);
    const payloadHash = keccak256(sliceHex(encodedPayload, 81));
    packets.push({
      guid: sliceHex(encodedPayload, 81, 113),
      nonce: BigInt(sliceHex(packetHeader, 1, 9)),
      packetHeader,
      payloadHash,
      sourceTxHash: input.receipt.transactionHash,
      srcLogIndex: Number(log.logIndex),
      sourceReceipt: input.receipt,
    });
  }
  packets.sort((a, b) => a.srcLogIndex - b.srcLogIndex);
  return packets;
}

export function requirePacketCount(
  packets: readonly PacketDetails[],
  expected: number,
  label: string,
): PacketDetails[] {
  if (packets.length !== expected) {
    throw new Error(`${label} decoded ${packets.length} PacketSent logs, want ${expected}`);
  }
  return [...packets];
}

export function multiSendIndexerEvidence(input: {
  srcEid: number;
  dstEid: number;
  packets: readonly PacketDetails[];
}): MultiSendIndexerEvidence {
  const [first] = input.packets;
  if (first === undefined) {
    throw new Error("multi-send indexer evidence requires at least one packet");
  }
  for (const packet of input.packets) {
    if (packet.sourceTxHash !== first.sourceTxHash) {
      throw new Error("multi-send packets must share one source transaction hash");
    }
  }
  return {
    srcEid: input.srcEid,
    dstEid: input.dstEid,
    sourceTxHash: first.sourceTxHash,
    expectedPackets: input.packets.map((packet) => ({
      guid: packet.guid,
      nonce: packet.nonce,
      srcLogIndex: packet.srcLogIndex,
    })),
  };
}

function eventTopic(abi: Abi, eventName: string): Hex {
  const topic = encodeEventTopics({ abi, eventName })[0];
  if (topic === null || Array.isArray(topic)) {
    throw new Error(`event ${eventName} did not produce a single topic`);
  }
  return topic;
}

function sliceHex(value: Hex, start: number, end?: number): Hex {
  const body = value.slice(2);
  return `0x${body.slice(start * 2, end === undefined ? undefined : end * 2)}` as Hex;
}

function mutableTopics(topics: readonly Hex[]): [Hex, ...Hex[]] {
  if (topics.length === 0) {
    throw new Error("log is missing topics");
  }
  return [...topics] as [Hex, ...Hex[]];
}
