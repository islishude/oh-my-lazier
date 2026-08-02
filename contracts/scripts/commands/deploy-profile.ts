import hre from "hardhat";
import { runDeployProfileCommand } from "../command-cores/deploy-profile.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runDeployProfileCommand(hre));
