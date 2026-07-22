import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import { runIgnitionModuleCommand } from "../../scripts/commands/ignition-command.js";

const signer = "0x1111111111111111111111111111111111111111";
const deployed = "0x2222222222222222222222222222222222222222";

test("Ignition command verifies only after deployment connection closes", async () => {
  const directory = mkdtempSync(join(tmpdir(), "oml-ignition-command-"));
  const parameters = join(directory, "ignition-parameters.json");
  const runFile = join(directory, "run.json");
  writeFileSync(parameters, JSON.stringify({ ExampleModule: {} }));
  writeFileSync(
    runFile,
    JSON.stringify({
      input: {
        parameters,
        deploymentId: "sepolia-example",
        verify: true,
        expectedSigner: signer,
      },
      apply: true,
      confirmation: "approved",
    })
  );

  const events: string[] = [];
  const output: string[] = [];
  const diagnostics: string[] = [];
  const deployCalls: Array<{
    module: unknown;
    options: Record<string, unknown>;
  }> = [];
  const hre = fakeHre(events, deployCalls);
  const module = {} as never;
  const previousParams = process.env.OML_SCRIPT_PARAMS;
  const originalLog = console.log;
  const originalStderrWrite = process.stderr.write;
  process.env.OML_SCRIPT_PARAMS = runFile;
  console.log = (message?: unknown) => output.push(String(message));
  process.stderr.write = ((chunk: string | Uint8Array) => {
    diagnostics.push(String(chunk));
    return true;
  }) as typeof process.stderr.write;
  try {
    await runIgnitionModuleCommand({
      hre,
      command: "deploy:example",
      module,
      supportsSourceVerification: true,
      verifyDeploymentSources: async (request) => {
        events.push("verify");
        assert.equal(events.at(-2), "close");
        assert.equal(request.hre, hre);
        assert.equal(request.buildProfile, "production");
        assert.deepEqual(request.targets, [
          { network: "sepolia", deploymentId: "sepolia-example" },
        ]);
      },
    });
  } finally {
    console.log = originalLog;
    process.stderr.write = originalStderrWrite;
    if (previousParams === undefined) {
      delete process.env.OML_SCRIPT_PARAMS;
    } else {
      process.env.OML_SCRIPT_PARAMS = previousParams;
    }
  }

  assert.deepEqual(events, ["build", "connect", "deploy", "close", "verify"]);
  assert.deepEqual(deployCalls, [
    {
      module,
      options: {
        parameters: resolve(parameters),
        deploymentId: "sepolia-example",
        displayUi: true,
      },
    },
  ]);
  assert.match(diagnostics.join(""), /Ignition UI progress/);
  assert.equal(output.length, 1);
  assert.deepEqual(JSON.parse(output[0]) as unknown, {
    applied: true,
    network: "sepolia",
    chainId: 11155111,
    deploymentId: "sepolia-example",
    contracts: { Example: deployed },
    verified: true,
  });
});

test("Ignition configuration commands reject source verification", async () => {
  const directory = mkdtempSync(join(tmpdir(), "oml-ignition-config-command-"));
  const parameters = join(directory, "ignition-parameters.json");
  const runFile = join(directory, "run.json");
  writeFileSync(parameters, JSON.stringify({ ConfigModule: {} }));
  writeFileSync(
    runFile,
    JSON.stringify({
      input: {
        parameters,
        deploymentId: "sepolia-config",
        verify: false,
        expectedSigner: signer,
      },
      apply: false,
    })
  );

  const previousParams = process.env.OML_SCRIPT_PARAMS;
  process.env.OML_SCRIPT_PARAMS = runFile;
  try {
    await assert.rejects(
      runIgnitionModuleCommand({
        hre: {} as HardhatRuntimeEnvironment,
        command: "configure:example",
        module: {} as never,
      }),
      /input contains unknown field: verify/
    );
  } finally {
    if (previousParams === undefined) {
      delete process.env.OML_SCRIPT_PARAMS;
    } else {
      process.env.OML_SCRIPT_PARAMS = previousParams;
    }
  }
});

function fakeHre(
  events: string[],
  deployCalls: Array<{
    module: unknown;
    options: Record<string, unknown>;
  }>
): HardhatRuntimeEnvironment {
  const connection = {
    networkName: "sepolia",
    networkConfig: { chainId: 11155111 },
    viem: {
      async getPublicClient() {
        return {
          chain: { id: 11155111 },
          async getChainId() {
            return 11155111;
          },
        };
      },
      async getWalletClients() {
        return [{ account: { address: signer } }];
      },
    },
    ignition: {
      async deploy(module: unknown, options: Record<string, unknown>) {
        events.push("deploy");
        deployCalls.push({ module, options });
        process.stdout.write("Ignition UI progress\n");
        return { Example: { address: deployed } };
      },
    },
    async close() {
      events.push("close");
    },
  };

  return {
    globalOptions: { network: "sepolia", buildProfile: "production" },
    tasks: {
      getTask() {
        return {
          async run() {
            events.push("build");
          },
        };
      },
    },
    network: {
      async create() {
        events.push("connect");
        return connection;
      },
    },
  } as unknown as HardhatRuntimeEnvironment;
}
