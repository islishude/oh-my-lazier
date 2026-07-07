import assert from "node:assert/strict";
import test from "node:test";
import type { Address } from "viem";
import { shouldSetPriceFeed } from "./worker-price-feed.js";

test("shouldSetPriceFeed skips matching configured feed", () => {
  assert.equal(
    shouldSetPriceFeed(
      "0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa",
      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" as Address,
    ),
    false,
  );
});

test("shouldSetPriceFeed rotates on mismatch", () => {
  assert.equal(
    shouldSetPriceFeed(
      "0x1111111111111111111111111111111111111111",
      "0x2222222222222222222222222222222222222222" as Address,
    ),
    true,
  );
});
