import hre from "hardhat";
import { runInspectLzConfigCommand } from "../command-cores/inspect-lz-config.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runInspectLzConfigCommand(hre));
