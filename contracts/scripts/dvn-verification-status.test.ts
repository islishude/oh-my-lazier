import { readFileSync } from "node:fs";
import { join } from "node:path";
import assert from "node:assert/strict";
import test from "node:test";
import {
  encodeAbiParameters,
  encodeEventTopics,
  getAddress,
  type Abi,
  type Address,
  type Hex,
} from "viem";
import {
  assertDVNVerificationReceipt,
  type VerificationLog,
} from "./dvn-verification-status.js";

const receiveUlnAbi = loadAbi(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
);
const endpointAbi = loadAbi(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);

const receiveUln = getAddress("0x1111111111111111111111111111111111111111");
const endpoint = getAddress("0x2222222222222222222222222222222222222222");
const openDVN = getAddress("0x3333333333333333333333333333333333333333");
const layerZeroLabsDVN = getAddress(
  "0x4444444444444444444444444444444444444444",
);
const payloadHash =
  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

test("assertDVNVerificationReceipt verifies both required DVNs and Endpoint PacketVerified", () => {
  const status = assertDVNVerificationReceipt({
    logs: [
      payloadVerifiedLog({ dvn: openDVN, confirmations: 12n }),
      payloadVerifiedLog({ dvn: layerZeroLabsDVN, confirmations: 12n }),
      packetVerifiedLog(),
    ],
    receiveUln,
    requiredDVNs: [openDVN, layerZeroLabsDVN],
    minConfirmations: 12n,
    receiveUlnAbi,
    endpoint,
    endpointAbi,
    expectedPayloadHash: payloadHash,
  });

  assert.equal(status.payloadVerified.length, 2);
  assert.equal(status.packetVerified, true);
});

test("assertDVNVerificationReceipt rejects missing required DVN", () => {
  assert.throws(
    () =>
      assertDVNVerificationReceipt({
        logs: [payloadVerifiedLog({ dvn: openDVN, confirmations: 12n })],
        receiveUln,
        requiredDVNs: [openDVN, layerZeroLabsDVN],
        minConfirmations: 12n,
        receiveUlnAbi,
      }),
    /missing ReceiveUln302 PayloadVerified/,
  );
});

test("assertDVNVerificationReceipt rejects insufficient confirmations", () => {
  assert.throws(
    () =>
      assertDVNVerificationReceipt({
        logs: [
          payloadVerifiedLog({ dvn: openDVN, confirmations: 11n }),
          payloadVerifiedLog({ dvn: layerZeroLabsDVN, confirmations: 12n }),
        ],
        receiveUln,
        requiredDVNs: [openDVN, layerZeroLabsDVN],
        minConfirmations: 12n,
        receiveUlnAbi,
      }),
    /below 12/,
  );
});

test("assertDVNVerificationReceipt rejects missing PacketVerified when endpoint is required", () => {
  assert.throws(
    () =>
      assertDVNVerificationReceipt({
        logs: [
          payloadVerifiedLog({ dvn: openDVN, confirmations: 12n }),
          payloadVerifiedLog({ dvn: layerZeroLabsDVN, confirmations: 12n }),
        ],
        receiveUln,
        requiredDVNs: [openDVN, layerZeroLabsDVN],
        minConfirmations: 12n,
        receiveUlnAbi,
        endpoint,
        endpointAbi,
      }),
    /PacketVerified/,
  );
});

function loadAbi(relativePath: string): Abi {
  return JSON.parse(readFileSync(join(process.cwd(), relativePath), "utf8"))
    .abi as Abi;
}

function payloadVerifiedLog(input: {
  dvn: Address;
  confirmations: bigint;
}): VerificationLog {
  return {
    address: receiveUln,
    topics: encodeEventTopics({
      abi: receiveUlnAbi,
      eventName: "PayloadVerified",
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [
        { type: "address" },
        { type: "bytes" },
        { type: "uint256" },
        { type: "bytes32" },
      ],
      [input.dvn, "0x01020304", input.confirmations, payloadHash],
    ),
  };
}

function packetVerifiedLog(): VerificationLog {
  return {
    address: endpoint,
    topics: encodeEventTopics({
      abi: endpointAbi,
      eventName: "PacketVerified",
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
        { type: "bytes32" },
      ],
      [
        {
          srcEid: 40161,
          sender:
            "0x0000000000000000000000005555555555555555555555555555555555555555",
          nonce: 1n,
        },
        getAddress("0x6666666666666666666666666666666666666666"),
        payloadHash,
      ],
    ),
  };
}
