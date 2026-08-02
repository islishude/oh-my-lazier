import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import {
  LOCAL_E2E_IGNITION_DEPLOYMENT_IDS,
  assertLocalE2EIgnitionDirectory,
  buildLocalE2EChainParameters,
  buildLocalE2EPathwayParameters,
  resolveLocalE2EDeployInput,
  writeLocalE2EIgnitionParameters,
} from "../../scripts/e2e-local-deploy.js";
import {
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  NIL_DVN_COUNT,
  decodeExecutorConfig,
  decodeUlnConfig,
} from "../../scripts/lz-config.js";
import type { LocalChainDeployment } from "../../scripts/e2e-local-artifacts.js";

const deployer = "0x1111111111111111111111111111111111111111";
const worker = "0x2222222222222222222222222222222222222222";

test("local E2E Ignition deployment IDs are stable and direction-specific", () => {
  assert.deepEqual(LOCAL_E2E_IGNITION_DEPLOYMENT_IDS, {
    chainA: "local-e2e-chain-a",
    chainB: "local-e2e-chain-b",
    pathwayAToB: "local-e2e-pathway-a-to-b",
    pathwayBToA: "local-e2e-pathway-b-to-a",
  });
});

test("local E2E requires isolated Ignition state below its temporary directory", () => {
  assert.doesNotThrow(() =>
    assertLocalE2EIgnitionDirectory("tmp/e2e/ignition", "tmp/e2e")
  );
  assert.throws(
    () => assertLocalE2EIgnitionDirectory("ignition", "tmp/e2e"),
    /local E2E requires Hardhat Ignition path/
  );
});

test("resolveLocalE2EDeployInput keeps infrastructure secrets outside business input", () => {
  const directory = mkdtempSync(path.join(tmpdir(), "oml-e2e-deploy-"));
  try {
    writeFileSync(
      path.join(directory, "worker-keystore.json"),
      JSON.stringify({ address: worker.slice(2) })
    );
    const resolved = resolveLocalE2EDeployInput(
      { tmpDir: directory },
      {
        E2E_CHAIN_A_HOST_RPC_URL: "http://127.0.0.1:28545",
        E2E_CHAIN_B_HOST_RPC_URL: "http://127.0.0.1:28546",
        E2E_HOST_DATABASE_URL: "postgres://host/worker",
        E2E_KMS_REGION: "test-1",
      }
    );

    assert.equal(resolved.tmpDir, directory);
    assert.equal(resolved.chains[0].hostRpcUrl, "http://127.0.0.1:28545");
    assert.equal(resolved.chains[1].hostRpcUrl, "http://127.0.0.1:28546");
    assert.equal(resolved.databaseURLs.host, "postgres://host/worker");
    assert.equal(resolved.kms.region, "test-1");
    assert.equal(resolved.workerAddress, worker);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test("buildLocalE2EChainParameters preserves token and submitter topology", () => {
  const rendered = buildLocalE2EChainParameters(
    chainSpec("a", 90101, 31337),
    deployer,
    worker,
    123n
  ).LocalE2EChain;

  assert.equal(rendered.tokenName, "Local OFT A");
  assert.equal(rendered.tokenSymbol, "LOFTA");
  assert.equal(rendered.initialSupply, 123n);
  assert.deepEqual(rendered.priceFeedSubmitters, [deployer, worker]);
});

test("local E2E writes absolute Ignition parameter files with bigint values", async () => {
  const directory = mkdtempSync(path.join(tmpdir(), "oml-e2e-parameters-"));
  try {
    const parametersPath = await writeLocalE2EIgnitionParameters(
      directory,
      LOCAL_E2E_IGNITION_DEPLOYMENT_IDS.chainA,
      buildLocalE2EChainParameters(
        chainSpec("a", 90101, 31337),
        deployer,
        worker,
        123n
      )
    );

    assert.equal(
      parametersPath,
      path.resolve(
        directory,
        "ignition-parameters",
        "local-e2e-chain-a.json"
      )
    );
    const parameters = JSON.parse(
      readFileSync(parametersPath, "utf8")
    ) as {
      LocalE2EChain: { initialSupply: string };
    };
    assert.equal(parameters.LocalE2EChain.initialSupply, "123n");
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test("buildLocalE2EPathwayParameters encodes two required DVNs and one enforced option", () => {
  const source = chainDeployment("a", 90101, 31337);
  const destination = chainDeployment("b", 90102, 31338);
  const params = buildLocalE2EPathwayParameters(
    source,
    destination,
    1_700_000_000n,
    deployer
  ).LocalE2EPathway;

  assert.equal(params.remoteEid, destination.eid);
  assert.equal(
    params.remotePeer,
    "0x0000000000000000000000006666666666666666666666666666666666666661"
  );
  assert.equal(params.defaultUlnConfig.requiredDVNCount, 2);
  assert.equal(params.defaultUlnConfig.optionalDVNCount, 0);
  assert.deepEqual(params.defaultUlnConfig.requiredDVNs, [
    source.primaryOpenDVN,
    source.secondaryOpenDVN,
  ]);
  assert.deepEqual(
    params.sendConfig.map((entry) => entry.configType),
    [CONFIG_TYPE_EXECUTOR, CONFIG_TYPE_ULN]
  );
  assert.deepEqual(decodeExecutorConfig(params.sendConfig[0].config), {
    maxMessageSize: 10_000,
    executor: source.openExecutor,
  });
  const customUlnConfig = decodeUlnConfig(params.sendConfig[1].config);
  assert.equal(customUlnConfig.optionalDVNCount, NIL_DVN_COUNT);
  assert.equal(customUlnConfig.requiredDVNCount, 2);
  assert.deepEqual(params.receiveConfig, [params.sendConfig[1]]);
  assert.deepEqual(params.enforcedOptions, [
    {
      eid: destination.eid,
      msgType: 1,
      options: "0x0003010011010000000000000000000000000003d090",
    },
  ]);
  assert.equal(params.primaryDVNVerifier, source.dvnSigner);
  assert.equal(params.secondaryDVNVerifier, deployer);
  assert.equal(params.priceSnapshot.updatedAt, 1_700_000_000n);
});

test("local E2E sends rely on the single enforced lzReceive option", () => {
  const runSource = readFileSync("contracts/scripts/e2e-local-run.ts", "utf8");
  assert.doesNotMatch(runSource, /\blzReceiveGas\s*:/);
});

function chainSpec(key: "a" | "b", eid: number, chainId: number) {
  return {
    key,
    name: `local-anvil-${key}`,
    eid,
    chainId,
    hostRpcUrl: `http://127.0.0.1:${key === "a" ? 18545 : 18546}`,
    containerRpcUrl: `http://anvil-${key}:8545`,
  } as const;
}

function chainDeployment(
  key: "a" | "b",
  eid: number,
  chainId: number
): LocalChainDeployment {
  const offset = key === "a" ? 0 : 1;
  return {
    ...chainSpec(key, eid, chainId),
    endpoint: `0x333333333333333333333333333333333333333${offset}`,
    sendUln: `0x444444444444444444444444444444444444444${offset}`,
    receiveUln: `0x555555555555555555555555555555555555555${offset}`,
    oft: `0x666666666666666666666666666666666666666${offset}`,
    priceFeed: `0x777777777777777777777777777777777777777${offset}`,
    openExecutor: `0x888888888888888888888888888888888888888${offset}`,
    primaryOpenDVN: `0x999999999999999999999999999999999999999${offset}`,
    secondaryOpenDVN: `0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa${offset}`,
    executorSigner: key === "a" ? deployer : worker,
    dvnSigner: key === "a" ? deployer : worker,
  };
}
