import hre from "hardhat";
import OpenDVNPathwayConfigModule from "../../../ignition/modules/OpenDVNPathwayConfig.js";
import { runCommand } from "../command-harness.js";
import { runIgnitionModuleCommand } from "./ignition-command.js";

await runCommand(() =>
  runIgnitionModuleCommand({
    hre,
    command: "configure:open-dvn-pathway",
    module: OpenDVNPathwayConfigModule,
  })
);
