import path from "node:path";
import {
  listDeployments,
  status,
  type StatusResult,
} from "@nomicfoundation/ignition-core";
import { getAddress, isAddress, type Address } from "viem";

export type IgnitionRuntime = {
  config: {
    paths: {
      ignition: string;
    };
  };
};

export type RequiredIgnitionContract = {
  futureId: string;
  contractName: string;
};

export type IgnitionDeploymentRequest = {
  deploymentId: string;
  expectedChainId: number;
  requiredContracts: readonly RequiredIgnitionContract[];
};

export type IgnitionContractState = {
  futureId: string;
  contractName: string;
  address: Address;
  sourceName: string;
};

export type MissingIgnitionDeployment = {
  kind: "missing";
  deploymentId: string;
  deploymentDir: string;
};

export type ReadyIgnitionDeployment = {
  kind: "ready";
  deploymentId: string;
  deploymentDir: string;
  chainId: number;
  contracts: Readonly<Record<string, IgnitionContractState>>;
};

export type IgnitionDeploymentState =
  | MissingIgnitionDeployment
  | ReadyIgnitionDeployment;

export type IgnitionDeploymentStateIO = {
  listDeployments(deploymentsDir: string): Promise<string[]>;
  status(deploymentDir: string): Promise<StatusResult>;
};

const defaultIO: IgnitionDeploymentStateIO = {
  listDeployments,
  status,
};

const deploymentIdPattern = /^[a-zA-Z][a-zA-Z0-9_-]*$/;

/** Returns the directory in which Ignition stores individual deployments. */
export function ignitionDeploymentsDir(hre: IgnitionRuntime): string {
  return path.join(hre.config.paths.ignition, "deployments");
}

/**
 * Reads and validates an Ignition deployment without parsing Ignition's
 * implementation-owned deployed_addresses.json file.
 *
 * A missing deployment is returned as a normal result so callers can enter a
 * bootstrap flow. Once a deployment directory exists, unreadable, incomplete,
 * or inconsistent state is always an error.
 */
export async function readIgnitionDeploymentState(
  hre: IgnitionRuntime,
  request: IgnitionDeploymentRequest,
  io: IgnitionDeploymentStateIO = defaultIO
): Promise<IgnitionDeploymentState> {
  validateRequest(request);

  const deploymentsDir = ignitionDeploymentsDir(hre);
  const deploymentDir = path.join(deploymentsDir, request.deploymentId);
  const deployments = await io.listDeployments(deploymentsDir);

  if (!deployments.includes(request.deploymentId)) {
    return {
      kind: "missing",
      deploymentId: request.deploymentId,
      deploymentDir,
    };
  }

  let deploymentStatus: StatusResult;
  try {
    deploymentStatus = await io.status(deploymentDir);
  } catch (error) {
    throw new Error(
      `Ignition deployment "${
        request.deploymentId
      }" at ${deploymentDir} cannot be read: ${errorMessage(error)}`,
      { cause: error }
    );
  }

  if (deploymentStatus.chainId !== request.expectedChainId) {
    throw new Error(
      `Ignition deployment "${request.deploymentId}" has chain ID ${deploymentStatus.chainId}, expected ${request.expectedChainId}`
    );
  }

  assertComplete(request.deploymentId, deploymentStatus);

  const contracts = normalizeContracts(request.deploymentId, deploymentStatus);
  for (const required of request.requiredContracts) {
    const contract = contracts[required.futureId];
    if (contract === undefined) {
      throw new Error(
        `Ignition deployment "${request.deploymentId}" is missing required future "${required.futureId}"`
      );
    }
    if (!deploymentStatus.successful.includes(required.futureId)) {
      throw new Error(
        `Ignition deployment "${request.deploymentId}" required future "${required.futureId}" is not successful`
      );
    }
    if (contract.contractName !== required.contractName) {
      throw new Error(
        `Ignition deployment "${request.deploymentId}" future "${required.futureId}" has contract name "${contract.contractName}", expected "${required.contractName}"`
      );
    }
  }

  return {
    kind: "ready",
    deploymentId: request.deploymentId,
    deploymentDir,
    chainId: deploymentStatus.chainId,
    contracts,
  };
}

function validateRequest(request: IgnitionDeploymentRequest): void {
  if (!deploymentIdPattern.test(request.deploymentId)) {
    throw new Error(
      `Invalid Ignition deployment ID "${request.deploymentId}"; expected a letter followed by letters, numbers, dashes, or underscores`
    );
  }
  if (
    !Number.isSafeInteger(request.expectedChainId) ||
    request.expectedChainId <= 0
  ) {
    throw new Error("expectedChainId must be a positive safe integer");
  }

  const futureIds = new Set<string>();
  for (const required of request.requiredContracts) {
    if (required.futureId === "") {
      throw new Error("Required Ignition future ID must not be empty");
    }
    if (required.contractName === "") {
      throw new Error(
        `Required Ignition contract name for "${required.futureId}" must not be empty`
      );
    }
    if (futureIds.has(required.futureId)) {
      throw new Error(
        `Duplicate required Ignition future ID "${required.futureId}"`
      );
    }
    futureIds.add(required.futureId);
  }
}

function assertComplete(
  deploymentId: string,
  deploymentStatus: StatusResult
): void {
  const incomplete: string[] = [];
  if (deploymentStatus.started.length > 0) {
    incomplete.push(`started=[${deploymentStatus.started.join(", ")}]`);
  }
  if (deploymentStatus.held.length > 0) {
    incomplete.push(
      `held=[${deploymentStatus.held
        .map(({ futureId }) => futureId)
        .join(", ")}]`
    );
  }
  if (deploymentStatus.timedOut.length > 0) {
    incomplete.push(
      `timedOut=[${deploymentStatus.timedOut
        .map(({ futureId }) => futureId)
        .join(", ")}]`
    );
  }
  if (deploymentStatus.failed.length > 0) {
    incomplete.push(
      `failed=[${deploymentStatus.failed
        .map(({ futureId }) => futureId)
        .join(", ")}]`
    );
  }
  if (incomplete.length > 0) {
    throw new Error(
      `Ignition deployment "${deploymentId}" is not complete: ${incomplete.join(
        "; "
      )}`
    );
  }
}

function normalizeContracts(
  deploymentId: string,
  deploymentStatus: StatusResult
): Readonly<Record<string, IgnitionContractState>> {
  const contracts: Record<string, IgnitionContractState> = {};
  for (const [futureId, contract] of Object.entries(
    deploymentStatus.contracts
  )) {
    if (contract.id !== futureId) {
      throw new Error(
        `Ignition deployment "${deploymentId}" future "${futureId}" has mismatched contract ID "${contract.id}"`
      );
    }
    if (!isAddress(contract.address)) {
      throw new Error(
        `Ignition deployment "${deploymentId}" future "${futureId}" has invalid contract address "${contract.address}"`
      );
    }
    contracts[futureId] = {
      futureId,
      contractName: contract.contractName,
      address: getAddress(contract.address),
      sourceName: contract.sourceName,
    };
  }
  return contracts;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
