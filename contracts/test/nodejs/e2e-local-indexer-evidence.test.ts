import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import {
  encodeAbiParameters,
  encodeEventTopics,
  getAddress,
  type Abi,
  type Address,
  type Hex,
  type TransactionReceipt,
} from "viem";
import {
  destinationReplayEvidence,
  multiSendIndexerEvidence,
  packetsFromSourceReceipt,
  requirePacketCount,
  type DestinationReplayObservation,
} from "../../scripts/e2e-local-indexer-evidence.js";

const endpointAbi = loadAbi(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json"
);
const endpoint = getAddress("0x1111111111111111111111111111111111111111");
const sendLib = getAddress("0x2222222222222222222222222222222222222222");
const txHash =
  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" as Hex;

test("packetsFromSourceReceipt decodes and sorts PacketSent logs", () => {
  const secondGUID =
    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" as Hex;
  const firstGUID =
    "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" as Hex;
  const receipt = receiptWithLogs([
    packetSentLog({ guid: secondGUID, nonce: 8n, logIndex: 12 }),
    packetSentLog({ guid: firstGUID, nonce: 7n, logIndex: 9 }),
  ]);

  const packets = packetsFromSourceReceipt({
    receipt,
    endpoint,
    endpointAbi,
  });

  assert.equal(packets.length, 2);
  assert.equal(packets[0]?.guid, firstGUID);
  assert.equal(packets[0]?.nonce, 7n);
  assert.equal(packets[0]?.srcLogIndex, 9);
  assert.equal(packets[1]?.guid, secondGUID);
  assert.equal(packets[1]?.sourceTxHash, txHash);
});

test("requirePacketCount rejects unexpected PacketSent count", () => {
  assert.throws(
    () => requirePacketCount([], 2, "multi-send"),
    /multi-send decoded 0 PacketSent logs, want 2/
  );
});

test("multiSendIndexerEvidence requires one source transaction", () => {
  const receipt = receiptWithLogs([
    packetSentLog({
      guid: "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" as Hex,
      nonce: 1n,
      logIndex: 4,
    }),
    packetSentLog({
      guid: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" as Hex,
      nonce: 2n,
      logIndex: 7,
    }),
  ]);
  const packets = packetsFromSourceReceipt({ receipt, endpoint, endpointAbi });

  assert.deepEqual(
    multiSendIndexerEvidence({ srcEid: 90101, dstEid: 90102, packets }),
    {
      srcEid: 90101,
      dstEid: 90102,
      sourceTxHash: txHash,
      expectedPackets: [
        {
          guid: "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
          nonce: 1n,
          srcLogIndex: 4,
        },
        {
          guid: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
          nonce: 2n,
          srcLogIndex: 7,
        },
      ],
    }
  );
});

