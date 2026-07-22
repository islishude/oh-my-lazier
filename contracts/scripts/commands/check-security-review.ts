import { checkSecurityReview } from "../security-review.js";
import { loadScriptRunFile, runCommand } from "../command-harness.js";
import { parseEmptyInput } from "./input-parsers.js";

await runCommand(() => {
  loadScriptRunFile(parseEmptyInput, { allowMissing: true });
  checkSecurityReview();
});
