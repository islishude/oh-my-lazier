import { readFileSync } from "node:fs";

type RequiredDoc = {
  path: string;
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
      "Confirmations must be 12 unless the top-level plan is explicitly updated.",
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
      "laz_chain_paused == 1",
      "laz_pathway_paused == 1",
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
      "For DVN migration, confirm the proposed config still uses `dvn.mode: shadow` until the explicit active-mode change is approved.",
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
      '"canary"',
      '"dvnJoin"',
      '"rollback"',
      '"manualRetryPlan"',
    ],
  },
];

const errors: string[] = [];
for (const doc of requiredDocs) {
  let body: string;
  try {
    body = readFileSync(doc.path, "utf8");
  } catch (error) {
    errors.push(`${doc.path}: cannot read file: ${(error as Error).message}`);
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

if (errors.length > 0) {
  throw new Error(`runbook review check failed:\n${errors.join("\n")}`);
}

console.log(`runbook review ok: ${requiredDocs.length} documents checked`);