test("destinationReplayEvidence maps packet observations by guid", () => {
  const receipt = receiptWithLogs([
    packetSentLog({
      guid: "0x1111111111111111111111111111111111111111111111111111111111111111" as Hex,
      nonce: 11n,
      logIndex: 1,
    }),
    packetSentLog({
      guid: "0x2222222222222222222222222222222222222222222222222222222222222222" as Hex,
      nonce: 12n,
      logIndex: 2,
    }),
  ]);
  const packets = packetsFromSourceReceipt({ receipt, endpoint, endpointAbi });
  const observations: DestinationReplayObservation[] = [
    {
      guid: packets[1]!.guid,
      commitTxHash:
        "0x3333333333333333333333333333333333333333333333333333333333333333",
      receiveTxHash:
        "0x4444444444444444444444444444444444444444444444444444444444444444",
      verifyTxHash:
        "0x5555555555555555555555555555555555555555555555555555555555555555",
    },
    {
      guid: packets[0]!.guid,
      commitTxHash:
        "0x6666666666666666666666666666666666666666666666666666666666666666",
      receiveTxHash:
        "0x7777777777777777777777777777777777777777777777777777777777777777",
      verifyTxHash:
        "0x8888888888888888888888888888888888888888888888888888888888888888",
    },
  ];

  assert.deepEqual(
    destinationReplayEvidence({
      srcEid: 90101,
      dstEid: 90102,
      packets,
      observations,
    }),
    {
      srcEid: 90101,
      dstEid: 90102,
      sourceTxHash: txHash,
      expectedPackets: [
        {
          guid: packets[0]!.guid,
          nonce: 11n,
          srcLogIndex: 1,
          packetHeader: packets[0]!.packetHeader,
          payloadHash: packets[0]!.payloadHash,
          commitTxHash:
            "0x6666666666666666666666666666666666666666666666666666666666666666",
          receiveTxHash:
            "0x7777777777777777777777777777777777777777777777777777777777777777",
          verifyTxHash:
            "0x8888888888888888888888888888888888888888888888888888888888888888",
        },
        {
          guid: packets[1]!.guid,
          nonce: 12n,
          srcLogIndex: 2,
          packetHeader: packets[1]!.packetHeader,
          payloadHash: packets[1]!.payloadHash,
          commitTxHash:
            "0x3333333333333333333333333333333333333333333333333333333333333333",
          receiveTxHash:
            "0x4444444444444444444444444444444444444444444444444444444444444444",
          verifyTxHash:
            "0x5555555555555555555555555555555555555555555555555555555555555555",
        },
      ],
    }
  );
});

test("destinationReplayEvidence validates tx hashes and missing observations", () => {
  const receipt = receiptWithLogs([
    packetSentLog({
      guid: "0x9999999999999999999999999999999999999999999999999999999999999999" as Hex,
      nonce: 13n,
      logIndex: 3,
    }),
  ]);
  const packets = packetsFromSourceReceipt({ receipt, endpoint, endpointAbi });

  assert.throws(
    () =>
      destinationReplayEvidence({
        srcEid: 90101,
        dstEid: 90102,
        packets,
        observations: [
          {
            guid: packets[0]!.guid,
            commitTxHash: "0x1234" as Hex,
            receiveTxHash:
              "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            verifyTxHash:
              "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
          },
        ],
      }),
    /commitTxHash must be a 32-byte hex value/
  );
  assert.throws(
    () =>
      destinationReplayEvidence({
        srcEid: 90101,
        dstEid: 90102,
        packets,
        observations: [],
      }),
    /missing destination replay observation/
  );
});

function loadAbi(relativePath: string): Abi {
  return JSON.parse(readFileSync(join(process.cwd(), relativePath), "utf8"))
    .abi as Abi;
}

function receiptWithLogs(
  logs: ReturnType<typeof packetSentLog>[]
): TransactionReceipt {
  return {
    transactionHash: txHash,
    logs,
  } as unknown as TransactionReceipt;
}

function packetSentLog(input: { guid: Hex; nonce: bigint; logIndex: number }) {
  return {
    address: endpoint,
    topics: encodeEventTopics({
      abi: endpointAbi,
      eventName: "PacketSent",
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [{ type: "bytes" }, { type: "bytes" }, { type: "address" }],
      [encodedPacket(input), "0x", sendLib]
    ),
    transactionHash: txHash,
    logIndex: input.logIndex,
  };
}

function encodedPacket(input: { guid: Hex; nonce: bigint }): Hex {
  return `0x01${uint64(input.nonce)}${uint32(90101)}${addressToBytes32(
    getAddress("0x3333333333333333333333333333333333333333")
  )}${uint32(90102)}${addressToBytes32(
    getAddress("0x4444444444444444444444444444444444444444")
  )}${input.guid.slice(2)}68656c6c6f` as Hex;
}

function uint64(value: bigint): string {
  return value.toString(16).padStart(16, "0");
}

function uint32(value: number): string {
  return value.toString(16).padStart(8, "0");
}

function addressToBytes32(address: Address): string {
  return address.slice(2).padStart(64, "0");
}
