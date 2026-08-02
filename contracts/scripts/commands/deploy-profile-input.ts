import type { DeployProfileInput, DeploymentPhase } from "../deploy-profile.js";
import {
  enumField,
  optionalBooleanField,
  parseInputObject,
  stringField,
} from "./input-parsers.js";

const deploymentPhases = [
  "render",
  "deploy-test-oft",
  "deploy-workers",
  "configure-workers",
  "configure-oapp",
  "verify",
  "all",
] as const satisfies readonly DeploymentPhase[];

export function parseDeployProfileInput(
  value: unknown,
  label: string
): DeployProfileInput {
  const input = parseInputObject(value, label, [
    "profilePath",
    "outDir",
    "phase",
    "verifySource",
  ]);
  return {
    profilePath: stringField(input, "profilePath", label),
    outDir: stringField(input, "outDir", label),
    phase: enumField(input, "phase", deploymentPhases, label),
    verifySource: optionalBooleanField(input, "verifySource", label),
  };
}
