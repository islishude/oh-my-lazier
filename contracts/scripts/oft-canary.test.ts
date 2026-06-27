import assert from "node:assert/strict";
import test from "node:test";
import { buildCanarySendParam, buildLzReceiveOption } from "./oft-canary.js";

test("buildLzReceiveOption encodes one zero-value executor lzReceive option", () => {
  assert.equal(
    buildLzReceiveOption(100_000n),
    "0x000301002101000000000000000000000000000186a000000000000000000000000000000000",
  );
});

test("buildLzReceiveOption rejects zero gas", () => {
  assert.throws(() => buildLzReceiveOption(0n), /non-zero uint128/);
});

test("buildCanarySendParam builds first-phase OFT send params", () => {
  const sendParam = buildCanarySendParam({
    dstEid: 40245,
    recipient: "0x1111111111111111111111111111111111111111",
    amountLD: 1_000_000n,
    minAmountLD: 999_000n,
    lzReceiveGas: 200_000n,
  });

  assert.deepEqual(sendParam, {
    dstEid: 40245,
    to: "0x0000000000000000000000001111111111111111111111111111111111111111",
    amountLD: 1_000_000n,
    minAmountLD: 999_000n,
    extraOptions:
      "0x00030100210100000000000000000000000000030d4000000000000000000000000000000000",
    composeMsg: "0x",
    oftCmd: "0x",
  });
});

test("buildCanarySendParam rejects slippage above amount", () => {
  assert.throws(
    () =>
      buildCanarySendParam({
        dstEid: 40245,
        recipient: "0x1111111111111111111111111111111111111111",
        amountLD: 1_000n,
        minAmountLD: 1_001n,
        lzReceiveGas: 200_000n,
      }),
    /minAmountLD must be positive and no greater than amountLD/,
  );
});
