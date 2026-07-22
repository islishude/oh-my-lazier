import assert from "node:assert/strict";
import test from "node:test";
import {
  validatePriceConfigReport,
  type PriceConfigReport,
} from "../../scripts/price-config-check.js";

test("validatePriceConfigReport accepts fresh executor and DVN configs", () => {
  assert.deepEqual(validatePriceConfigReport(baseReport()), []);
});

test("validatePriceConfigReport rejects stale and mismatched configs", () => {
  const report = baseReport();
  report.priceFeed.priceSnapshot = {
    ...report.priceFeed.priceSnapshot,
    dstGasPriceInSrcToken: 0n,
    updatedAt: 900n,
    staleAfter: 120n,
  };
  report.workers[0] = {
    ...report.workers[0],
    priceFeed: "0x9999999999999999999999999999999999999999",
    feeModel: {
      ...report.workers[0].feeModel,
      marginBps: 10_001,
    },
  };

  const errors = validatePriceConfigReport(report);

  assert.equal(errors.length, 5);
  assert.match(errors[0], /dstGasPriceInSrcToken/);
  assert.match(errors[1], /exceeds/);
  assert.match(errors[2], /staleAfter/);
  assert.match(errors[3], /priceFeed/);
  assert.match(errors[4], /marginBps/);
});

test("validatePriceConfigReport rejects future updatedAt", () => {
  const report = baseReport();
  report.priceFeed = {
    ...report.priceFeed,
    priceSnapshot: {
      ...report.priceFeed.priceSnapshot,
      updatedAt: 1001n,
    },
  };

  const errors = validatePriceConfigReport(report);

  assert.equal(errors.length, 1);
  assert.match(errors[0], /future/);
});

function baseReport(): PriceConfigReport {
  return {
    chainId: 11155111,
    dstEid: 40449,
    checkedAt: 1000n,
    maxAgeSeconds: 60n,
    expectedStaleAfter: 1800n,
    priceFeed: {
      address: "0x3333333333333333333333333333333333333333",
      priceSnapshot: {
        dstGasPriceInSrcToken: 2n,
        dstDataFeePerByteInSrcToken: 0n,
        updatedAt: 950n,
        staleAfter: 1800n,
      },
    },
    workers: [
      {
        label: "OpenExecutor",
        address: "0x1111111111111111111111111111111111111111",
        priceFeed: "0x3333333333333333333333333333333333333333",
        feeModel: {
          baseFee: 1n,
          dstGasOverhead: 50_000n,
          dataSizeOverheadBytes: 0n,
          marginBps: 100,
        },
      },
      {
        label: "OpenDVN",
        address: "0x2222222222222222222222222222222222222222",
        priceFeed: "0x3333333333333333333333333333333333333333",
        feeModel: {
          baseFee: 1n,
          dstGasOverhead: 150_000n,
          dataSizeOverheadBytes: 0n,
          marginBps: 100,
        },
      },
    ],
  };
}
