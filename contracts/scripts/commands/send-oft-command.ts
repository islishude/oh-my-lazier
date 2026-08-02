import hre from "hardhat";
import { buildSendOFTPlan, sendOFT } from "../oft-send-runner.js";
import {
  assertExpectedSigner,
  createApplyGate,
  loadScriptRunFile,
  withWriteConnection,
} from "../command-harness.js";
import { jsonStringify } from "../lib.js";
import {
  addressField,
  bigintField,
  optionalAddressField,
  optionalBigintField,
  optionalHexField,
  parseInputObject,
  uint32Field,
} from "./input-parsers.js";
import {
  buildArtifacts,
  chainClients,
  requireApplyFlag,
  requiredNetwork,
} from "./runtime.js";

export async function runSendOFTCommand(label = "TestOFT.send"): Promise<void> {
  const runFile = loadScriptRunFile((value, inputLabel) => {
    const input = parseInputObject(value, inputLabel, [
      "testOFT",
      "recipient",
      "dstEid",
      "amountLD",
      "minAmountLD",
      "lzReceiveGas",
      "extraOptions",
      "refundAddress",
      "expectedSigner",
    ]);
    return {
      testOFT: addressField(input, "testOFT", inputLabel),
      recipient: addressField(input, "recipient", inputLabel),
      dstEid: uint32Field(input, "dstEid", inputLabel),
      amountLD: bigintField(input, "amountLD", inputLabel),
      minAmountLD: optionalBigintField(input, "minAmountLD", inputLabel),
      lzReceiveGas: optionalBigintField(input, "lzReceiveGas", inputLabel),
      extraOptions: optionalHexField(input, "extraOptions", inputLabel),
      refundAddress: optionalAddressField(input, "refundAddress", inputLabel),
      expectedSigner: addressField(input, "expectedSigner", inputLabel),
    };
  });
  const apply = requireApplyFlag(runFile);
  const network = requiredNetwork(hre);
  const plan = buildSendOFTPlan(runFile.input);
  if (!apply) {
    console.log(jsonStringify({ applied: false, network, plan }));
    return;
  }
  await buildArtifacts(hre);
  const gate = createApplyGate(runFile);
  await withWriteConnection(hre, { network }, async (context) => {
    assertExpectedSigner(context.signerAddress, runFile.input.expectedSigner);
    await gate.authorize({
      command: label,
      targets: [{ network: context.networkName, chainId: context.chainId }],
      actions: ["send TestOFT message"],
    });
    await sendOFT(label, runFile.input, chainClients(context));
  });
}
