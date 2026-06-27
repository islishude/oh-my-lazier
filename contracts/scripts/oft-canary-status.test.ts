import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import assert from "node:assert/strict";
import {
  encodeAbiParameters,
  encodeEventTopics,
  getAddress,
  type Abi,
  type Address,
  type Hex,
} from "viem";
import {
  assertCanaryDestinationReceipt,
  assertCanaryRecipientBalance,
  assertCanarySourceReceipt,
  type CanaryLog,
} from "./oft-canary-status.js";

const endpointAbi = loadAbi(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);
const sendLibAbi = loadAbi(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/SendLibBase.sol/SendLibBase.json",
);

const endpoint = getAddress("0x1111111111111111111111111111111111111111");
const sendLib = getAddress("0x2222222222222222222222222222222222222222");
const executor = getAddress("0x3333333333333333333333333333333333333333");

test("assertCanarySourceReceipt verifies PacketSent and ExecutorFeePaid executor", () => {
  const status = assertCanarySourceReceipt({
    logs: [
      packetSentLog({ endpoint, sendLibrary: sendLib }),
      executorFeePaidLog({ sendLib, executor, fee: 42n }),
    ],
    endpoint,
    sendLib,
    expectedExecutor: executor,
    endpointAbi,
    sendLibAbi,
  });

  assert.equal(status.packetSent, true);
  assert.equal(status.sendLibrary, sendLib);
  assert.equal(status.executor, executor);
  assert.equal(status.executorFee, 42n);
});

test("assertCanarySourceReceipt rejects unexpected executor", () => {
  assert.throws(
    () =>
      assertCanarySourceReceipt({
        logs: [
          packetSentLog({ endpoint, sendLibrary: sendLib }),
          executorFeePaidLog({
            sendLib,
            executor: getAddress("0x4444444444444444444444444444444444444444"),
            fee: 42n,
          }),
        ],
        endpoint,
        sendLib,
        expectedExecutor: executor,
        endpointAbi,
        sendLibAbi,
      }),
    /does not match expected/,
  );
});

test("assertCanaryDestinationReceipt verifies delivery and rejects alerts", () => {
  assert.deepEqual(
    assertCanaryDestinationReceipt({
      logs: [packetDeliveredLog({ endpoint })],
      endpoint,
      endpointAbi,
    }),
    { packetDelivered: true },
  );

  assert.throws(
    () =>
      assertCanaryDestinationReceipt({
        logs: [
          packetDeliveredLog({ endpoint }),
          lzReceiveAlertLog({ endpoint }),
        ],
        endpoint,
        endpointAbi,
      }),
    /LzReceiveAlert/,
  );
});

test("assertCanaryRecipientBalance verifies minimum destination balance", () => {
  const recipient = getAddress("0x8888888888888888888888888888888888888888");
  assert.deepEqual(
    assertCanaryRecipientBalance({
      recipient,
      balance: 10n,
      minBalance: 10n,
    }),
    { recipient, balance: 10n, minBalance: 10n },
  );

  assert.throws(
    () =>
      assertCanaryRecipientBalance({
        recipient,
        balance: 9n,
        minBalance: 10n,
      }),
    /below expected minimum/,
  );
});

function loadAbi(relativePath: string): Abi {
  return JSON.parse(readFileSync(join(process.cwd(), relativePath), "utf8"))
    .abi as Abi;
}

function packetSentLog(input: {
  endpoint: Address;
  sendLibrary: Address;
}): CanaryLog {
  return {
    address: input.endpoint,
    topics: encodeEventTopics({
      abi: endpointAbi,
      eventName: "PacketSent",
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [{ type: "bytes" }, { type: "bytes" }, { type: "address" }],
      ["0x010203", "0x", input.sendLibrary],
    ),
  };
}

function executorFeePaidLog(input: {
  sendLib: Address;
  executor: Address;
  fee: bigint;
}): CanaryLog {
  return {
    address: input.sendLib,
    topics: encodeEventTopics({
      abi: sendLibAbi,
      eventName: "ExecutorFeePaid",
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [{ type: "address" }, { type: "uint256" }],
      [input.executor, input.fee],
    ),
  };
}

function packetDeliveredLog(input: { endpoint: Address }): CanaryLog {
  return {
    address: input.endpoint,
    topics: encodeEventTopics({
      abi: endpointAbi,
      eventName: "PacketDelivered",
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [
        {
          type: "tuple",
          components: [
            { name: "srcEid", type: "uint32" },
            { name: "sender", type: "bytes32" },
            { name: "nonce", type: "uint64" },
          ],
        },
        { type: "address" },
      ],
      [
        {
          srcEid: 40161,
          sender:
            "0x0000000000000000000000005555555555555555555555555555555555555555",
          nonce: 1n,
        },
        getAddress("0x6666666666666666666666666666666666666666"),
      ],
    ),
  };
}

function lzReceiveAlertLog(input: { endpoint: Address }): CanaryLog {
  const receiver = getAddress("0x6666666666666666666666666666666666666666");
  return {
    address: input.endpoint,
    topics: encodeEventTopics({
      abi: endpointAbi,
      eventName: "LzReceiveAlert",
      args: { receiver, executor },
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [
        {
          type: "tuple",
          components: [
            { name: "srcEid", type: "uint32" },
            { name: "sender", type: "bytes32" },
            { name: "nonce", type: "uint64" },
          ],
        },
        { type: "bytes32" },
        { type: "uint256" },
        { type: "uint256" },
        { type: "bytes" },
        { type: "bytes" },
        { type: "bytes" },
      ],
      [
        {
          srcEid: 40161,
          sender:
            "0x0000000000000000000000005555555555555555555555555555555555555555",
          nonce: 1n,
        },
        "0x7777777777777777777777777777777777777777777777777777777777777777",
        200000n,
        0n,
        "0x",
        "0x",
        "0x1234",
      ],
    ),
  };
}
