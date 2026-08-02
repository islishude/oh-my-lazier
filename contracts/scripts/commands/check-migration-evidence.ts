import { runMigrationEvidenceCommand } from "../command-cores/check-migration-evidence.js";
import { runCommand } from "../command-harness.js";

await runCommand(runMigrationEvidenceCommand);
