import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { ScriptRunFile, WriteNetworkContext } from "../command-harness.js";
import type { ChainClients } from "../lib.js";

export function requiredNetwork(hre: HardhatRuntimeEnvironment): string {
  const network = hre.globalOptions.network;
  if (network === undefined || network === "hardhat") {
    throw new Error("a named HTTP network must be selected with --network");
  }
  return network;
}

export function requireApplyFlag(
  runFile: Pick<ScriptRunFile<unknown>, "apply">
): boolean {
  if (runFile.apply === undefined) {
    throw new Error("script parameters.apply is required for this command");
  }
  return runFile.apply;
}

/** Build project artifacts quietly before a command loads or consumes them. */
export async function buildArtifacts(
  hre: HardhatRuntimeEnvironment,
  buildProfile = hre.globalOptions.buildProfile ?? "default"
): Promise<void> {
  await hre.tasks.getTask(["build"]).run({
    force: false,
    files: [],
    quiet: true,
    defaultBuildProfile: buildProfile,
    noTests: true,
    noContracts: false,
  });
}

export function chainClients(context: WriteNetworkContext): ChainClients {
  const account = context.walletClient.account;
  if (account === undefined) {
    throw new Error(
      `Hardhat network ${context.networkName} wallet has no account`
    );
  }
  return {
    account,
    publicClient: context.publicClient,
    walletClient: context.walletClient,
  };
}
