import { readFileSync } from "node:fs";
import { jsonStringify, requiredEnv } from "./lib.js";

export type EvidenceRef = {
  ref: string;
  capturedAt?: string;
  reviewer?: string;
};

export type MigrationDirectionEvidence = {
  label: string;
  srcEid: number;
  dstEid: number;
  configDiff: EvidenceRef;
  deploymentPreflight: EvidenceRef;
  lzConfigBefore: EvidenceRef;
  lzConfigAfter: EvidenceRef;
  priceConfigCheck: EvidenceRef;
  drainCheckBeforeSwitch: EvidenceRef;
  canarySourceReceipt: EvidenceRef;
  canaryDestinationReceipt: EvidenceRef;
  dvnVerificationReceipt: EvidenceRef;
};

export type RollbackEvidence = {
  previousExecutorConfig: EvidenceRef;
  previousSendUlnConfig: EvidenceRef;
  previousReceiveUlnConfig: EvidenceRef;
  ownerPauseAccount: string;
  signerAccount: string;
  drainCheck: EvidenceRef;
  manualRetryPlan: EvidenceRef;
};

export type MigrationEvidenceRecord = {
  ticket: string;
  environment: string;
  scope: string;
  operatorContacts: string[];
  ownerAccount: string;
  signerAccount: string;
  makeCheck: EvidenceRef;
  keyManagementReview: EvidenceRef;
  priceBotReview: EvidenceRef;
  rateLimitReview: EvidenceRef;
  monitoringReview: EvidenceRef;
  securityReview: EvidenceRef;
  directions: MigrationDirectionEvidence[];
  rollback: RollbackEvidence;
};

export function validateMigrationEvidenceRecord(
  record: MigrationEvidenceRecord,
): string[] {
  const errors: string[] = [];
  requireNonEmpty(errors, record.ticket, "ticket");
  requireNonEmpty(errors, record.environment, "environment");
  requireNonEmpty(errors, record.scope, "scope");
  requireNonEmpty(errors, record.ownerAccount, "ownerAccount");
  requireNonEmpty(errors, record.signerAccount, "signerAccount");
  requireStringArray(errors, record.operatorContacts, "operatorContacts");
  requireEvidence(errors, record.makeCheck, "makeCheck");
  requireEvidence(errors, record.keyManagementReview, "keyManagementReview");
  requireEvidence(errors, record.priceBotReview, "priceBotReview");
  requireEvidence(errors, record.rateLimitReview, "rateLimitReview");
  requireEvidence(errors, record.monitoringReview, "monitoringReview");
  requireEvidence(errors, record.securityReview, "securityReview");
  validateDirections(errors, record.directions);
  validateRollback(errors, record.rollback);
  return errors;
}

function validateDirections(
  errors: string[],
  directions: MigrationDirectionEvidence[],
): void {
  if (!Array.isArray(directions) || directions.length === 0) {
    errors.push("directions must contain at least one direction");
    return;
  }
  const seen = new Set<string>();
  directions.forEach((direction, index) => {
    const prefix = `directions[${index}]`;
    requireNonEmpty(errors, direction.label, `${prefix}.label`);
    requirePositiveInteger(errors, direction.srcEid, `${prefix}.srcEid`);
    requirePositiveInteger(errors, direction.dstEid, `${prefix}.dstEid`);
    if (direction.srcEid === direction.dstEid) {
      errors.push(`${prefix}.srcEid and ${prefix}.dstEid must differ`);
    }
    const key = `${direction.srcEid}->${direction.dstEid}`;
    if (seen.has(key)) {
      errors.push(`${prefix} duplicates direction ${key}`);
    }
    seen.add(key);
    requireEvidence(errors, direction.configDiff, `${prefix}.configDiff`);
    requireEvidence(
      errors,
      direction.deploymentPreflight,
      `${prefix}.deploymentPreflight`,
    );
    requireEvidence(errors, direction.lzConfigBefore, `${prefix}.lzConfigBefore`);
    requireEvidence(errors, direction.lzConfigAfter, `${prefix}.lzConfigAfter`);
    requireEvidence(
      errors,
      direction.priceConfigCheck,
      `${prefix}.priceConfigCheck`,
    );
    requireEvidence(
      errors,
      direction.drainCheckBeforeSwitch,
      `${prefix}.drainCheckBeforeSwitch`,
    );
    requireEvidence(
      errors,
      direction.canarySourceReceipt,
      `${prefix}.canarySourceReceipt`,
    );
    requireEvidence(
      errors,
      direction.canaryDestinationReceipt,
      `${prefix}.canaryDestinationReceipt`,
    );
    requireEvidence(
      errors,
      direction.dvnVerificationReceipt,
      `${prefix}.dvnVerificationReceipt`,
    );
  });
}

function validateRollback(errors: string[], rollback: RollbackEvidence): void {
  if (!isRecord(rollback)) {
    errors.push("rollback is required");
    return;
  }
  requireEvidence(
    errors,
    rollback.previousExecutorConfig,
    "rollback.previousExecutorConfig",
  );
  requireEvidence(
    errors,
    rollback.previousSendUlnConfig,
    "rollback.previousSendUlnConfig",
  );
  requireEvidence(
    errors,
    rollback.previousReceiveUlnConfig,
    "rollback.previousReceiveUlnConfig",
  );
  requireNonEmpty(errors, rollback.ownerPauseAccount, "rollback.ownerPauseAccount");
  requireNonEmpty(errors, rollback.signerAccount, "rollback.signerAccount");
  requireEvidence(errors, rollback.drainCheck, "rollback.drainCheck");
  requireEvidence(errors, rollback.manualRetryPlan, "rollback.manualRetryPlan");
}

function requireEvidence(
  errors: string[],
  value: EvidenceRef,
  field: string,
): void {
  if (!isRecord(value)) {
    errors.push(`${field} evidence is required`);
    return;
  }
  requireNonEmpty(errors, value.ref, `${field}.ref`);
  if (value.capturedAt !== undefined) {
    requireNonEmpty(errors, value.capturedAt, `${field}.capturedAt`);
  }
  if (value.reviewer !== undefined) {
    requireNonEmpty(errors, value.reviewer, `${field}.reviewer`);
  }
}

function requireStringArray(
  errors: string[],
  value: string[],
  field: string,
): void {
  if (!Array.isArray(value) || value.length === 0) {
    errors.push(`${field} must contain at least one value`);
    return;
  }
  value.forEach((item, index) => {
    requireNonEmpty(errors, item, `${field}[${index}]`);
  });
}

function requirePositiveInteger(
  errors: string[],
  value: number,
  field: string,
): void {
  if (!Number.isSafeInteger(value) || value <= 0) {
    errors.push(`${field} must be a positive integer`);
  }
}

function requireNonEmpty(
  errors: string[],
  value: string,
  field: string,
): void {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${field} must be a non-empty string`);
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const path = requiredEnv("MIGRATION_EVIDENCE");
  const record = JSON.parse(
    readFileSync(path, "utf8"),
  ) as MigrationEvidenceRecord;
  const errors = validateMigrationEvidenceRecord(record);
  if (errors.length > 0) {
    console.error(jsonStringify({ ok: false, errors }));
    process.exit(1);
  }
  console.log(
    jsonStringify({
      ok: true,
      ticket: record.ticket,
      environment: record.environment,
      directions: record.directions.length,
    }),
  );
}
