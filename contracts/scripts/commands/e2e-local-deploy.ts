import hre from "hardhat";
import { runLocalE2EDeployCommand } from "../command-cores/e2e-local-deploy.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runLocalE2EDeployCommand(hre));
