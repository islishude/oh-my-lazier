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
} from "viem";
import {
  sourceWorkerFeeClaims,
  type FeeEventLog,
} from "./e2e-fee-withdrawal.js";

const sendLibAbi = loadAbi(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json",
);

const sendLib = getAddress("0x1111111111111111111111111111111111111111");
const openExecutor = getAddress("0x2222222222222222222222222222222222222222");
const primaryOpenDVN = getAddress("0x3333333333333333333333333333333333333333");
const secondaryOpenDVN = getAddress("0x4444444444444444444444444444444444444444");

test("sourceWorkerFeeClaims maps executor and DVN fees by worker address", () => {
  const claims = sourceWorkerFeeClaims({
    sourceName: "local-a",
    logs: [
      dvnFeePaidLog({
        requiredDVNs: [secondaryOpenDVN, primaryOpenDVN],
        fees: [31n, 17n],
      }),
    ],
    sendLib,
    sendLibAbi,
    openExecutor,
    primaryOpenDVN,
    secondaryOpenDVN,
    executorFee: 5n,
  });

  assert.deepEqual(claims, [
    { role: "open_executor", worker: openExecutor, amount: 5n },
    { role: "primary_open_dvn", worker: primaryOpenDVN, amount: 17n },
    { role: "secondary_open_dvn", worker: secondaryOpenDVN, amount: 31n },
  ]);
});

test("sourceWorkerFeeClaims rejects missing source DVN fee entries", () => {
  assert.throws(
    () =>
      sourceWorkerFeeClaims({
        sourceName: "local-a",
        logs: [
          dvnFeePaidLog({
            requiredDVNs: [primaryOpenDVN],
            fees: [17n],
          }),
        ],
        sendLib,
        sendLibAbi,
        openExecutor,
        primaryOpenDVN,
        secondaryOpenDVN,
        executorFee: 5n,
      }),
    /missing source DVN secondary_open_dvn/,
  );
});

test("sourceWorkerFeeClaims rejects zero fee claims", () => {
  assert.throws(
    () =>
      sourceWorkerFeeClaims({
        sourceName: "local-a",
        logs: [
          dvnFeePaidLog({
            requiredDVNs: [primaryOpenDVN, secondaryOpenDVN],
            fees: [0n, 17n],
          }),
        ],
        sendLib,
        sendLibAbi,
        openExecutor,
        primaryOpenDVN,
        secondaryOpenDVN,
        executorFee: 5n,
      }),
    /primary_open_dvn fee must be positive/,
  );
});

function loadAbi(relativePath: string): Abi {
  return JSON.parse(readFileSync(join(process.cwd(), relativePath), "utf8"))
    .abi as Abi;
}

function dvnFeePaidLog(input: {
  requiredDVNs: Address[];
  fees: bigint[];
}): FeeEventLog {
  return {
    address: sendLib,
    topics: encodeEventTopics({
      abi: sendLibAbi,
      eventName: "DVNFeePaid",
    }) as readonly Hex[],
    data: encodeAbiParameters(
      [{ type: "address[]" }, { type: "address[]" }, { type: "uint256[]" }],
      [input.requiredDVNs, [], input.fees],
    ),
  };
}
