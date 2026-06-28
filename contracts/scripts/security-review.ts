import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

type RequiredSecurityDoc = {
  path: string;
  anchors: string[];
};

const requiredDocs: RequiredSecurityDoc[] = [
  {
    path: "docs/security/security-review.md",
    anchors: [
      "release-readiness artifact, not a",
      "mainnet approval",
      "make check",
      "make security-check",
      "npm run check:security-review",
      "npm run check:npm-audit-disposition",
      "secret logging guard",
      "zero critical npm findings",
      "govulncheck` currently reports no vulnerabilities in called Go code.",
      "Private key material, decrypted keystore JSON, KMS signatures, and raw secrets",
      "Phase 1 remains EVM-only and scoped to Ethereum Sepolia <-> Base Sepolia.",
      "Self-only DVN is rejected",
      "Confirmations are fixed at 12 unless the top-level plan is updated.",
      "Source head conflicts pause chains.",
      "Receipt and log conflicts pause pathways.",
      "Migration evidence must pass `npm run check:migration-evidence`.",
      "### S-001: Open npm audit high and moderate toolchain advisories",
      "Severity: release blocker for mainnet approval.",
      "No final exhaustive security review or equivalent human approval has been",
      "No funded testnet deployment, canary transfer, DVN join, or rollback evidence",
      "M9 must remain in progress until S-001 and the external migration evidence",
    ],
  },
  {
    path: "docs/security/npm-audit-disposition.md",
    anchors: [
      "It is not final mainnet approval.",
      "npm run check:npm-audit-disposition",
      "make security-check",
      "zero critical findings",
      "every high or moderate finding to be present in the recorded disposition set",
      "Critical npm findings are closed for the current dependency graph.",
      "High and moderate findings remain open release-readiness items.",
      "Do not apply npm's suggested LayerZero downgrade automatically",
      "Mainnet readiness requires one of:",
    ],
  },
];

const forbiddenPatterns: Array<[RegExp, string]> = [
  [/\/Users\//, "local user paths"],
  [/\.codex\//, "tool-cache paths"],
  [/rollout-[0-9]{4}-[0-9]{2}-[0-9]{2}/, "personal rollout logs"],
];

const logCallPattern =
  /\b(logger|slog|log)\.(Debug|Info|Warn|Error|Printf|Println|Print)\b|console\.log\b/;

const forbiddenLogSecretPattern =
  /\b(private[_-]?key|secret|password|signature|api[_-]?key|access[_-]?key|session[_-]?token|keystore|credential)s?\b/i;

const logScanRoots = ["go", "contracts/scripts"];
const logScanExtensions = new Set([".go", ".ts"]);

export function validateSecurityReview(): string[] {
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
    for (const [pattern, description] of forbiddenPatterns) {
      if (pattern.test(body)) {
        errors.push(`${doc.path}: must not contain ${description}`);
      }
    }
  }
  errors.push(...validateNoSecretLogging());
  return errors;
}

function validateNoSecretLogging(): string[] {
  const sources = new Map<string, string>();
  for (const file of scanSourceFiles(logScanRoots)) {
    sources.set(file, readFileSync(file, "utf8"));
  }
  return findSecretLoggingErrors(sources);
}

export function findSecretLoggingErrors(
  sources: Map<string, string>,
): string[] {
  const errors: string[] = [];
  for (const [file, source] of sources) {
    const lines = source.split(/\r?\n/);
    lines.forEach((line, index) => {
      if (!logCallPattern.test(line)) {
        return;
      }
      if (!forbiddenLogSecretPattern.test(line)) {
        return;
      }
      errors.push(
        `${file}:${index + 1}: log call mentions secret-bearing material`,
      );
    });
  }
  return errors;
}

function scanSourceFiles(roots: string[]): string[] {
  const files: string[] = [];
  const visit = (path: string): void => {
    const stat = statSync(path);
    if (stat.isDirectory()) {
      if (path.includes("node_modules") || path.includes("artifacts")) {
        return;
      }
      for (const entry of readdirSync(path)) {
        visit(join(path, entry));
      }
      return;
    }
    for (const extension of logScanExtensions) {
      if (path.endsWith(extension)) {
        if (path.endsWith(".test.ts") || path.endsWith("_test.go")) {
          return;
        }
        files.push(path);
        return;
      }
    }
  };
  for (const root of roots) {
    visit(root);
  }
  return files;
}

function main(): void {
  const errors = validateSecurityReview();
  if (errors.length > 0) {
    throw new Error(`security review check failed:\n${errors.join("\n")}`);
  }
  console.log(`security review ok: ${requiredDocs.length} documents checked`);
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}
