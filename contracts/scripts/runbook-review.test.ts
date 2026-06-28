import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  validateRunbookDocuments,
  validateRunbookReview,
} from "./runbook-review.js";

test("runbook review accepts current repository documents and alert rules", () => {
  assert.deepEqual(validateRunbookReview(), []);
});

test("runbook review rejects missing required alert rules", () => {
  const documents = currentDocuments();
  const alerts = documents.get("docs/monitoring/prometheus-alerts.yml");
  assert.ok(alerts);
  documents.set(
    "docs/monitoring/prometheus-alerts.yml",
    alerts.replace("alert: LazDVNQuorumConflict", "alert: MissingRule"),
  );

  assert.deepEqual(validateRunbookDocuments(documents), [
    "docs/monitoring/prometheus-alerts.yml: missing alert LazDVNQuorumConflict",
  ]);
});

test("runbook review rejects missing runbook anchors", () => {
  const documents = currentDocuments();
  const monitoring = documents.get("docs/runbooks/monitoring.md");
  assert.ok(monitoring);
  documents.set(
    "docs/runbooks/monitoring.md",
    monitoring.replace("LazWorkerReadinessFailed", "MissingReadinessAlert"),
  );

  assert.deepEqual(validateRunbookDocuments(documents), [
    'docs/runbooks/monitoring.md: missing required anchor "LazWorkerReadinessFailed"',
  ]);
});

function currentDocuments(): Map<string, string> {
  return new Map([
    [
      "docs/runbooks/mainnet-readiness.md",
      readFileSync("docs/runbooks/mainnet-readiness.md", "utf8"),
    ],
    [
      "docs/runbooks/monitoring.md",
      readFileSync("docs/runbooks/monitoring.md", "utf8"),
    ],
    [
      "docs/runbooks/config-diff.md",
      readFileSync("docs/runbooks/config-diff.md", "utf8"),
    ],
    [
      "docs/runbooks/key-management.md",
      readFileSync("docs/runbooks/key-management.md", "utf8"),
    ],
    [
      "docs/runbooks/price-bot.md",
      readFileSync("docs/runbooks/price-bot.md", "utf8"),
    ],
    [
      "docs/runbooks/rate-limit.md",
      readFileSync("docs/runbooks/rate-limit.md", "utf8"),
    ],
    [
      "docs/deployments/testnet-migration-evidence.example.json",
      readFileSync(
        "docs/deployments/testnet-migration-evidence.example.json",
        "utf8",
      ),
    ],
    [
      "docs/monitoring/prometheus-alerts.yml",
      readFileSync("docs/monitoring/prometheus-alerts.yml", "utf8"),
    ],
  ]);
}
