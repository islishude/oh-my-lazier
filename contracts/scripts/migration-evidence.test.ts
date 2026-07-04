import assert from "node:assert/strict";
import test from "node:test";
import {
  validateMigrationEvidenceRecord,
  type DVNVerificationEvidence,
  type EvidenceRef,
  type MigrationEvidenceRecord,
  type PriceConfigEvidence,
} from "./migration-evidence.js";

test("validateMigrationEvidenceRecord accepts complete migration evidence", () => {
  assert.deepEqual(validateMigrationEvidenceRecord(baseRecord()), []);
});

test("validateMigrationEvidenceRecord rejects price config evidence not bound to source workers", () => {
  const record = baseRecord();
  const evidence = record.directions[0].priceConfigCheck;
  const wrongPriceFeed = "0x9999999999999999999999999999999999999999";
  record.directions[0].priceConfigCheck = {
    ...evidence,
    priceFeed: {
      ...evidence.priceFeed,
      address: wrongPriceFeed,
    },
    executor: {
      ...evidence.executor,
      address: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      priceFeed: wrongPriceFeed,
    },
    dvn: {
      ...evidence.dvn,
      address: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      priceFeed: wrongPriceFeed,
    },
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[0].priceConfigCheck.priceFeed.address must equal sourceWorkers.priceFeed 0x4444444444444444444444444444444444444444",
    "directions[0].priceConfigCheck.executor.address must equal sourceWorkers.openExecutor 0x2222222222222222222222222222222222222222",
    "directions[0].priceConfigCheck.dvn.address must equal sourceWorkers.openDVN 0x3333333333333333333333333333333333333333",
  ]);
});

test("validateMigrationEvidenceRecord rejects missing required artifacts", () => {
  const record = baseRecord();
  record.operatorContacts = [];
  record.directions[0] = {
    ...record.directions[0],
    dstEid: record.directions[0].srcEid,
    canaryDestinationReceipt: evidence(""),
    canary: {
      amountLD: "0",
      senderAccount: "",
      recipientAccount: "",
      minRecipientBalanceLD: "not-a-number",
      sourceReceipt: evidence(""),
      destinationReceipt: evidence(""),
      recipientBalanceCheck: evidence(""),
    },
    dvnJoin: {
      confirmations: 0,
      requiredDVNs: ["OpenDVN"],
      optionalDVNsDisabled: false,
      configCheck: evidence(""),
    },
  };
  record.layerZeroAddressCheck = evidence("");
  record.readinessCheck = evidence("");
  record.runbookReview = evidence("");
  record.rollback.dryRun = evidence("");
  record.rollback.restoredConfigCheck = evidence("");
  record.rollback.canaryAfterRollback = evidence("");
  record.rollback.manualRetryPlan = evidence("");

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "operatorContacts must contain at least one value",
    "layerZeroAddressCheck.ref must be a non-empty string",
    "readinessCheck.ref must be a non-empty string",
    "runbookReview.ref must be a non-empty string",
    "directions[0].srcEid and directions[0].dstEid must differ",
    "directions[0].priceConfigCheck.dstEid must equal direction dstEid 40161",
    "directions[0].canaryDestinationReceipt.ref must be a non-empty string",
    "directions[0].canary.amountLD must be a positive decimal integer string",
    "directions[0].canary.senderAccount must be an EVM address",
    "directions[0].canary.recipientAccount must be an EVM address",
    "directions[0].canary.minRecipientBalanceLD must be a positive decimal integer string",
    "directions[0].canary.sourceReceipt.ref must be a non-empty string",
    "directions[0].canary.destinationReceipt.ref must be a non-empty string",
    "directions[0].canary.recipientBalanceCheck.ref must be a non-empty string",
    "directions[0].dvnJoin.confirmations must be a positive integer",
    "directions[0].dvnJoin.requiredDVNs must include layerzero labs dvn",
    "directions[0].dvnJoin.requiredDVNs must not be self-only",
    "directions[0].dvnJoin.optionalDVNsDisabled must be true",
    "directions[0].dvnJoin.configCheck.ref must be a non-empty string",
    "directions[0].dvnVerificationReceipt.expectedDstEid must equal direction dstEid 40161",
    "directions contains unsupported phase-1 direction 40161->40161",
    "directions missing reciprocal direction 40161->40449",
    "directions missing phase-1 direction 40161->40449",
    "rollback.dryRun.ref must be a non-empty string",
    "rollback.restoredConfigCheck.ref must be a non-empty string",
    "rollback.canaryAfterRollback.ref must be a non-empty string",
    "rollback.manualRetryPlan.ref must be a non-empty string",
  ]);
});

