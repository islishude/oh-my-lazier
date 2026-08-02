import hre from "hardhat";
import { runConfigureWorkersCommand } from "../command-cores/configure-workers.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runConfigureWorkersCommand(hre));
