import assert from "node:assert/strict";
import test from "node:test";
import {
  validatePriceConfigReport,
  type PriceConfigReport,
} from "./price-config-check.js";

test("validatePriceConfigReport accepts fresh executor and DVN configs", () => {
  assert.deepEqual(validatePriceConfigReport(baseReport()), []);
});

test("validatePriceConfigReport rejects stale and mismatched configs", () => {
  const report = baseReport();
  report.workers[0] = {
    ...report.workers[0],
    priceConfig: {
      ...report.workers[0].priceConfig,
      dstGasPriceInSrcToken: 0n,
      updatedAt: 900n,
      staleAfter: 120n,
    },
  };

  const errors = validatePriceConfigReport(report);

  assert.equal(errors.length, 3);
  assert.match(errors[0], /dstGasPriceInSrcToken/);
  assert.match(errors[1], /exceeds/);
  assert.match(errors[2], /staleAfter/);
});

test("validatePriceConfigReport rejects future updatedAt", () => {
  const report = baseReport();
  report.workers[1] = {
    ...report.workers[1],
    priceConfig: { ...report.workers[1].priceConfig, updatedAt: 1001n },
  };

  const errors = validatePriceConfigReport(report);

  assert.equal(errors.length, 1);
  assert.match(errors[0], /future/);
});

function baseReport(): PriceConfigReport {
  return {
    chainId: 11155111,
    dstEid: 40245,
    checkedAt: 1000n,
    maxAgeSeconds: 60n,
    expectedStaleAfter: 1800n,
    workers: [
      {
        label: "OpenExecutor",
        address: "0x1111111111111111111111111111111111111111",
        priceConfig: {
          baseFee: 1n,
          dstGasPriceInSrcToken: 2n,
          bufferBps: 100,
          updatedAt: 950n,
          staleAfter: 1800n,
        },
      },
      {
        label: "OpenDVN",
        address: "0x2222222222222222222222222222222222222222",
        priceConfig: {
          baseFee: 1n,
          dstGasPriceInSrcToken: 2n,
          bufferBps: 100,
          updatedAt: 950n,
          staleAfter: 1800n,
        },
      },
    ],
  };
}