test("validateMigrationEvidenceRecord rejects zero and invalid account addresses", () => {
  const record = baseRecord();
  record.ownerAccount = "0x0000000000000000000000000000000000000000";
  record.signerAccount = "not-an-address";
  record.directions[0].sourceWorkers.openExecutor =
    "0x0000000000000000000000000000000000000000";
  record.directions[0].sourceWorkers.openDVN = "not-an-address";
  record.directions[0].sourceWorkers.priceFeed =
    "0x0000000000000000000000000000000000000000";
  record.directions[1].destinationWorkers.openDVN = "0xabc";
  record.directions[0].canary.senderAccount =
    "0x0000000000000000000000000000000000000000";
  record.directions[1].canary.recipientAccount = "0xabc";
  record.rollback.ownerPauseAccount =
    "0x0000000000000000000000000000000000000000";
  record.rollback.signerAccount = "";

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "ownerAccount must not be the zero address",
    "signerAccount must be an EVM address",
    "directions[0].sourceWorkers.openExecutor must not be the zero address",
    "directions[0].sourceWorkers.openDVN must be an EVM address",
    "directions[0].sourceWorkers.priceFeed must not be the zero address",
    "directions[0].canary.senderAccount must not be the zero address",
    "directions[1].destinationWorkers.openDVN must be an EVM address",
    "directions[1].canary.recipientAccount must be an EVM address",
    "rollback.ownerPauseAccount must not be the zero address",
    "rollback.signerAccount must be an EVM address",
  ]);
});

test("validateMigrationEvidenceRecord rejects duplicate directions", () => {
  const record = baseRecord();
  record.directions.push({ ...record.directions[0], label: "duplicate" });

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, ["directions[2] duplicates direction 40161->40449"]);
});

test("validateMigrationEvidenceRecord rejects missing reciprocal direction", () => {
  const record = baseRecord();
  record.directions = [record.directions[0]];

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions missing reciprocal direction 40449->40161",
    "directions missing phase-1 direction 40449->40161",
  ]);
});

test("validateMigrationEvidenceRecord rejects unsupported phase-1 directions", () => {
  const record = baseRecord();
  record.directions[1] = {
    ...record.directions[1],
    srcEid: 40449,
    dstEid: 99999,
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[1].priceConfigCheck.dstEid must equal direction dstEid 99999",
    "directions[1].dvnVerificationReceipt.expectedDstEid must equal direction dstEid 99999",
    "directions missing reciprocal direction 40449->40161",
    "directions contains unsupported phase-1 direction 40449->99999",
    "directions missing reciprocal direction 99999->40449",
    "directions missing phase-1 direction 40449->40161",
  ]);
});

test("validateMigrationEvidenceRecord rejects weak DVN verification packet identity", () => {
  const record = baseRecord();
  record.directions[0].dvnVerificationReceipt = {
    ...record.directions[0].dvnVerificationReceipt,
    expectedPayloadHash: "0xabc",
    expectedSrcEid: 40449,
    expectedDstEid: 40161,
    expectedNonce: "0",
    expectedSender: "0x0000000000000000000000000000000000000000",
    expectedReceiver: "not-an-address",
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[0].dvnVerificationReceipt.expectedPayloadHash must be a bytes32 hex string",
    "directions[0].dvnVerificationReceipt.expectedSrcEid must equal direction srcEid 40161",
    "directions[0].dvnVerificationReceipt.expectedDstEid must equal direction dstEid 40449",
    "directions[0].dvnVerificationReceipt.expectedNonce must be a positive decimal integer string",
    "directions[0].dvnVerificationReceipt.expectedSender must not be the zero address",
    "directions[0].dvnVerificationReceipt.expectedReceiver must be an EVM address",
  ]);
});

