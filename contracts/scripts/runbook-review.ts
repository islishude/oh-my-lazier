import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

type RequiredDoc = {
  path: string;
  anchors: string[];
};

type RequiredAlertRule = {
  alert: string;
  anchors: string[];
};

const requiredDocs: RequiredDoc[] = [
  {
    path: "docs/runbooks/mainnet-readiness.md",
    anchors: [
      "No `composeMsg`.",
      "No `lzCompose`.",
      "No native drop.",
      "No ordered execution.",
      "No non-EVM chain support.",
      "No self-only DVN.",
      "Required DVNs must include both OpenDVN and an independent LayerZero Labs DVN.",
      "Confirmations must be explicitly configured per chain and match the approved LayerZero ULN configuration.",
      "MIGRATION_EVIDENCE=<record.json> npm run check:migration-evidence",
      "npm run check:runbooks",
      "make security-check",
      "go run ./go/cmd/readinesscheck -config <worker.yaml>",
      "npm run inspect:lz-config",
      "Reject mainnet readiness if:",
    ],
  },
  {
    path: "docs/runbooks/monitoring.md",
    anchors: [
      "/healthz",
      "/readyz",
      "/metrics",
      "docs/monitoring/prometheus-alerts.yml",
      "LazWorkerReadinessFailed",
      "LazChainPaused",
      "LazPathwayPaused",
      "LazDVNQuorumConflict",
      "LazDVNReorgDetected",
      "LazPacketManualReview",
      "LazExecutorReceiveFailed",
      "LazWorkerManualReview",
      "LazTxOutboxFailed",
      "LazIndexerPollFailing",
      "LazIndexerCursorStalled",
      "laz_chain_paused == 1",
      "laz_pathway_paused == 1",
      "laz_indexer_poll_success",
      "laz_indexer_cursor_last_block",
      "go run ./go/cmd/readinesscheck -config <worker.yaml> -format json",
    ],
  },
  {
    path: "docs/runbooks/config-diff.md",
    anchors: [
      "go run ./go/cmd/configdiff",
      "-fail-on-diff",
      "Confirm signer changes are expected and do not point to unapproved keys.",
      "For DVN migration, confirm each proposed pathway still uses `pathways[].dvn.mode: shadow` until the explicit active-mode change is approved.",
    ],
  },
  {
    path: "docs/runbooks/key-management.md",
    anchors: [
      "AWS KMS `ECC_SECG_P256K1`",
      "local geth keystore JSON",
      "Never infer approval from a successful transaction alone.",
      "Run `make test-integration` when Docker is available",
      "Never log private key material, decrypted keystore content, KMS signatures, or raw secrets.",
      "rollback signer",
    ],
  },
  {
    path: "docs/runbooks/price-bot.md",
    anchors: [
      "go run ./go/cmd/pricebot-once -config <worker.yaml>",
      "npm run check:price-config",
      "For each unique source/destination/source-worker pair",
      "`updatedAt` is recent",
      "`staleAfter` matches the approved config",
      "If the newly submitted price config is wrong:",
    ],
  },
  {
    path: "docs/runbooks/rate-limit.md",
    anchors: [
      "pauseSend(uint32 dstEid, bool paused)",
      "pauseReceive(uint32 srcEid, bool paused)",
      "setOutboundRateLimit(uint32 dstEid, RateLimitConfig config)",
      "capacity = 0",
      "go run ./go/cmd/draincheck -config <worker.yaml>",
      "Do not approve mainnet readiness if:",
    ],
  },
  {
    path: "docs/deployments/testnet-migration-evidence.example.json",
    anchors: [
      '"layerZeroAddressCheck"',
      '"readinessCheck"',
      '"runbookReview"',
      '"ownerAccount"',
      '"signerAccount"',
      '"sourceWorkers"',
      '"canary"',
      '"dvnJoin"',
      '"rollback"',
      '"manualRetryPlan"',
    ],
  },
];

