import assert from "node:assert/strict";
import { join, resolve } from "node:path";
import test from "node:test";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import {
  verifyIgnitionDeploymentSources,
  type VerificationProcessRequest,
} from "../../scripts/ignition-source-verification.js";
import { readOnlyVerificationConfig } from "../../../hardhat.verify.config.js";

const projectRoot = resolve(".");

test("verification config never resolves local accounts for HTTP networks", () => {
  const networks = Object.entries(readOnlyVerificationConfig.networks ?? {});
  assert(networks.some(([, network]) => network.type === "http"));
  for (const [name, network] of networks) {
    if (network.type === "http") {
      assert.equal(network.accounts, "remote", name);
    }
  }
});

test("verification subprocesses are serial and use explicit immutable CLI inputs", async () => {
  const calls: VerificationProcessRequest[] = [];
  const events: string[] = [];
  let active = 0;
  let maximumActive = 0;
  const environment = { ETHERSCAN_API_KEY: "must-not-appear-in-args" };

  await verifyIgnitionDeploymentSources(
    {
      hre: fakeHre(),
      buildProfile: "production",
      targets: [
        { network: "sepolia", deploymentId: "sepolia-open-workers" },
        { network: "hoodi", deploymentId: "hoodi-open-workers" },
      ],
    },
    {
      environment,
      hardhatCliPath: process.execPath,
      inspectDeployment: async () => 1,
      runProcess: async (request) => {
        calls.push(request);
        active += 1;
        maximumActive = Math.max(maximumActive, active);
        events.push(`start:${request.args.at(-1)}`);
        await Promise.resolve();
        events.push(`end:${request.args.at(-1)}`);
        active -= 1;
        return { exitCode: 0, signal: null };
      },
    }
  );

  assert.equal(maximumActive, 1);
  assert.deepEqual(events, [
    "start:sepolia-open-workers",
    "end:sepolia-open-workers",
    "start:hoodi-open-workers",
    "end:hoodi-open-workers",
  ]);
  assert.equal(calls.length, 2);
  assert.deepEqual(calls[0], {
    executable: process.execPath,
    args: [
      process.execPath,
      "--config",
      join(projectRoot, "hardhat.verify.config.ts"),
      "--network",
      "sepolia",
      "--build-profile",
      "production",
      "ignition",
      "verify",
      "sepolia-open-workers",
    ],
    cwd: projectRoot,
    environment,
  });
  assert(
    !calls.flatMap((call) => call.args).includes(environment.ETHERSCAN_API_KEY)
  );
});

test("verification preflights every deployment before starting subprocesses", async () => {
  const inspected: string[] = [];
  let processCalls = 0;

  await assert.rejects(
    verifyIgnitionDeploymentSources(
      {
        hre: fakeHre(),
        buildProfile: "production",
        targets: [
          { network: "sepolia", deploymentId: "sepolia-open-workers" },
          { network: "hoodi", deploymentId: "hoodi-open-workers" },
        ],
      },
      {
        hardhatCliPath: process.execPath,
        inspectDeployment: async (deploymentDir) => {
          inspected.push(deploymentDir);
          return inspected.length === 1 ? 1 : 0;
        },
        runProcess: async () => {
          processCalls += 1;
          return { exitCode: 0, signal: null };
        },
      }
    ),
    /hoodi-open-workers has no resolvable contracts/
  );

  assert.equal(inspected.length, 2);
  assert.equal(processCalls, 0);
});

test("verification rejects nonzero exits without echoing subprocess output or secrets", async () => {
  const secret = "never-echo-this-api-key";
  let calls = 0;
  await assert.rejects(
    verifyIgnitionDeploymentSources(
      {
        hre: fakeHre(),
        buildProfile: "production",
        targets: [
          { network: "sepolia", deploymentId: "sepolia-open-workers" },
          { network: "hoodi", deploymentId: "hoodi-open-workers" },
        ],
      },
      {
        environment: { ETHERSCAN_API_KEY: secret },
        hardhatCliPath: process.execPath,
        inspectDeployment: async () => 1,
        runProcess: async () => {
          calls += 1;
          return { exitCode: 7, signal: null };
        },
      }
    ),
    (error: unknown) => {
      assert(error instanceof Error);
      assert.match(error.message, /failed with exit code 7/);
      assert.doesNotMatch(error.message, new RegExp(secret));
      return true;
    }
  );
  assert.equal(calls, 1);
});

test("verification validates providers, profile, HTTP network, and deployment id", async () => {
  const baseRequest = {
    hre: fakeHre(),
    buildProfile: "production",
    targets: [{ network: "sepolia", deploymentId: "sepolia-open-workers" }],
  } as const;
  const dependencies = {
    hardhatCliPath: process.execPath,
    inspectDeployment: async () => 1,
    runProcess: async () => ({ exitCode: 0, signal: null }),
  };

  await assert.rejects(
    verifyIgnitionDeploymentSources(
      {
        ...baseRequest,
        hre: fakeHre({ providersEnabled: false }),
      },
      dependencies
    ),
    /requires an enabled provider/
  );
  await assert.rejects(
    verifyIgnitionDeploymentSources(
      { ...baseRequest, buildProfile: "prod profile" },
      dependencies
    ),
    /requires a valid build profile/
  );
  await assert.rejects(
    verifyIgnitionDeploymentSources(
      { ...baseRequest, buildProfile: "missing" },
      dependencies
    ),
    /build profile missing is not configured/
  );
  await assert.rejects(
    verifyIgnitionDeploymentSources(
      {
        ...baseRequest,
        targets: [{ network: "hardhat", deploymentId: "local-deployment" }],
      },
      dependencies
    ),
    /must be an HTTP network/
  );
  await assert.rejects(
    verifyIgnitionDeploymentSources(
      {
        ...baseRequest,
        targets: [{ network: "sepolia", deploymentId: "--unsafe" }],
      },
      dependencies
    ),
    /invalid Ignition deployment id/
  );
});

function fakeHre(
  options: { providersEnabled?: boolean } = {}
): HardhatRuntimeEnvironment {
  const providersEnabled = options.providersEnabled ?? true;
  return {
    config: {
      paths: {
        root: projectRoot,
        ignition: join(projectRoot, "contracts/ignition"),
      },
      networks: {
        hardhat: { type: "edr-simulated", chainId: 31337 },
        sepolia: { type: "http", chainId: 11155111 },
        hoodi: { type: "http", chainId: 560048 },
      },
      solidity: {
        profiles: { default: {}, production: {} },
      },
      verify: {
        etherscan: { enabled: providersEnabled },
        blockscout: { enabled: false },
        sourcify: { enabled: false },
      },
    },
  } as unknown as HardhatRuntimeEnvironment;
}
