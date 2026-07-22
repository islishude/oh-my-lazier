import hre from "hardhat";
import OpenWorkersModule from "../../ignition/modules/OpenWorkers.js";
import { runCommand } from "../command-harness.js";
import { runIgnitionModuleCommand } from "./ignition-command.js";

await runCommand(() =>
  runIgnitionModuleCommand({
    hre,
    command: "deploy:open-workers",
    module: OpenWorkersModule,
    supportsSourceVerification: true,
  })
);
