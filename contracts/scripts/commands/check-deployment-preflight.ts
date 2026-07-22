import hre from "hardhat";
import { runDeploymentPreflightCommand } from "../command-cores/check-deployment-preflight.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runDeploymentPreflightCommand(hre));
