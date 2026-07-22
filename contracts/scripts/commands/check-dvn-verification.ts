import hre from "hardhat";
import { runCheckDVNVerificationCommand } from "../command-cores/check-dvn-verification.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runCheckDVNVerificationCommand(hre));
