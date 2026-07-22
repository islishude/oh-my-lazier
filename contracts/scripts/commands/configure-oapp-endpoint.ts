import hre from "hardhat";
import OAppEndpointConfigModule from "../../../ignition/modules/OAppEndpointConfig.js";
import { runCommand } from "../command-harness.js";
import { runIgnitionModuleCommand } from "./ignition-command.js";

await runCommand(() =>
  runIgnitionModuleCommand({
    hre,
    command: "configure:oapp-endpoint",
    module: OAppEndpointConfigModule,
  })
);
