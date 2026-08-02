import hre from "hardhat";
import { runCheckOFTCanaryCommand } from "../command-cores/check-oft-canary.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runCheckOFTCanaryCommand(hre));
