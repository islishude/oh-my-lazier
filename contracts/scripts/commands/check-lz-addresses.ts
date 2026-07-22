import { checkLayerZeroAddresses } from "../check-lz-addresses.js";
import { loadScriptRunFile, runCommand } from "../command-harness.js";
import { parseEmptyInput } from "./input-parsers.js";

await runCommand(async () => {
  loadScriptRunFile(parseEmptyInput, { allowMissing: true });
  await checkLayerZeroAddresses();
});
