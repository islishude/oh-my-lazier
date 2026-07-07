import { readFileSync } from "node:fs";
import { EndpointId } from "@layerzerolabs/lz-definitions";
import { jsonStringify, requiredEnv } from "./lib.js";

const sepoliaEid = EndpointId.SEPOLIA_V2_TESTNET;
const hoodiEid = EndpointId.HOODI_V2_TESTNET;
const phase1DirectionKeys = new Set([
  `${sepoliaEid}->${hoodiEid}`,
  `${hoodiEid}->${sepoliaEid}`,
]);

export type EvidenceRef = {
  ref: string;
  capturedAt?: string;
  reviewer?: string;
};

export type MigrationDirectionEvidence = {
  label: string;
  srcEid: number;
  dstEid: number;
  sourceWorkers: SourceWorkersEvidence;
  destinationWorkers: DestinationWorkersEvidence;
  configDiff: EvidenceRef;
  deploymentPreflight: EvidenceRef;
  lzConfigBefore: EvidenceRef;
  lzConfigAfter: EvidenceRef;
  priceConfigCheck: PriceConfigEvidence;
  drainCheckBeforeSwitch: EvidenceRef;
  canarySourceReceipt: EvidenceRef;
  canaryDestinationReceipt: EvidenceRef;
  canary: CanaryEvidence;
  dvnJoin: DVNJoinEvidence;
  dvnVerificationReceipt: DVNVerificationEvidence;
};

export type SourceWorkersEvidence = {
  openExecutor: string;
  openDVN: string;
  priceFeed: string;
};

