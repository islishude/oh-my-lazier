import { checkMigrationEvidence } from "../migration-evidence.js";
import { loadScriptRunFile } from "../command-harness.js";
import { parseInputObject, stringField } from "../commands/input-parsers.js";

export type MigrationEvidenceCommandInput = {
  migrationEvidence: string;
};

export function parseMigrationEvidenceCommandInput(
  value: unknown,
  label: string
): MigrationEvidenceCommandInput {
  const input = parseInputObject(value, label, ["migrationEvidence"]);
  return {
    migrationEvidence: stringField(input, "migrationEvidence", label),
  };
}

export function runMigrationEvidenceCommand(): void {
  const runFile = loadScriptRunFile(parseMigrationEvidenceCommandInput);
  checkMigrationEvidence(runFile.input.migrationEvidence);
}
