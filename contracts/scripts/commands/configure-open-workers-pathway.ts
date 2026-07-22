import hre from "hardhat";
import OpenWorkersPathwayConfigModule from "../../../ignition/modules/OpenWorkersPathwayConfig.js";
import { runCommand } from "../command-harness.js";
import { runIgnitionModuleCommand } from "./ignition-command.js";

await runCommand(() =>
  runIgnitionModuleCommand({
    hre,
    command: "configure:open-workers-pathway",
    module: OpenWorkersPathwayConfigModule,
  })
);
