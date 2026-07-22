import hre from "hardhat";
import { runOFTPathwayCommand } from "../command-cores/oft-pathway.js";
import { runCommand } from "../command-harness.js";

await runCommand(() => runOFTPathwayCommand(hre));
