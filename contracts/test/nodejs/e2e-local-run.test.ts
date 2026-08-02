import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { Address } from "viem";
import type { ApplySummary } from "../../scripts/command-harness.js";
import type {
  LocalChainDeployment,
  LocalE2EDeployment,
} from "../../scripts/e2e-local-artifacts.js";
import {
  LOCAL_E2E_RUN_NETWORKS,
  resolveLocalE2ERunInput,
  runLocalE2E,
  validateLocalE2ERunDeployment,
} from "../../scripts/e2e-local-run.js";

const deployer = address("11");

test("local E2E run input has a default readiness endpoint", () => {
  assert.deepEqual(resolveLocalE2ERunInput({ tmpDir: "tmp/e2e" }), {
    tmpDir: "tmp/e2e",
    workerReadyUrl: "http://127.0.0.1:19090/readyz",
  });
  assert.deepEqual(
    resolveLocalE2ERunInput({
      tmpDir: "tmp/custom",
      workerReadyUrl: "http://worker:9090/readyz",
    }),
    {
      tmpDir: "tmp/custom",
      workerReadyUrl: "http://worker:9090/readyz",
    }
  );
});

test("local E2E run input rejects invalid paths and readiness URLs", () => {
  assert.throws(
    () => resolveLocalE2ERunInput({ tmpDir: " " }),
    /tmpDir must not be empty/
  );
  assert.throws(
    () =>
      resolveLocalE2ERunInput({
        tmpDir: "tmp/e2e",
        workerReadyUrl: "file:///tmp/readyz",
      }),
    /must use http or https/
  );
});

test("local E2E run deployment is pinned to the two named Hardhat networks", () => {
  const deployment = localDeployment();
  assert.equal(validateLocalE2ERunDeployment(deployment), deployment);
  assert.deepEqual(LOCAL_E2E_RUN_NETWORKS, {
    chainA: "local-anvil-a",
    chainB: "local-anvil-b",
  });

  assert.throws(
    () =>
      validateLocalE2ERunDeployment({
        ...deployment,
        chains: {
          ...deployment.chains,
          b: { ...deployment.chains.b, chainId: 31339 },
        },
      }),
    /local-anvil-b with chain id 31338/
  );
});

test("apply false returns a plan without readiness polling or network connections", async () => {
  const tmpDir = await mkdtemp(path.join(os.tmpdir(), "oml-e2e-run-"));
  try {
    await writeFile(
      path.join(tmpDir, "deployments.json"),
      `${JSON.stringify(localDeployment())}\n`
    );
    let summary: ApplySummary | undefined;
    const result = await runLocalE2E(resolveLocalE2ERunInput({ tmpDir }), {
      hre: {} as HardhatRuntimeEnvironment,
      gate: {
        async authorize(value) {
          summary = value;
          return false;
        },
      },
      fetch: async () => {
        throw new Error("readiness must not be polled for apply false");
      },
    });

    assert.deepEqual(result, {
      applied: false,
      directions: [
        "local-anvil-a->local-anvil-b",
        "local-anvil-b->local-anvil-a",
      ],
    });
    assert.deepEqual(
      summary?.targets.map(({ network, chainId }) => ({ network, chainId })),
      [
        { network: "local-anvil-a", chainId: 31337 },
        { network: "local-anvil-b", chainId: 31338 },
      ]
    );
  } finally {
    await rm(tmpDir, { recursive: true, force: true });
  }
});

test("local E2E runner no longer constructs raw Viem clients or private-key accounts", async () => {
  const source = await readFile("contracts/scripts/e2e-local-run.ts", "utf8");
  assert.doesNotMatch(
    source,
    /\b(?:createPublicClient|createWalletClient|defineChain|privateKeyToAccount)\b/
  );
  assert.match(source, /network: LOCAL_E2E_RUN_NETWORKS\.chainA/);
  assert.match(source, /network: LOCAL_E2E_RUN_NETWORKS\.chainB/);
  assert.match(source, /clients\.provider\.request/);
});

function localDeployment(): LocalE2EDeployment {
  return {
    generatedAt: "2026-07-21T00:00:00.000Z",
    deployer,
    worker: address("22"),
    signers: {
      kms: {
        keyId: "kms-key",
        region: "us-east-1",
        address: address("33"),
        hostEndpoint: "http://127.0.0.1:4566",
        containerEndpoint: "http://localstack:4566",
      },
      keystore: { address: address("22") },
    },
    parameters: {
      confirmations: "1",
      maxMessageSize: 10_000,
      minLzReceiveGas: "100000",
      lzReceiveGas: "250000",
      maxLzReceiveGas: "1000000",
    },
    chains: {
      a: localChain("a", "local-anvil-a", 90101, 31337, 0),
      b: localChain("b", "local-anvil-b", 90102, 31338, 10),
    },
  };
}

function localChain(
  key: "a" | "b",
  name: string,
  eid: number,
  chainId: number,
  offset: number
): LocalChainDeployment {
  return {
    key,
    name,
    eid,
    chainId,
    hostRpcUrl: `http://127.0.0.1:${18545 + offset}`,
    containerRpcUrl: "http://anvil:8545",
    endpoint: address(`${30 + offset}`),
    sendUln: address(`${31 + offset}`),
    receiveUln: address(`${32 + offset}`),
    oft: address(`${33 + offset}`),
    priceFeed: address(`${34 + offset}`),
    openExecutor: address(`${35 + offset}`),
    primaryOpenDVN: address(`${36 + offset}`),
    secondaryOpenDVN: address(`${37 + offset}`),
    executorSigner: address(`${38 + offset}`),
    dvnSigner: address(`${39 + offset}`),
  };
}

function address(byte: string): Address {
  return `0x${byte.padStart(2, "0").repeat(20)}` as Address;
}
