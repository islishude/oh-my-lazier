import { runCommand } from "../command-harness.js";
import { runSendOFTCommand } from "./send-oft-command.js";

await runCommand(() => runSendOFTCommand("TestOFT.send canary"));