test("validateMigrationEvidenceRecord rejects stale or mismatched price snapshot evidence", () => {
  const record = baseRecord();
  record.directions[0].priceConfigCheck = {
    ...record.directions[0].priceConfigCheck,
    dstEid: 40161,
    checkedAt: "1000",
    maxAgeSeconds: "60",
    expectedStaleAfter: "1800",
    priceFeed: {
      address: "0x4444444444444444444444444444444444444444",
      priceSnapshot: {
        updatedAt: "900",
        staleAfter: "120",
        dstGasPriceInSrcToken: "0",
      },
    },
    executor: {
      address: "0x2222222222222222222222222222222222222222",
      priceFeed: "0x9999999999999999999999999999999999999999",
      baseFee: "-1",
      dstGasOverhead: "-1",
      marginBps: 10001,
    },
    dvn: {
      address: "0x3333333333333333333333333333333333333333",
      priceFeed: "0x4444444444444444444444444444444444444444",
      baseFee: "0",
      dstGasOverhead: "150000",
      marginBps: 100,
    },
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[0].priceConfigCheck.dstEid must equal direction dstEid 40449",
    "directions[0].priceConfigCheck.priceFeed.priceSnapshot.dstGasPriceInSrcToken must be a positive decimal integer string",
    "directions[0].priceConfigCheck.priceFeed.priceSnapshot.updatedAt age exceeds 60s",
    "directions[0].priceConfigCheck.priceFeed.priceSnapshot.staleAfter must equal expectedStaleAfter 1800",
    "directions[0].priceConfigCheck.executor.priceFeed must equal priceFeed.address 0x4444444444444444444444444444444444444444",
    "directions[0].priceConfigCheck.executor.baseFee must be a non-negative decimal integer string",
    "directions[0].priceConfigCheck.executor.dstGasOverhead must be a non-negative decimal integer string",
    "directions[0].priceConfigCheck.executor.marginBps must be between 0 and 10000 bps",
  ]);
});

function baseRecord(): MigrationEvidenceRecord {
  return {
    ticket: "MIG-001",
    environment: "testnet",
    scope: "Ethereum Sepolia <-> Hoodi executor and DVN join rehearsal",
    operatorContacts: ["ops@example.com"],
    ownerAccount: "0x1111111111111111111111111111111111111111",
    signerAccount: "0x2222222222222222222222222222222222222222",
    makeCheck: evidence("artifacts/make-check.txt"),
    layerZeroAddressCheck: evidence("artifacts/layerzero-address-check.json"),
    readinessCheck: evidence("artifacts/readinesscheck.json"),
    keyManagementReview: evidence("docs/runbooks/key-management.md"),
    priceBotReview: evidence("docs/runbooks/price-bot.md"),
    rateLimitReview: evidence("docs/runbooks/rate-limit.md"),
    monitoringReview: evidence("docs/runbooks/monitoring.md"),
    runbookReview: evidence("artifacts/runbook-review.txt"),
    securityReview: evidence("docs/security/security-review.md"),
    directions: [
      {
        label: "Ethereum Sepolia to Hoodi",
        srcEid: 40161,
        dstEid: 40449,
        sourceWorkers: {
          openExecutor: "0x2222222222222222222222222222222222222222",
          openDVN: "0x3333333333333333333333333333333333333333",
          priceFeed: "0x4444444444444444444444444444444444444444",
        },
        destinationWorkers: {
          openDVN: "0x6666666666666666666666666666666666666666",
        },
        configDiff: evidence("artifacts/configdiff-eth-sepolia-to-hoodi.json"),
        deploymentPreflight: evidence(
          "artifacts/deployment-preflight-eth-sepolia.json",
        ),
        lzConfigBefore: evidence(
          "artifacts/lz-config-eth-sepolia-to-hoodi.before.json",
        ),
        lzConfigAfter: evidence(
          "artifacts/lz-config-eth-sepolia-to-hoodi.after.json",
        ),
        priceConfigCheck: priceConfigEvidence(
          "artifacts/price-config-eth-sepolia-to-hoodi.json",
          40449,
          "0x4444444444444444444444444444444444444444",
          "0x2222222222222222222222222222222222222222",
          "0x3333333333333333333333333333333333333333",
        ),
        drainCheckBeforeSwitch: evidence(
          "artifacts/drain-eth-sepolia-to-hoodi.json",
        ),
        canarySourceReceipt: evidence("artifacts/canary-source.json"),
        canaryDestinationReceipt: evidence("artifacts/canary-destination.json"),
        canary: {
          amountLD: "1000000000000000",
          senderAccount: "0x3333333333333333333333333333333333333333",
          recipientAccount: "0x4444444444444444444444444444444444444444",
          minRecipientBalanceLD: "1000000000000000",
          sourceReceipt: evidence("artifacts/canary-source.json"),
          destinationReceipt: evidence("artifacts/canary-destination.json"),
          recipientBalanceCheck: evidence(
            "artifacts/canary-recipient-balance.json",
          ),
        },
        dvnJoin: {
          confirmations: 12,
          requiredDVNs: ["OpenDVN", "LayerZero Labs DVN"],
          optionalDVNsDisabled: true,
          configCheck: evidence("artifacts/dvn-join-config.json"),
        },
        dvnVerificationReceipt: dvnVerification(
          "artifacts/dvn-verification.json",
          40161,
          40449,
        ),
      },
      {
        label: "Hoodi to Ethereum Sepolia",
        srcEid: 40449,
        dstEid: 40161,
        sourceWorkers: {
          openExecutor: "0x5555555555555555555555555555555555555555",
          openDVN: "0x6666666666666666666666666666666666666666",
          priceFeed: "0x7777777777777777777777777777777777777777",
        },
        destinationWorkers: {
          openDVN: "0x3333333333333333333333333333333333333333",
        },
        configDiff: evidence("artifacts/configdiff-hoodi-to-eth-sepolia.json"),
        deploymentPreflight: evidence(
          "artifacts/deployment-preflight-hoodi.json",
        ),
        lzConfigBefore: evidence(
          "artifacts/lz-config-hoodi-to-eth-sepolia.before.json",
        ),
        lzConfigAfter: evidence(
          "artifacts/lz-config-hoodi-to-eth-sepolia.after.json",
        ),
        priceConfigCheck: priceConfigEvidence(
          "artifacts/price-config-hoodi-to-eth-sepolia.json",
          40161,
          "0x7777777777777777777777777777777777777777",
          "0x5555555555555555555555555555555555555555",
          "0x6666666666666666666666666666666666666666",
        ),
        drainCheckBeforeSwitch: evidence(
          "artifacts/drain-hoodi-to-eth-sepolia.json",
        ),
        canarySourceReceipt: evidence("artifacts/canary-source-reverse.json"),
        canaryDestinationReceipt: evidence(
          "artifacts/canary-destination-reverse.json",
        ),
        canary: {
          amountLD: "1000000000000000",
          senderAccount: "0x3333333333333333333333333333333333333333",
          recipientAccount: "0x4444444444444444444444444444444444444444",
          minRecipientBalanceLD: "1000000000000000",
          sourceReceipt: evidence("artifacts/canary-source-reverse.json"),
          destinationReceipt: evidence(
            "artifacts/canary-destination-reverse.json",
          ),
          recipientBalanceCheck: evidence(
            "artifacts/canary-recipient-balance-reverse.json",
          ),
        },
        dvnJoin: {
          confirmations: 12,
          requiredDVNs: ["OpenDVN", "LayerZero Labs DVN"],
          optionalDVNsDisabled: true,
          configCheck: evidence("artifacts/dvn-join-config-reverse.json"),
        },
        dvnVerificationReceipt: dvnVerification(
          "artifacts/dvn-verification-reverse.json",
          40449,
          40161,
        ),
      },
    ],
    rollback: {
      previousExecutorConfig: evidence("artifacts/executor-before.json"),
      previousSendUlnConfig: evidence("artifacts/send-uln-before.json"),
      previousReceiveUlnConfig: evidence("artifacts/receive-uln-before.json"),
      dryRun: evidence("artifacts/rollback-dry-run.json"),
      restoredConfigCheck: evidence("artifacts/rollback-lz-config.after.json"),
      canaryAfterRollback: evidence("artifacts/rollback-canary.json"),
      ownerPauseAccount: "0x1111111111111111111111111111111111111111",
      signerAccount: "0x2222222222222222222222222222222222222222",
      drainCheck: evidence("artifacts/drain-rollback.json"),
      manualRetryPlan: evidence("artifacts/manual-retry.md"),
    },
  };
}