export type DestinationWorkersEvidence = {
  openDVN: string;
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

export type PriceSnapshotEvidence = {
  dstGasPriceInSrcToken: string;
  dstDataFeePerByteInSrcToken: string;
  updatedAt: string;
  staleAfter: string;
};

export type WorkerFeeModelEvidence = {
  address: string;
  priceFeed: string;
  baseFee: string;
  dstGasOverhead: string;
  dataSizeOverheadBytes: string;
  marginBps: number;
};

export type PriceConfigEvidence = EvidenceRef & {
  dstEid: number;
  checkedAt: string;
  maxAgeSeconds: string;
  expectedStaleAfter: string;
  priceFeed: {
    address: string;
    priceSnapshot: PriceSnapshotEvidence;
  };
  executor: WorkerFeeModelEvidence;
  dvn: WorkerFeeModelEvidence;
};

export type DVNVerificationEvidence = EvidenceRef & {
  expectedPayloadHash: string;
  expectedSrcEid: number;
  expectedDstEid: number;
  expectedNonce: string;
  expectedSender: string;
  expectedReceiver: string;
};

export type RollbackEvidence = {
  previousExecutorConfig: EvidenceRef;
  previousSendUlnConfig: EvidenceRef;
  previousReceiveUlnConfig: EvidenceRef;
  dryRun: EvidenceRef;
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
    validateSourceWorkers(
      errors,
      direction.sourceWorkers,
      `${prefix}.sourceWorkers`,
    );
    validateDestinationWorkers(
      errors,
      direction.destinationWorkers,
      `${prefix}.destinationWorkers`,
    );
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
    validatePriceConfigEvidence(
      errors,
      direction.priceConfigCheck,
      `${prefix}.priceConfigCheck`,
      direction.dstEid,
      direction.sourceWorkers,
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
    validateDVNVerification(
      errors,
      direction.dvnVerificationReceipt,
      `${prefix}.dvnVerificationReceipt`,
      direction.srcEid,
      direction.dstEid,
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

function validateSourceWorkers(
  errors: string[],
  workers: SourceWorkersEvidence,
  field: string,
): void {
  if (!isRecord(workers)) {
    errors.push(`${field} is required`);
    return;
  }
  requireEVMAddress(errors, workers.openExecutor, `${field}.openExecutor`);
  requireEVMAddress(errors, workers.openDVN, `${field}.openDVN`);
  requireEVMAddress(errors, workers.priceFeed, `${field}.priceFeed`);
}

function validateDestinationWorkers(
  errors: string[],
  workers: DestinationWorkersEvidence,
  field: string,
): void {
  if (!isRecord(workers)) {
    errors.push(`${field} is required`);
    return;
  }
  requireEVMAddress(errors, workers.openDVN, `${field}.openDVN`);
}

function validatePriceConfigEvidence(
  errors: string[],
  evidence: PriceConfigEvidence,
  field: string,
  dstEid: number,
  sourceWorkers?: SourceWorkersEvidence,
): void {
  if (!isRecord(evidence)) {
    errors.push(`${field} evidence is required`);
    return;
  }
  requireEvidence(errors, evidence, field);
  if (evidence.dstEid !== dstEid) {
    errors.push(`${field}.dstEid must equal direction dstEid ${dstEid}`);
  }
  const checkedAt = requirePositiveDecimalIntegerValue(
    errors,
    evidence.checkedAt,
    `${field}.checkedAt`,
  );
  const maxAge = requirePositiveDecimalIntegerValue(
    errors,
    evidence.maxAgeSeconds,
    `${field}.maxAgeSeconds`,
  );
  const expectedStaleAfter = requirePositiveDecimalIntegerValue(
    errors,
    evidence.expectedStaleAfter,
    `${field}.expectedStaleAfter`,
  );
  let priceFeedAddress: string | undefined;
  if (!isRecord(evidence.priceFeed)) {
    errors.push(`${field}.priceFeed is required`);
  } else {
    priceFeedAddress = evidence.priceFeed.address;
    requireEVMAddress(
      errors,
      evidence.priceFeed.address,
      `${field}.priceFeed.address`,
    );
    requireMatchingAddress(
      errors,
      evidence.priceFeed.address,
      `${field}.priceFeed.address`,
      sourceWorkers?.priceFeed,
      "sourceWorkers.priceFeed",
    );
    validatePriceSnapshotEvidence(
      errors,
      evidence.priceFeed.priceSnapshot,
      `${field}.priceFeed.priceSnapshot`,
      checkedAt,
      maxAge,
      expectedStaleAfter,
    );
  }
  validateWorkerFeeModelEvidence(
    errors,
    evidence.executor,
    `${field}.executor`,
    priceFeedAddress,
    {
      address: sourceWorkers?.openExecutor,
      field: "sourceWorkers.openExecutor",
    },
  );
  validateWorkerFeeModelEvidence(
    errors,
    evidence.dvn,
    `${field}.dvn`,
    priceFeedAddress,
    { address: sourceWorkers?.openDVN, field: "sourceWorkers.openDVN" },
  );
}

function validatePriceSnapshotEvidence(
  errors: string[],
  evidence: PriceSnapshotEvidence,
  field: string,
  checkedAt?: bigint,
  maxAge?: bigint,
  expectedStaleAfter?: bigint,
): void {
  if (!isRecord(evidence)) {
    errors.push(`${field} evidence is required`);
    return;
  }
  const updatedAt = requirePositiveDecimalIntegerValue(
    errors,
    evidence.updatedAt,
    `${field}.updatedAt`,
  );
  const staleAfter = requirePositiveDecimalIntegerValue(
    errors,
    evidence.staleAfter,
    `${field}.staleAfter`,
  );
  requirePositiveDecimalIntegerValue(
    errors,
    evidence.dstGasPriceInSrcToken,
    `${field}.dstGasPriceInSrcToken`,
  );
  requireNonNegativeDecimalIntegerValue(
    errors,
    evidence.dstDataFeePerByteInSrcToken,
    `${field}.dstDataFeePerByteInSrcToken`,
  );
  if (
    checkedAt !== undefined &&
    updatedAt !== undefined &&
    updatedAt > checkedAt
  ) {
    errors.push(`${field}.updatedAt must not be in the future`);
  }
  if (
    checkedAt !== undefined &&
    maxAge !== undefined &&
    updatedAt !== undefined &&
    checkedAt >= updatedAt &&
    checkedAt - updatedAt > maxAge
  ) {
    errors.push(`${field}.updatedAt age exceeds ${maxAge}s`);
  }
  if (
    expectedStaleAfter !== undefined &&
    staleAfter !== undefined &&
    staleAfter !== expectedStaleAfter
  ) {
    errors.push(
      `${field}.staleAfter must equal expectedStaleAfter ${expectedStaleAfter}`,
    );
  }
}

function validateWorkerFeeModelEvidence(
  errors: string[],
  evidence: WorkerFeeModelEvidence,
  field: string,
  expectedPriceFeed?: string,
  expectedWorker?: { address?: string; field: string },
): void {
  if (!isRecord(evidence)) {
    errors.push(`${field} evidence is required`);
    return;
  }
  requireEVMAddress(errors, evidence.address, `${field}.address`);
  requireMatchingAddress(
    errors,
    evidence.address,
    `${field}.address`,
    expectedWorker?.address,
    expectedWorker?.field,
  );
  requireEVMAddress(errors, evidence.priceFeed, `${field}.priceFeed`);
  if (
    expectedPriceFeed !== undefined &&
    isAddressString(evidence.priceFeed) &&
    evidence.priceFeed.toLowerCase() !== expectedPriceFeed.toLowerCase()
  ) {
    errors.push(
      `${field}.priceFeed must equal priceFeed.address ${expectedPriceFeed}`,
    );
  }
  requireNonNegativeDecimalIntegerValue(
    errors,
    evidence.baseFee,
    `${field}.baseFee`,
  );
  requireNonNegativeDecimalIntegerValue(
    errors,
    evidence.dstGasOverhead,
    `${field}.dstGasOverhead`,
  );
  requireNonNegativeDecimalIntegerValue(
    errors,
    evidence.dataSizeOverheadBytes,
    `${field}.dataSizeOverheadBytes`,
  );
  requireBps(errors, evidence.marginBps, `${field}.marginBps`);
}

function requireMatchingAddress(
  errors: string[],
  actual: string,
  actualField: string,
  expected?: string,
  expectedField?: string,
): void {
  if (
    expected === undefined ||
    expectedField === undefined ||
    !isNonZeroAddressString(actual) ||
    !isNonZeroAddressString(expected)
  ) {
    return;
  }
  if (actual.toLowerCase() !== expected.toLowerCase()) {
    errors.push(`${actualField} must equal ${expectedField} ${expected}`);
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
  requirePositiveInteger(
    errors,
    dvnJoin.confirmations,
    `${field}.confirmations`,
  );
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

function validateDVNVerification(
  errors: string[],
  verification: DVNVerificationEvidence,
  field: string,
  srcEid: number,
  dstEid: number,
): void {
  if (!isRecord(verification)) {
    errors.push(`${field} evidence is required`);
    return;
  }
  requireEvidence(errors, verification, field);
  requireBytes32(
    errors,
    verification.expectedPayloadHash,
    `${field}.expectedPayloadHash`,
  );
  if (verification.expectedSrcEid !== srcEid) {
    errors.push(
      `${field}.expectedSrcEid must equal direction srcEid ${srcEid}`,
    );
  }
  if (verification.expectedDstEid !== dstEid) {
    errors.push(
      `${field}.expectedDstEid must equal direction dstEid ${dstEid}`,
    );
  }
  requirePositiveDecimalInteger(
    errors,
    verification.expectedNonce,
    `${field}.expectedNonce`,
  );
  requireEVMAddress(
    errors,
    verification.expectedSender,
    `${field}.expectedSender`,
  );
  requireEVMAddress(
    errors,
    verification.expectedReceiver,
    `${field}.expectedReceiver`,
  );
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
  requireEvidence(errors, rollback.dryRun, "rollback.dryRun");
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
  requirePositiveDecimalIntegerValue(errors, value, field);
}

function requirePositiveDecimalIntegerValue(
  errors: string[],
  value: string,
  field: string,
): bigint | undefined {
  if (typeof value !== "string" || !/^[1-9][0-9]*$/.test(value)) {
    errors.push(`${field} must be a positive decimal integer string`);
    return undefined;
  }
  return BigInt(value);
}

function requireNonNegativeDecimalIntegerValue(
  errors: string[],
  value: string,
  field: string,
): bigint | undefined {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value)) {
    errors.push(`${field} must be a non-negative decimal integer string`);
    return undefined;
  }
  return BigInt(value);
}

function requireBps(errors: string[], value: number, field: string): void {
  if (!Number.isSafeInteger(value) || value < 0 || value > 10_000) {
    errors.push(`${field} must be between 0 and 10000 bps`);
  }
}

function requireBytes32(errors: string[], value: string, field: string): void {
  if (typeof value !== "string" || !/^0x[0-9a-fA-F]{64}$/.test(value)) {
    errors.push(`${field} must be a bytes32 hex string`);
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
  if (!isAddressString(value)) {
    errors.push(`${field} must be an EVM address`);
    return;
  }
  if (/^0x0{40}$/i.test(value)) {
    errors.push(`${field} must not be the zero address`);
  }
}

function isNonZeroAddressString(value: unknown): value is string {
  return isAddressString(value) && !/^0x0{40}$/i.test(value);
}

function isAddressString(value: unknown): value is string {
  return typeof value === "string" && /^0x[0-9a-fA-F]{40}$/.test(value);
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
