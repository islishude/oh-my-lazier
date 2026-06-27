import { readFileSync } from "node:fs";
import { jsonStringify, requiredEnv } from "./lib.js";

const phase1DirectionKeys = new Set(["40161->40245", "40245->40161"]);

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
  canary: CanaryEvidence;
  dvnJoin: DVNJoinEvidence;
  dvnVerificationReceipt: EvidenceRef;
};

export type CanaryEvidence = {
  amountLD: string;
  senderAccount: string;
  recipientAccount: string;
  minRecipientBalanceLD: string;
  sourceReceipt: EvidenceRef;
  destinationReceipt: EvidenceRef;
  recipientBalanceCheck: EvidenceRef;
};

export type DVNJoinEvidence = {
  confirmations: number;
  requiredDVNs: string[];
  optionalDVNsDisabled: boolean;
  configCheck: EvidenceRef;
};

export type RollbackEvidence = {
  previousExecutorConfig: EvidenceRef;
  previousSendUlnConfig: EvidenceRef;
  previousReceiveUlnConfig: EvidenceRef;
  restoredConfigCheck: EvidenceRef;
  canaryAfterRollback: EvidenceRef;
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
  layerZeroAddressCheck: EvidenceRef;
  readinessCheck: EvidenceRef;
  keyManagementReview: EvidenceRef;
  priceBotReview: EvidenceRef;
  rateLimitReview: EvidenceRef;
  monitoringReview: EvidenceRef;
  runbookReview: EvidenceRef;
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
  requireEVMAddress(errors, record.ownerAccount, "ownerAccount");
  requireEVMAddress(errors, record.signerAccount, "signerAccount");
  requireStringArray(errors, record.operatorContacts, "operatorContacts");
  requireEvidence(errors, record.makeCheck, "makeCheck");
  requireEvidence(
    errors,
    record.layerZeroAddressCheck,
    "layerZeroAddressCheck",
  );
  requireEvidence(errors, record.readinessCheck, "readinessCheck");
  requireEvidence(errors, record.keyManagementReview, "keyManagementReview");
  requireEvidence(errors, record.priceBotReview, "priceBotReview");
  requireEvidence(errors, record.rateLimitReview, "rateLimitReview");
  requireEvidence(errors, record.monitoringReview, "monitoringReview");
  requireEvidence(errors, record.runbookReview, "runbookReview");
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
  const directionKeys: string[] = [];
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
    directionKeys.push(key);
    requireEvidence(errors, direction.configDiff, `${prefix}.configDiff`);
    requireEvidence(
      errors,
      direction.deploymentPreflight,
      `${prefix}.deploymentPreflight`,
    );
    requireEvidence(
      errors,
      direction.lzConfigBefore,
      `${prefix}.lzConfigBefore`,
    );
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
    validateCanary(errors, direction.canary, `${prefix}.canary`);
    validateDVNJoin(errors, direction.dvnJoin, `${prefix}.dvnJoin`);
    requireEvidence(
      errors,
      direction.dvnVerificationReceipt,
      `${prefix}.dvnVerificationReceipt`,
    );
  });
  for (const key of directionKeys) {
    if (!phase1DirectionKeys.has(key)) {
      errors.push(`directions contains unsupported phase-1 direction ${key}`);
    }
    const [srcEid, dstEid] = key.split("->");
    const reverseKey = `${dstEid}->${srcEid}`;
    if (!seen.has(reverseKey)) {
      errors.push(`directions missing reciprocal direction ${reverseKey}`);
    }
  }
  for (const expectedKey of phase1DirectionKeys) {
    if (!seen.has(expectedKey)) {
      errors.push(`directions missing phase-1 direction ${expectedKey}`);
    }
  }
}

function validateCanary(
  errors: string[],
  canary: CanaryEvidence,
  field: string,
): void {
  if (!isRecord(canary)) {
    errors.push(`${field} is required`);
    return;
  }
  requirePositiveDecimalInteger(errors, canary.amountLD, `${field}.amountLD`);
  requireEVMAddress(errors, canary.senderAccount, `${field}.senderAccount`);
  requireEVMAddress(
    errors,
    canary.recipientAccount,
    `${field}.recipientAccount`,
  );
  requirePositiveDecimalInteger(
    errors,
    canary.minRecipientBalanceLD,
    `${field}.minRecipientBalanceLD`,
  );
  requireEvidence(errors, canary.sourceReceipt, `${field}.sourceReceipt`);
  requireEvidence(
    errors,
    canary.destinationReceipt,
    `${field}.destinationReceipt`,
  );
  requireEvidence(
    errors,
    canary.recipientBalanceCheck,
    `${field}.recipientBalanceCheck`,
  );
}

function validateDVNJoin(
  errors: string[],
  dvnJoin: DVNJoinEvidence,
  field: string,
): void {
  if (!isRecord(dvnJoin)) {
    errors.push(`${field} is required`);
    return;
  }
  if (dvnJoin.confirmations !== 12) {
    errors.push(`${field}.confirmations must be 12`);
  }
  requireStringArray(errors, dvnJoin.requiredDVNs, `${field}.requiredDVNs`);
  if (Array.isArray(dvnJoin.requiredDVNs)) {
    const required = new Set(
      dvnJoin.requiredDVNs.map((value) => value.toLowerCase()),
    );
    for (const label of ["opendvn", "layerzero labs dvn"]) {
      if (!required.has(label)) {
        errors.push(`${field}.requiredDVNs must include ${label}`);
      }
    }
    if (required.size < 2) {
      errors.push(`${field}.requiredDVNs must not be self-only`);
    }
  }
  if (dvnJoin.optionalDVNsDisabled !== true) {
    errors.push(`${field}.optionalDVNsDisabled must be true`);
  }
  requireEvidence(errors, dvnJoin.configCheck, `${field}.configCheck`);
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
  requireEvidence(
    errors,
    rollback.restoredConfigCheck,
    "rollback.restoredConfigCheck",
  );
  requireEvidence(
    errors,
    rollback.canaryAfterRollback,
    "rollback.canaryAfterRollback",
  );
  requireEVMAddress(
    errors,
    rollback.ownerPauseAccount,
    "rollback.ownerPauseAccount",
  );
  requireEVMAddress(errors, rollback.signerAccount, "rollback.signerAccount");
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

function requirePositiveDecimalInteger(
  errors: string[],
  value: string,
  field: string,
): void {
  if (typeof value !== "string" || !/^[1-9][0-9]*$/.test(value)) {
    errors.push(`${field} must be a positive decimal integer string`);
  }
}

function requireNonEmpty(errors: string[], value: string, field: string): void {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${field} must be a non-empty string`);
  }
}

function requireEVMAddress(
  errors: string[],
  value: string,
  field: string,
): void {
  if (typeof value !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(value)) {
    errors.push(`${field} must be an EVM address`);
    return;
  }
  if (/^0x0{40}$/i.test(value)) {
    errors.push(`${field} must not be the zero address`);
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
