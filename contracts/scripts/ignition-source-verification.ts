import { spawn } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { getVerificationInformation } from "@nomicfoundation/ignition-core";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";

const IGNITION_DEPLOYMENT_ID = /^[a-zA-Z][a-zA-Z0-9_-]*$/;
const DEFAULT_VERIFICATION_CONFIG = "hardhat.verify.config.ts";

export type IgnitionSourceVerificationTarget = {
  network: string;
  deploymentId: string;
};

export type IgnitionSourceVerificationRequest = {
  hre: HardhatRuntimeEnvironment;
  targets: readonly IgnitionSourceVerificationTarget[];
  buildProfile: string;
};

export type VerificationProcessRequest = {
  executable: string;
  args: readonly string[];
  cwd: string;
  environment: NodeJS.ProcessEnv;
};

export type VerificationProcessResult = {
  exitCode: number | null;
  signal: NodeJS.Signals | null;
};

export type IgnitionSourceVerificationDependencies = {
  runProcess?: (
    request: VerificationProcessRequest
  ) => Promise<VerificationProcessResult>;
  inspectDeployment?: (deploymentDir: string) => Promise<number>;
  hardhatCliPath?: string;
  configPath?: string;
  environment?: NodeJS.ProcessEnv;
};

/**
 * Verify one or more Ignition deployments in isolated, read-only Hardhat
 * subprocesses. Targets are intentionally processed serially so provider
 * failures stop the stage before another verification request is submitted.
 */
export async function verifyIgnitionDeploymentSources(
  request: IgnitionSourceVerificationRequest,
  dependencies: IgnitionSourceVerificationDependencies = {}
): Promise<void> {
  validateVerificationRequest(request);

  const projectRoot = request.hre.config.paths.root;
  const configPath = resolve(
    dependencies.configPath ?? join(projectRoot, DEFAULT_VERIFICATION_CONFIG)
  );
  if (!existsSync(configPath)) {
    throw new Error(
      `Hardhat verification config does not exist: ${configPath}`
    );
  }

  const inspectDeployment =
    dependencies.inspectDeployment ?? countResolvableContracts;
  const preparedTargets = [];
  for (const target of request.targets) {
    const deploymentDir = join(
      request.hre.config.paths.ignition,
      "deployments",
      target.deploymentId
    );
    let resolvableContracts: number;
    try {
      resolvableContracts = await inspectDeployment(deploymentDir);
    } catch {
      throw new Error(
        `Ignition deployment ${target.deploymentId} cannot be read for source verification`
      );
    }
    if (resolvableContracts < 1) {
      throw new Error(
        `Ignition deployment ${target.deploymentId} has no resolvable contracts to verify`
      );
    }
    preparedTargets.push(target);
  }

  const hardhatCliPath = dependencies.hardhatCliPath ?? resolveHardhatCliPath();
  if (!existsSync(hardhatCliPath)) {
    throw new Error(`Hardhat CLI does not exist: ${hardhatCliPath}`);
  }
  const runProcess = dependencies.runProcess ?? runVerificationProcess;
  const environment = dependencies.environment ?? process.env;

  for (const target of preparedTargets) {
    const args = [
      hardhatCliPath,
      "--config",
      configPath,
      "--network",
      target.network,
      "--build-profile",
      request.buildProfile,
      "ignition",
      "verify",
      target.deploymentId,
    ];

    let result: VerificationProcessResult;
    try {
      result = await runProcess({
        executable: process.execPath,
        args,
        cwd: projectRoot,
        environment,
      });
    } catch {
      throw new Error(
        `failed to start source verification for Ignition deployment ${target.deploymentId} on ${target.network}`
      );
    }

    if (result.signal !== null) {
      throw new Error(
        `source verification for Ignition deployment ${target.deploymentId} on ${target.network} terminated by signal ${result.signal}`
      );
    }
    if (result.exitCode !== 0) {
      throw new Error(
        `source verification for Ignition deployment ${
          target.deploymentId
        } on ${target.network} failed with exit code ${String(result.exitCode)}`
      );
    }
  }
}

function validateVerificationRequest(
  request: IgnitionSourceVerificationRequest
): void {
  const providers = request.hre.config.verify;
  if (
    !providers.etherscan.enabled &&
    !providers.blockscout.enabled &&
    !providers.sourcify.enabled
  ) {
    throw new Error("source verification requires an enabled provider");
  }
  if (request.buildProfile.trim() === "" || /\s/.test(request.buildProfile)) {
    throw new Error("source verification requires a valid build profile");
  }
  if (!(request.buildProfile in request.hre.config.solidity.profiles)) {
    throw new Error(
      `Hardhat build profile ${request.buildProfile} is not configured`
    );
  }
  if (request.targets.length === 0) {
    throw new Error("source verification requires at least one deployment");
  }

  for (const target of request.targets) {
    if (!IGNITION_DEPLOYMENT_ID.test(target.deploymentId)) {
      throw new Error(
        `invalid Ignition deployment id for source verification: ${target.deploymentId}`
      );
    }
    const network = request.hre.config.networks[target.network];
    if (network === undefined) {
      throw new Error(`Hardhat network ${target.network} is not configured`);
    }
    if (network.type !== "http") {
      throw new Error(
        `Hardhat network ${target.network} must be an HTTP network for source verification`
      );
    }
    if (network.chainId === undefined) {
      throw new Error(
        `Hardhat network ${target.network} must configure chainId for source verification`
      );
    }
  }
}

async function countResolvableContracts(
  deploymentDir: string
): Promise<number> {
  let count = 0;
  for await (const verification of getVerificationInformation(deploymentDir)) {
    if (typeof verification !== "string") {
      count += 1;
    }
  }
  return count;
}

function resolveHardhatCliPath(): string {
  const require = createRequire(import.meta.url);
  const packagePath = require.resolve("hardhat/package.json");
  const packageJson = JSON.parse(readFileSync(packagePath, "utf8")) as {
    bin?: string | Record<string, string>;
  };
  const bin =
    typeof packageJson.bin === "string"
      ? packageJson.bin
      : packageJson.bin?.hardhat;
  if (bin === undefined || bin.trim() === "") {
    throw new Error("installed Hardhat package does not declare a CLI");
  }
  return resolve(dirname(packagePath), bin);
}

function runVerificationProcess(
  request: VerificationProcessRequest
): Promise<VerificationProcessResult> {
  return new Promise((resolveProcess, rejectProcess) => {
    const child = spawn(request.executable, [...request.args], {
      cwd: request.cwd,
      env: request.environment,
      shell: false,
      // Preserve command stdout for its final machine-readable JSON result.
      // Verification diagnostics from both child streams are operational logs.
      stdio: ["inherit", 2, 2],
    });
    child.once("error", rejectProcess);
    child.once("close", (exitCode, signal) => {
      resolveProcess({ exitCode, signal });
    });
  });
}
