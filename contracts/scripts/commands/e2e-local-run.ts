import hre from "hardhat";
import { runLocalE2ERunCommand } from "../command-cores/e2e-local-run.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runLocalE2ERunCommand(hre));
