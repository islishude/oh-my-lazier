import assert from "node:assert/strict";
import test from "node:test";
import { encodeErrorResult, type Abi } from "viem";
import {
  decodeKnownContractError,
  enrichKnownContractError,
  formatKnownContractError,
} from "../../scripts/contract-error.js";

const workerErrorsABI = [
  {
    type: "error",
    name: "PriceSnapshotStale",
    inputs: [
      { name: "dstEid", type: "uint32" },
      { name: "updatedAt", type: "uint256" },
      { name: "staleAfter", type: "uint256" },
    ],
  },
] as const satisfies Abi;

test("decodeKnownContractError decodes nested revert data", () => {
  const data = encodeErrorResult({
    abi: workerErrorsABI,
    errorName: "PriceSnapshotStale",
    args: [40449, 1_800_000_000n, 1_800n],
  });
  const error = new Error("contract call failed", {
    cause: { data },
  });

  const decoded = decodeKnownContractError(error, workerErrorsABI);

  assert.deepEqual(decoded, {
    name: "PriceSnapshotStale",
    signature: "PriceSnapshotStale(uint32,uint256,uint256)",
    selector: "0xd1cc11be",
    args: [40449, 1_800_000_000n, 1_800n],
  });
  assert.equal(
    formatKnownContractError(decoded),
    'PriceSnapshotStale(uint32,uint256,uint256) selector=0xd1cc11be args=[40449,"1800000000","1800"]'
  );
});

test("decodeKnownContractError maps selector-only viem messages", () => {
  const error = new Error(
    'Unable to decode signature "0xd1cc11be" as it was not found on the provided ABI.'
  );

  assert.deepEqual(decodeKnownContractError(error, workerErrorsABI), {
    name: "PriceSnapshotStale",
    signature: "PriceSnapshotStale(uint32,uint256,uint256)",
    selector: "0xd1cc11be",
  });
});

test("enrichKnownContractError adds operator hint for stale prices", () => {
  const data = encodeErrorResult({
    abi: workerErrorsABI,
    errorName: "PriceSnapshotStale",
    args: [40449, 1_800_000_000n, 1_800n],
  });

  const enriched = enrichKnownContractError({
    error: { data },
    abi: workerErrorsABI,
    context: "TestOFT.quoteSend",
  });

  assert.ok(enriched);
  assert.match(
    enriched.message,
    /TestOFT\.quoteSend reverted with PriceSnapshotStale/
  );
  assert.match(enriched.message, /Refresh the source OpenPriceFeed/);
});
