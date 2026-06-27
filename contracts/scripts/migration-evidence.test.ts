import assert from "node:assert/strict";
import test from "node:test";
import {
  validateMigrationEvidenceRecord,
  type EvidenceRef,
  type MigrationEvidenceRecord,
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
  };
  record.rollback.manualRetryPlan = evidence("");

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, [
    "operatorContacts must contain at least one value",
    "directions[0].srcEid and directions[0].dstEid must differ",
    "directions[0].canaryDestinationReceipt.ref must be a non-empty string",
    "rollback.manualRetryPlan.ref must be a non-empty string",
  ]);
});

test("validateMigrationEvidenceRecord rejects duplicate directions", () => {
  const record = baseRecord();
  record.directions.push({ ...record.directions[0], label: "duplicate" });

  const errors = validateMigrationEvidenceRecord(record);

  assert.deepEqual(errors, ["directions[1] duplicates direction 40161->40245"]);
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
    keyManagementReview: evidence("docs/runbooks/key-management.md"),
    priceBotReview: evidence("docs/runbooks/price-bot.md"),
    rateLimitReview: evidence("docs/runbooks/rate-limit.md"),
    monitoringReview: evidence("docs/runbooks/monitoring.md"),
    securityReview: evidence("docs/security/parent-agent-security-review.md"),
    directions: [
      {
        label: "Ethereum Sepolia to Base Sepolia",
        srcEid: 40161,
        dstEid: 40245,
        configDiff: evidence("artifacts/configdiff-sepolia-base.json"),
        deploymentPreflight: evidence("artifacts/preflight-sepolia.json"),
        lzConfigBefore: evidence("artifacts/lz-config-sepolia-base.before.json"),
        lzConfigAfter: evidence("artifacts/lz-config-sepolia-base.after.json"),
        priceConfigCheck: evidence("artifacts/price-config-sepolia-base.json"),
        drainCheckBeforeSwitch: evidence("artifacts/drain-sepolia-base.json"),
        canarySourceReceipt: evidence("artifacts/canary-source.json"),
        canaryDestinationReceipt: evidence("artifacts/canary-destination.json"),
        dvnVerificationReceipt: evidence("artifacts/dvn-verification.json"),
      },
    ],
    rollback: {
      previousExecutorConfig: evidence("artifacts/executor-before.json"),
      previousSendUlnConfig: evidence("artifacts/send-uln-before.json"),
      previousReceiveUlnConfig: evidence("artifacts/receive-uln-before.json"),
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
