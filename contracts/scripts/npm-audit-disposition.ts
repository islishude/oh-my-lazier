import { execFileSync } from "node:child_process";

type AuditVulnerability = {
  name: string;
  severity: "info" | "low" | "moderate" | "high" | "critical";
  isDirect?: boolean;
};

type AuditReport = {
  vulnerabilities?: Record<string, AuditVulnerability>;
  metadata?: {
    vulnerabilities?: Partial<
      Record<AuditVulnerability["severity"] | "total", number>
    >;
  };
  message?: string;
};

const allowedOpenFindings = new Map<string, AuditVulnerability["severity"]>([
  ["@chainlink/contracts-ccip", "high"],
  ["@layerzerolabs/lz-evm-messagelib-v2", "high"],
  ["@layerzerolabs/lz-evm-oapp-v2", "high"],
  ["@openzeppelin/contracts", "high"],
  ["@openzeppelin/contracts-upgradeable", "high"],
]);

function runAudit(): AuditReport {
  try {
    return JSON.parse(
      execFileSync("npm", ["audit", "--audit-level=moderate", "--json"], {
        encoding: "utf8",
      })
    );
  } catch (error) {
    const output = (error as { stdout?: Buffer | string }).stdout;
    if (!output) {
      throw error;
    }
    return JSON.parse(output.toString());
  }
}

export function validateNPMAuditDisposition(report: AuditReport): {
  disposedFindings: number;
  total: number;
} {
  const counts = report.metadata?.vulnerabilities;
  if (!counts || !report.vulnerabilities) {
    throw new Error(
      `npm audit did not return a vulnerability report: ${report.message ?? "missing metadata"
      }`
    );
  }

  const critical = counts.critical ?? 0;
  if (critical !== 0) {
    throw new Error(`npm audit critical vulnerabilities: ${critical}`);
  }

  const errors: string[] = [];
  const openModerateOrHigh = new Set<string>();
  for (const vulnerability of Object.values(report.vulnerabilities)) {
    if (
      vulnerability.severity !== "high" &&
      vulnerability.severity !== "moderate"
    ) {
      continue;
    }
    openModerateOrHigh.add(vulnerability.name);
    const expectedSeverity = allowedOpenFindings.get(vulnerability.name);
    if (!expectedSeverity) {
      errors.push(
        `${vulnerability.name}: undisposed ${vulnerability.severity} finding`
      );
      continue;
    }
    if (expectedSeverity !== vulnerability.severity) {
      errors.push(
        `${vulnerability.name}: severity changed from ${expectedSeverity} to ${vulnerability.severity}`
      );
    }
  }

  for (const name of allowedOpenFindings.keys()) {
    if (!openModerateOrHigh.has(name)) {
      errors.push(
        `${name}: no high or moderate finding remains; remove from allowedOpenFindings`
      );
    }
  }

  if (errors.length > 0) {
    throw new Error(
      `npm audit disposition check failed:\n${errors.join("\n")}`
    );
  }

  return { disposedFindings: openModerateOrHigh.size, total: counts.total ?? 0 };
}

export function checkNPMAuditDisposition(): void {
  const { disposedFindings, total } = validateNPMAuditDisposition(runAudit());

  console.log(
    `npm audit disposition ok: critical=0, disposed high/moderate findings=${disposedFindings}, total=${total}`
  );
}
