import hre from "hardhat";
import { runPriceConfigCommand } from "../command-cores/check-price-config.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runPriceConfigCommand(hre));
