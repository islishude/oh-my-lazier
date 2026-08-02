import hre from "hardhat";
import { runConfigureLzRollbackCommand } from "../command-cores/configure-lz-rollback.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runConfigureLzRollbackCommand(hre));
