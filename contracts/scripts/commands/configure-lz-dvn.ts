import hre from "hardhat";
import { runConfigureLzDVNCommand } from "../command-cores/configure-lz-dvn.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runConfigureLzDVNCommand(hre));
