import hre from "hardhat";
import TestOFTModule from "../../ignition/modules/TestOFT.js";
import { runCommand } from "../command-harness.js";
import { runIgnitionModuleCommand } from "./ignition-command.js";

await runCommand(() =>
  runIgnitionModuleCommand({
    hre,
    command: "deploy:test-oft",
    module: TestOFTModule,
    supportsSourceVerification: true,
  })
);
