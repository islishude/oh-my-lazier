import assert from "node:assert/strict";
import test from "node:test";
import { validateNPMAuditDisposition } from "../../scripts/npm-audit-disposition.js";
import {
  findSecretLoggingErrors,
  validateSecurityReview,
} from "../../scripts/security-review.js";

type AuditReport = Parameters<typeof validateNPMAuditDisposition>[0];

function disposedAuditReport(): AuditReport {
  const vulnerabilities = Object.fromEntries(
    [
      "@chainlink/contracts-ccip",
      "@layerzerolabs/lz-evm-messagelib-v2",
      "@layerzerolabs/lz-evm-oapp-v2",
      "@openzeppelin/contracts",
      "@openzeppelin/contracts-upgradeable",
    ].map((name) => [name, { name, severity: "high" as const }])
  );
  return {
    vulnerabilities,
    metadata: { vulnerabilities: { critical: 0, high: 5, moderate: 0, total: 5 } },
  };
}

const auditCases: Array<{
  name: string;
  change: (report: AuditReport) => void;
  error?: RegExp;
}> = [
  { name: "accepts exactly disposed findings", change: () => {} },
  {
    name: "accepts unrelated low findings",
    change: (report) => {
      report.vulnerabilities!.other = { name: "other", severity: "low" };
    },
  },
  ...["@chainlink/contracts-ccip", "@openzeppelin/contracts"].map((name) => ({
    name: `rejects resolved disposition for ${name}`,
    change: (report: AuditReport) => {
      delete report.vulnerabilities![name];
    },
    error: new RegExp(`${name}: no high or moderate finding remains; remove from allowedOpenFindings`),
  })),
  ...(["low", "info"] as const).map((severity) => ({
    name: `rejects disposition downgraded to ${severity}`,
    change: (report: AuditReport) => {
      report.vulnerabilities!["@chainlink/contracts-ccip"].severity = severity;
    },
    error: /@chainlink\/contracts-ccip: no high or moderate finding remains; remove from allowedOpenFindings/,
  })),
  {
    name: "rejects stale dispositions when all findings are resolved",
    change: (report) => {
      report.vulnerabilities = {};
      report.metadata = { vulnerabilities: { total: 0, critical: 0 } };
    },
    error: /@chainlink\/contracts-ccip: no high or moderate finding remains; remove from allowedOpenFindings/,
  },
  {
    name: "rejects changed severity",
    change: (report) => {
      report.vulnerabilities!["@chainlink/contracts-ccip"].severity = "moderate";
    },
    error: /@chainlink\/contracts-ccip: severity changed from high to moderate/,
  },
  ...(["high", "moderate"] as const).flatMap((severity) =>
    ["undici", "@actions/http-client"].map((name) => ({
      name: `rejects reintroduced ${severity} finding for ${name}`,
      change: (report: AuditReport) => {
        report.vulnerabilities![name] = { name, severity };
      },
      error: new RegExp(`${name}: undisposed ${severity} finding`),
    }))
  ),
  {
    name: "rejects undisposed findings",
    change: (report) => {
      report.vulnerabilities!.other = { name: "other", severity: "high" };
    },
    error: /other: undisposed high finding/,
  },
  {
    name: "rejects critical findings",
    change: (report) => {
      report.metadata!.vulnerabilities!.critical = 1;
    },
    error: /npm audit critical vulnerabilities: 1/,
  },
  {
    name: "rejects missing metadata",
    change: (report) => {
      delete report.metadata;
    },
    error: /npm audit did not return a vulnerability report/,
  },
  {
    name: "rejects missing vulnerabilities",
    change: (report) => {
      delete report.vulnerabilities;
    },
    error: /npm audit did not return a vulnerability report/,
  },
];

for (const { name, change, error } of auditCases) {
  test(`npm audit disposition ${name}`, () => {
    const report = disposedAuditReport();
    change(report);
    if (error) {
      assert.throws(() => validateNPMAuditDisposition(report), error);
    } else {
      assert.doesNotThrow(() => validateNPMAuditDisposition(report));
    }
  });
}

test("security review check accepts current repository documents and logs", () => {
  assert.deepEqual(validateSecurityReview(), []);
});

test("secret logging guard rejects secret-bearing log calls", () => {
  const sources = new Map([
    [
      "go/internal/example.go",
      'logger.Info("loaded signer", "private_key", privateKey)\n',
    ],
    [
      "contracts/scripts/example.ts",
      'console.log("kms signature", signature);\n',
    ],
  ]);

  assert.deepEqual(findSecretLoggingErrors(sources), [
    "go/internal/example.go:1: log call mentions secret-bearing material",
    "contracts/scripts/example.ts:1: log call mentions secret-bearing material",
  ]);
});

test("secret logging guard allows non-log security validation text", () => {
  const sources = new Map([
    [
      "go/internal/example.go",
      'return errors.New("keystore password source is required")\n',
    ],
  ]);

  assert.deepEqual(findSecretLoggingErrors(sources), []);
});