function evidence(ref: string): EvidenceRef {
  return { ref, capturedAt: "2026-06-27T00:00:00Z", reviewer: "ops" };
}

function priceConfigEvidence(
  ref: string,
  dstEid: number,
  priceFeed: string,
  executor: string,
  dvn: string,
): PriceConfigEvidence {
  return {
    ...evidence(ref),
    dstEid,
    checkedAt: "1000",
    maxAgeSeconds: "60",
    expectedStaleAfter: "1800",
    priceFeed: {
      address: priceFeed,
      priceSnapshot: {
        updatedAt: "950",
        staleAfter: "1800",
        dstGasPriceInSrcToken: "2",
      },
    },
    executor: {
      address: executor,
      priceFeed,
      baseFee: "1000",
      dstGasOverhead: "50000",
      marginBps: 100,
    },
    dvn: {
      address: dvn,
      priceFeed,
      baseFee: "2000",
      dstGasOverhead: "150000",
      marginBps: 200,
    },
  };
}

function dvnVerification(
  ref: string,
  srcEid: number,
  dstEid: number,
): DVNVerificationEvidence {
  return {
    ...evidence(ref),
    expectedPayloadHash:
      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    expectedSrcEid: srcEid,
    expectedDstEid: dstEid,
    expectedNonce: "1",
    expectedSender: "0x3333333333333333333333333333333333333333",
    expectedReceiver: "0x4444444444444444444444444444444444444444",
  };
}
