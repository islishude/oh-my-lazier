import hre from "hardhat";
import { runConfigureLzExecutorCommand } from "../command-cores/configure-lz-executor.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runConfigureLzExecutorCommand(hre));
