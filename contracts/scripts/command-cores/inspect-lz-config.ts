import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  inspectLzConfig,
  type InspectLzConfigInput,
} from "../inspect-lz-config.js";
import {
  loadScriptRunFile,
  withReadOnlyConnection,
} from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import {
  addressField,
  parseInputObject,
  uint32Field,
} from "../commands/input-parsers.js";
import { requiredNetwork } from "../commands/runtime.js";

export function parseInspectLzConfigCommandInput(
  value: unknown,
  label: string
): InspectLzConfigInput {
  const input = parseInputObject(value, label, [
    "endpoint",
    "oapp",
    "remoteEid",
    "sendUln",
    "receiveUln",
  ]);
  return {
    endpoint: addressField(input, "endpoint", label),
    oapp: addressField(input, "oapp", label),
    remoteEid: uint32Field(input, "remoteEid", label),
    sendUln: addressField(input, "sendUln", label),
    receiveUln: addressField(input, "receiveUln", label),
  };
}

export async function runInspectLzConfigCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseInspectLzConfigCommandInput);
  const network = requiredNetwork(hre);
  const report = await withReadOnlyConnection(
    hre,
    { network },
    ({ publicClient }) => inspectLzConfig(runFile.input, publicClient)
  );
  console.log(jsonStringify(report));
}
