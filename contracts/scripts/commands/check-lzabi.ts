import hre from "hardhat";
import { generateLzABI } from "../generate-lzabi.js";
import { loadScriptRunFile, runCommand } from "../command-harness.js";
import { parseEmptyInput } from "./input-parsers.js";
import { buildArtifacts } from "./runtime.js";

await runCommand(async () => {
  loadScriptRunFile(parseEmptyInput, { allowMissing: true });
  await buildArtifacts(hre);
  await generateLzABI(true);
});
