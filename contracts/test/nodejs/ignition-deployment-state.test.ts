import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";
import type { StatusResult } from "@nomicfoundation/ignition-core";
import {
  ignitionDeploymentsDir,
  readIgnitionDeploymentState,
  type IgnitionDeploymentStateIO,
  type IgnitionRuntime,
} from "../../scripts/ignition-deployment-state.js";

const deploymentId = "sepolia-open-workers";
const expectedChainId = 11155111;
const futureId = "OpenWorkers#OpenExecutor";
const contractName = "OpenExecutor";
const address = "0x1111111111111111111111111111111111111111";
const ignitionDir = path.resolve("tmp/test-ignition-state/ignition");
const hre: IgnitionRuntime = {
  config: { paths: { ignition: ignitionDir } },
};

function completeStatus(): StatusResult {
  return {
    started: [],
    successful: [futureId],
    held: [],
    timedOut: [],
    failed: [],
    chainId: expectedChainId,
    contracts: {
      [futureId]: {
        id: futureId,
        contractName,
        address,
        sourceName: "contracts/contracts/workers/OpenExecutor.sol",
        abi: [],
      },
    },
  };
}

function fakeIO(
  deploymentStatus: StatusResult = completeStatus()
): IgnitionDeploymentStateIO {
  return {
    async listDeployments() {
      return [deploymentId];
    },
    async status() {
      return deploymentStatus;
    },
  };
}

function request() {
  return {
    deploymentId,
    expectedChainId,
    requiredContracts: [{ futureId, contractName }],
  } as const;
}

test("reads deployment state from the configured Ignition directory", async () => {
  const calls: string[] = [];
  const io: IgnitionDeploymentStateIO = {
    async listDeployments(deploymentsDir) {
      calls.push(`list:${deploymentsDir}`);
      return [deploymentId];
    },
    async status(deploymentDir) {
      calls.push(`status:${deploymentDir}`);
      return completeStatus();
    },
  };

  const result = await readIgnitionDeploymentState(hre, request(), io);

  const deploymentsDir = path.join(ignitionDir, "deployments");
  assert.deepEqual(calls, [
    `list:${deploymentsDir}`,
    `status:${path.join(deploymentsDir, deploymentId)}`,
  ]);
  assert.equal(result.kind, "ready");
  if (result.kind === "ready") {
    assert.equal(result.chainId, expectedChainId);
    assert.equal(result.contracts[futureId]?.address, address);
    assert.equal(result.contracts[futureId]?.contractName, contractName);
  }
});

test("returns missing only when the deployment ID is not listed", async () => {
  let statusCalled = false;
  const result = await readIgnitionDeploymentState(hre, request(), {
    async listDeployments() {
      return ["some-other-deployment"];
    },
    async status() {
      statusCalled = true;
      return completeStatus();
    },
  });

  assert.deepEqual(result, {
    kind: "missing",
    deploymentId,
    deploymentDir: path.join(ignitionDeploymentsDir(hre), deploymentId),
  });
  assert.equal(statusCalled, false);
});

test("fails when listed deployment state is uninitialized or corrupt", async () => {
  const cause = new Error("deployment is uninitialized");
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), {
      async listDeployments() {
        return [deploymentId];
      },
      async status() {
        throw cause;
      },
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.match(
        error.message,
        /cannot be read: deployment is uninitialized/
      );
      assert.equal(error.cause, cause);
      return true;
    }
  );
});

test("rejects incomplete deployment status", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.started = [futureId];
  deploymentStatus.held = [
    { futureId: "Config#Held", heldId: 1, reason: "waiting" },
  ];
  deploymentStatus.timedOut = [
    { futureId: "Config#TimedOut", networkInteractionId: 2 },
  ];
  deploymentStatus.failed = [
    {
      futureId: "Config#Failed",
      networkInteractionId: 3,
      error: "execution reverted",
    },
  ];

  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /is not complete: started=\[OpenWorkers#OpenExecutor\]; held=\[Config#Held\]; timedOut=\[Config#TimedOut\]; failed=\[Config#Failed\]/
  );
});

test("rejects a deployment for the wrong chain", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.chainId = 560048;
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /has chain ID 560048, expected 11155111/
  );
});

test("rejects a missing required Future ID", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.contracts = {};
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /is missing required future "OpenWorkers#OpenExecutor"/
  );
});

test("rejects a required Future that is not successful", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.successful = [];
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /required future "OpenWorkers#OpenExecutor" is not successful/
  );
});

test("rejects a mismatched Future contract ID", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.contracts[futureId].id = "OpenWorkers#Wrong";
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /has mismatched contract ID "OpenWorkers#Wrong"/
  );
});

test("rejects an invalid deployed contract address", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.contracts[futureId].address = "not-an-address";
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /has invalid contract address "not-an-address"/
  );
});

test("rejects an unexpected deployed contract name", async () => {
  const deploymentStatus = completeStatus();
  deploymentStatus.contracts[futureId].contractName = "WrongExecutor";
  await assert.rejects(
    readIgnitionDeploymentState(hre, request(), fakeIO(deploymentStatus)),
    /has contract name "WrongExecutor", expected "OpenExecutor"/
  );
});