const requiredAlertRules: RequiredAlertRule[] = [
  {
    alert: "LazWorkerReadinessFailed",
    anchors: ['probe_success{job="laz-worker-readyz"} == 0', "severity: page"],
  },
  {
    alert: "LazChainPaused",
    anchors: ["laz_chain_paused == 1", "severity: page"],
  },
  {
    alert: "LazPathwayPaused",
    anchors: ["laz_pathway_paused == 1", "severity: page"],
  },
  {
    alert: "LazDVNQuorumConflict",
    anchors: [
      'laz_dvn_jobs_total{status="QUORUM_CONFLICT"} > 0',
      "severity: page",
    ],
  },
  {
    alert: "LazDVNReorgDetected",
    anchors: [
      'laz_dvn_jobs_total{status="REORG_DETECTED"} > 0',
      "severity: page",
    ],
  },
  {
    alert: "LazPacketManualReview",
    anchors: [
      'laz_packets_total{status="MANUAL_REVIEW"} > 0',
      "severity: ticket",
    ],
  },
  {
    alert: "LazExecutorReceiveFailed",
    anchors: [
      'laz_executor_jobs_total{status="LZ_RECEIVE_FAILED"} > 0',
      "severity: ticket",
    ],
  },
  {
    alert: "LazWorkerManualReview",
    anchors: [
      'laz_executor_jobs_total{status="MANUAL_REVIEW"} > 0 or laz_dvn_jobs_total{status="MANUAL_REVIEW"} > 0',
      "severity: ticket",
    ],
  },
  {
    alert: "LazTxOutboxFailed",
    anchors: [
      'laz_tx_outbox_total{status="failed",retry_state="exhausted"} > 0',
      "severity: ticket",
    ],
  },
  {
    alert: "LazIndexerPollFailing",
    anchors: ["laz_indexer_poll_success == 0", "severity: page"],
  },
  {
    alert: "LazIndexerCursorStalled",
    anchors: [
      "changes(laz_indexer_cursor_last_block[10m]) == 0",
      "severity: page",
    ],
  },
];

export function validateRunbookReview(): string[] {
  const documents = new Map<string, string>();
  const errors: string[] = [];
  for (const doc of requiredDocs) {
    try {
      documents.set(doc.path, readFileSync(doc.path, "utf8"));
    } catch (error) {
      errors.push(`${doc.path}: cannot read file: ${(error as Error).message}`);
    }
  }
  try {
    documents.set(
      "docs/monitoring/prometheus-alerts.yml",
      readFileSync("docs/monitoring/prometheus-alerts.yml", "utf8"),
    );
  } catch (error) {
    errors.push(
      `docs/monitoring/prometheus-alerts.yml: cannot read file: ${(error as Error).message}`,
    );
  }
  if (errors.length > 0) {
    return errors;
  }
  return validateRunbookDocuments(documents);
}

export function validateRunbookDocuments(
  documents: Map<string, string>,
): string[] {
  const errors: string[] = [];
  for (const doc of requiredDocs) {
    const body = documents.get(doc.path);
    if (body === undefined) {
      errors.push(`${doc.path}: cannot read file: missing document body`);
      continue;
    }
    for (const anchor of doc.anchors) {
      if (!body.includes(anchor)) {
        errors.push(
          `${doc.path}: missing required anchor ${JSON.stringify(anchor)}`,
        );
      }
    }
  }
  validateAlertRules(documents, errors);
  return errors;
}

function validateAlertRules(
  documents: Map<string, string>,
  errors: string[],
): void {
  const path = "docs/monitoring/prometheus-alerts.yml";
  const body = documents.get(path);
  if (body === undefined) {
    errors.push(`${path}: cannot read file: missing document body`);
    return;
  }
  for (const rule of requiredAlertRules) {
    if (!body.includes(`alert: ${rule.alert}`)) {
      errors.push(`${path}: missing alert ${rule.alert}`);
      continue;
    }
    for (const anchor of rule.anchors) {
      if (!body.includes(anchor)) {
        errors.push(
          `${path}: alert ${rule.alert} missing required anchor ${JSON.stringify(anchor)}`,
        );
      }
    }
  }
  if (!body.includes("runbook: docs/runbooks/monitoring.md")) {
    errors.push(`${path}: alert annotations must link the monitoring runbook`);
  }
}

function main(): void {
  const errors = validateRunbookReview();
  if (errors.length > 0) {
    throw new Error(`runbook review check failed:\n${errors.join("\n")}`);
  }
  console.log(
    `runbook review ok: ${requiredDocs.length} documents and ${requiredAlertRules.length} alert rules checked`,
  );
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}
