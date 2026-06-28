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
      confirmations: 11,
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
    "directions[0].dvnJoin.confirmations must be 12",
    "directions[0].dvnJoin.requiredDVNs must include layerzero labs dvn",
    "directions[0].dvnJoin.requiredDVNs must not be self-only",
    "directions[0].dvnJoin.optionalDVNsDisabled must be true",
    "directions[0].dvnJoin.configCheck.ref must be a non-empty string",
    "directions[0].dvnVerificationReceipt.expectedDstEid must equal direction dstEid 40161",
    "directions contains unsupported phase-1 direction 40161->40161",
    "directions missing reciprocal direction 40161->40245",
    "directions missing phase-1 direction 40161->40245",
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
    "directions[0].canary.senderAccount must not be the zero address",
    "directions[1].canary.recipientAccount must be an EVM address",
    "rollback.ownerPauseAccount must not be the zero address",
    "rollback.signerAccount must be an EVM address",
  ]);
});

test("validateMigrationEvidenceRecord rejects duplicate directions", () => {
  const record = baseRecord();
  record.directions.push({ ...record.directions[0], label: "duplicate" });

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, ["directions[2] duplicates direction 40161->40245"]);
});

test("validateMigrationEvidenceRecord rejects missing reciprocal direction", () => {
  const record = baseRecord();
  record.directions = [record.directions[0]];

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions missing reciprocal direction 40245->40161",
    "directions missing phase-1 direction 40245->40161",
  ]);
});

test("validateMigrationEvidenceRecord rejects unsupported phase-1 directions", () => {
  const record = baseRecord();
  record.directions[1] = {
    ...record.directions[1],
    srcEid: 40245,
    dstEid: 99999,
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[1].priceConfigCheck.dstEid must equal direction dstEid 99999",
    "directions[1].dvnVerificationReceipt.expectedDstEid must equal direction dstEid 99999",
    "directions missing reciprocal direction 40245->40161",
    "directions contains unsupported phase-1 direction 40245->99999",
    "directions missing reciprocal direction 99999->40245",
    "directions missing phase-1 direction 40245->40161",
  ]);
});

test("validateMigrationEvidenceRecord rejects weak DVN verification packet identity", () => {
  const record = baseRecord();
  record.directions[0].dvnVerificationReceipt = {
    ...record.directions[0].dvnVerificationReceipt,
    expectedPayloadHash: "0xabc",
    expectedSrcEid: 40245,
    expectedDstEid: 40161,
    expectedNonce: "0",
    expectedSender: "0x0000000000000000000000000000000000000000",
    expectedReceiver: "not-an-address",
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[0].dvnVerificationReceipt.expectedPayloadHash must be a bytes32 hex string",
    "directions[0].dvnVerificationReceipt.expectedSrcEid must equal direction srcEid 40161",
    "directions[0].dvnVerificationReceipt.expectedDstEid must equal direction dstEid 40245",
    "directions[0].dvnVerificationReceipt.expectedNonce must be a positive decimal integer string",
    "directions[0].dvnVerificationReceipt.expectedSender must not be the zero address",
    "directions[0].dvnVerificationReceipt.expectedReceiver must be an EVM address",
  ]);
});

test("validateMigrationEvidenceRecord rejects stale or mismatched price config evidence", () => {
  const record = baseRecord();
  record.directions[0].priceConfigCheck = {
    ...record.directions[0].priceConfigCheck,
    dstEid: 40161,
    checkedAt: "1000",
    maxAgeSeconds: "60",
    expectedStaleAfter: "1800",
    executor: {
      updatedAt: "900",
      staleAfter: "120",
      dstGasPriceInSrcToken: "0",
    },
    dvn: {
      updatedAt: "1001",
      staleAfter: "1800",
      dstGasPriceInSrcToken: "2",
    },
  };

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "directions[0].priceConfigCheck.dstEid must equal direction dstEid 40245",
    "directions[0].priceConfigCheck.executor.dstGasPriceInSrcToken must be a positive decimal integer string",
    "directions[0].priceConfigCheck.executor.updatedAt age exceeds 60s",
    "directions[0].priceConfigCheck.executor.staleAfter must equal expectedStaleAfter 1800",
    "directions[0].priceConfigCheck.dvn.updatedAt must not be in the future",
  ]);
});

function baseRecord(): MigrationEvidenceRecord {
  return {
    ticket: "MIG-001",
    environment: "testnet",
    scope: "Ethereum Sepolia <-> Base Sepolia executor and DVN join rehearsal",
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
        label: "Ethereum Sepolia to Base Sepolia",
        srcEid: 40161,
        dstEid: 40245,
        configDiff: evidence("artifacts/configdiff-sepolia-base.json"),
        deploymentPreflight: evidence("artifacts/preflight-sepolia.json"),
        lzConfigBefore: evidence(
          "artifacts/lz-config-sepolia-base.before.json",
        ),
        lzConfigAfter: evidence("artifacts/lz-config-sepolia-base.after.json"),
        priceConfigCheck: priceConfigEvidence(
          "artifacts/price-config-sepolia-base.json",
          40245,
        ),
        drainCheckBeforeSwitch: evidence("artifacts/drain-sepolia-base.json"),
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
          40245,
        ),
      },
      {
        label: "Base Sepolia to Ethereum Sepolia",
        srcEid: 40245,
        dstEid: 40161,
        configDiff: evidence("artifacts/configdiff-base-sepolia.json"),
        deploymentPreflight: evidence("artifacts/preflight-base-sepolia.json"),
        lzConfigBefore: evidence(
          "artifacts/lz-config-base-sepolia.before.json",
        ),
        lzConfigAfter: evidence("artifacts/lz-config-base-sepolia.after.json"),
        priceConfigCheck: priceConfigEvidence(
          "artifacts/price-config-base-sepolia.json",
          40161,
        ),
        drainCheckBeforeSwitch: evidence("artifacts/drain-base-sepolia.json"),
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
          40245,
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

function priceConfigEvidence(ref: string, dstEid: number): PriceConfigEvidence {
  return {
    ...evidence(ref),
    dstEid,
    checkedAt: "1000",
    maxAgeSeconds: "60",
    expectedStaleAfter: "1800",
    executor: {
      updatedAt: "950",
      staleAfter: "1800",
      dstGasPriceInSrcToken: "2",
    },
    dvn: {
      updatedAt: "950",
      staleAfter: "1800",
      dstGasPriceInSrcToken: "2",
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
