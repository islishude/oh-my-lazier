import { loadScriptRunFile, runCommand } from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import { renderOftPathwayParams } from "../render-oft-pathway-params.js";
import { parseRenderOftPathwayParamsInput } from "./render-oft-pathway-params-input.js";

await runCommand(() => {
  const runFile = loadScriptRunFile(parseRenderOftPathwayParamsInput);
  console.log(jsonStringify(renderOftPathwayParams(runFile.input)));
});
