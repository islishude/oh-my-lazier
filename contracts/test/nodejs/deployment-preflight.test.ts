import assert from "node:assert/strict";
import test from "node:test";
import {
  validateDeploymentPreflight,
  type DeploymentPreflightReport,
} from "../../scripts/deployment-preflight.js";

const expectedOwner = "0x1111111111111111111111111111111111111111";

test("validateDeploymentPreflight accepts matching owners and balances", () => {
  assert.deepEqual(validateDeploymentPreflight(baseReport()), []);
});

test("validateDeploymentPreflight reports owner mismatches", () => {
  const report = baseReport();
  report.contracts[1] = {
    ...report.contracts[1],
    owner: "0x2222222222222222222222222222222222222222",
  };

  const errors = validateDeploymentPreflight(report);

  assert.equal(errors.length, 1);
  assert.match(errors[0], /OpenExecutor owner/);
});

test("validateDeploymentPreflight reports insufficient canary balances", () => {
  const report = baseReport();
  report.canaryTreasury = {
    nativeBalance: {
      address: "0x3333333333333333333333333333333333333333",
      balance: 1n,
      minBalance: 2n,
    },
    tokenBalance: {
      address: "0x3333333333333333333333333333333333333333",
      balance: 3n,
      minBalance: 4n,
    },
  };

  const errors = validateDeploymentPreflight(report);

  assert.equal(errors.length, 2);
  assert.match(errors[0], /CANARY_TREASURY native/);
  assert.match(errors[1], /CANARY_TREASURY TestOFT/);
});

test("validateDeploymentPreflight reports total supply mismatch", () => {
  const report = baseReport();
  report.testOFTTotalSupply = { actual: 9n, expected: 10n };

  const errors = validateDeploymentPreflight(report);

  assert.equal(errors.length, 1);
  assert.match(errors[0], /totalSupply/);
});

function baseReport(): DeploymentPreflightReport {
  return {
    chainId: 11155111,
    expectedOwner,
    contracts: [
      {
        label: "TestOFT",
        address: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        owner: expectedOwner,
      },
      {
        label: "OpenExecutor",
        address: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        owner: expectedOwner,
      },
      {
        label: "OpenDVN",
        address: "0xcccccccccccccccccccccccccccccccccccccccc",
        owner: expectedOwner,
      },
    ],
    ownerNativeBalance: {
      address: expectedOwner,
      balance: 10n,
      minBalance: 10n,
    },
  };
}
