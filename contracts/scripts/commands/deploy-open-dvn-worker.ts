import hre from "hardhat";
import OpenDVNWorkerModule from "../../../ignition/modules/OpenDVNWorker.js";
import { runCommand } from "../command-harness.js";
import { runIgnitionModuleCommand } from "./ignition-command.js";

await runCommand(() =>
  runIgnitionModuleCommand({
    hre,
    command: "deploy:open-dvn-worker",
    module: OpenDVNWorkerModule,
    supportsSourceVerification: true,
  })
);
