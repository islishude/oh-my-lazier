import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type {
  IgnitionModule,
  IgnitionModuleResult,
} from "@nomicfoundation/ignition-core";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  assertExpectedSigner,
  assertNoSecretFields,
  createApplyGate,
  expectObject,
  loadScriptRunFile,
  withWriteConnection,
} from "../command-harness.js";
import { verifyIgnitionDeploymentSources } from "../ignition-source-verification.js";
import { withIgnitionUiOnStderr } from "../ignition-ui-output.js";
import { jsonStringify } from "../lib.js";
import {
  optionalBooleanField,
  addressField,
  parseInputObject,
  stringField,
} from "./input-parsers.js";
import { requireApplyFlag, requiredNetwork } from "./runtime.js";

export async function runIgnitionModuleCommand<
  ModuleId extends string,
  ContractName extends string,
  Results extends IgnitionModuleResult<ContractName>
>(input: {
  hre: HardhatRuntimeEnvironment;
  command: string;
  module: IgnitionModule<ModuleId, ContractName, Results>;
  supportsSourceVerification?: boolean;
  verifyDeploymentSources?: typeof verifyIgnitionDeploymentSources;
}): Promise<void> {
  const runFile = loadScriptRunFile((value, label) => {
    const allowedFields = [
      "parameters",
      "deploymentId",
      "expectedSigner",
      ...(input.supportsSourceVerification === true ? ["verify"] : []),
    ];
    const params = parseInputObject(value, label, allowedFields);
    return {
      parameters: stringField(params, "parameters", label),
      deploymentId: stringField(params, "deploymentId", label),
      verify:
        input.supportsSourceVerification === true
          ? optionalBooleanField(params, "verify", label) ?? false
          : false,
      expectedSigner: addressField(params, "expectedSigner", label),
    };
  });
  const apply = requireApplyFlag(runFile);
  const network = requiredNetwork(input.hre);
  const buildProfile = input.hre.globalOptions.buildProfile ?? "production";
  const parameters = resolve(runFile.input.parameters);
  validateIgnitionCommandFiles(parameters, runFile.input.deploymentId);
  if (!apply) {
    console.log(
      jsonStringify({
        applied: false,
        command: input.command,
        network,
        deploymentId: runFile.input.deploymentId,
        parameters,
        verify: runFile.input.verify,
        buildProfile,
      })
    );
    return;
  }

  await input.hre.tasks.getTask(["build"]).run({
    force: false,
    files: [],
    quiet: true,
    defaultBuildProfile: buildProfile,
    noTests: true,
    noContracts: false,
  });

  const gate = createApplyGate(runFile);
  const deployment = await withWriteConnection(
    input.hre,
    { network },
    async (context) => {
      assertExpectedSigner(context.signerAddress, runFile.input.expectedSigner);
      await gate.authorize({
        command: input.command,
        targets: [
          {
            network: context.networkName,
            chainId: context.chainId,
            deploymentIds: [runFile.input.deploymentId],
          },
        ],
        actions: ["reconcile the fixed Ignition module"],
      });
      const result = await withIgnitionUiOnStderr(() =>
        context.connection.ignition.deploy(input.module, {
          parameters,
          deploymentId: runFile.input.deploymentId,
          displayUi: true,
        })
      );
      return {
        network: context.networkName,
        chainId: context.chainId,
        contracts: Object.fromEntries(
          Object.entries(result).map(([name, contract]) => [
            name,
            (contract as { address: string }).address,
          ])
        ),
      };
    }
  );

  if (runFile.input.verify) {
    const verifyDeploymentSources =
      input.verifyDeploymentSources ?? verifyIgnitionDeploymentSources;
    await verifyDeploymentSources({
      hre: input.hre,
      targets: [{ network, deploymentId: runFile.input.deploymentId }],
      buildProfile,
    });
  }

  console.log(
    jsonStringify({
      applied: true,
      network: deployment.network,
      chainId: deployment.chainId,
      deploymentId: runFile.input.deploymentId,
      contracts: deployment.contracts,
      verified: runFile.input.verify,
    })
  );
}

export function validateIgnitionCommandFiles(
  parametersPath: string,
  deploymentId: string
): void {
  if (!/^[a-zA-Z][a-zA-Z0-9_-]*$/.test(deploymentId)) {
    throw new Error(`invalid Ignition deployment id: ${deploymentId}`);
  }

  let source: string;
  try {
    source = readFileSync(parametersPath, "utf8");
  } catch {
    throw new Error(
      `Ignition parameters file could not be read: ${parametersPath}`
    );
  }

  let parameters: unknown;
  try {
    parameters = JSON.parse(source) as unknown;
  } catch {
    throw new Error(
      `Ignition parameters file contains invalid JSON: ${parametersPath}`
    );
  }
  assertNoSecretFields(parameters, "Ignition parameters");
  expectObject(parameters, "Ignition parameters");
}
