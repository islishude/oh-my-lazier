import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  checkDVNVerification,
  type CheckDVNVerificationInput,
} from "../check-dvn-verification.js";
import {
  loadScriptRunFile,
  withReadOnlyConnection,
} from "../command-harness.js";
import {
  addressArrayField,
  addressField,
  bigintField,
  hexField,
  optionalAddressField,
  optionalBigintField,
  optionalHexField,
  optionalUint32Field,
  parseInputObject,
} from "../commands/input-parsers.js";
import { requiredNetwork } from "../commands/runtime.js";

export function parseCheckDVNVerificationCommandInput(
  value: unknown,
  label: string
): CheckDVNVerificationInput {
  const input = parseInputObject(value, label, [
    "txHash",
    "receiveUln",
    "requiredDVNs",
    "confirmations",
    "endpoint",
    "expectedPayloadHash",
    "expectedSrcEid",
    "expectedDstEid",
    "expectedNonce",
    "expectedSender",
    "expectedReceiver",
  ]);
  return {
    txHash: hexField(input, "txHash", label),
    receiveUln: addressField(input, "receiveUln", label),
    requiredDVNs: addressArrayField(input, "requiredDVNs", label),
    confirmations: bigintField(input, "confirmations", label),
    endpoint: optionalAddressField(input, "endpoint", label),
    expectedPayloadHash: optionalHexField(
      input,
      "expectedPayloadHash",
      label
    ),
    expectedSrcEid: optionalUint32Field(input, "expectedSrcEid", label),
    expectedDstEid: optionalUint32Field(input, "expectedDstEid", label),
    expectedNonce: optionalBigintField(input, "expectedNonce", label),
    expectedSender: optionalAddressField(input, "expectedSender", label),
    expectedReceiver: optionalAddressField(input, "expectedReceiver", label),
  };
}

export async function runCheckDVNVerificationCommand(
  hre: HardhatRuntimeEnvironment
): Promise<void> {
  const runFile = loadScriptRunFile(parseCheckDVNVerificationCommandInput);
  const network = requiredNetwork(hre);
  await withReadOnlyConnection(hre, { network }, ({ publicClient }) =>
    checkDVNVerification(runFile.input, publicClient)
  );
}
