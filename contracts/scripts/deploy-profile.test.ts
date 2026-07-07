import assert from "node:assert/strict";
import test from "node:test";
import type { PublicClient } from "viem";
import {
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  NIL_DVN_COUNT,
  decodeExecutorConfig,
  decodeUlnConfig,
} from "./lz-config.js";
import {
  buildCommandPlan,
  buildDeploymentState,
  deploymentPreflightArgs,
  extractOpenWorkerContracts,
  isBootstrapStateUnavailable,
  normalizeProfile,
  oappEndpointParameterFile,
  openWorkersParameterFile,
  openWorkersPathwayParameterFile,
  readPriceFeedFromWorkers,
  renderWorkerConfig,
  resolveWorkerStartBlocks,
  shouldRunConfigureOApp,
  shouldRunWorkerOnlyVerify,
  testOFTParameterFile,
  testOFTDeploymentAddressPath,
  type DeploymentProfile,
  workerDeploymentAddressPath,
} from "./deploy-profile.js";

test("normalizeProfile validates rehearsal mode and LayerZero metadata", () => {
  const profile = normalizeProfile(baseProfile());

  assert.equal(profile.mode, "test-oft-rehearsal");
  assert.equal(profile.dvnMode, "active");
  assert.equal(profile.chains[0].eid, 40161);
  assert.equal(profile.chains[0].nativeAssetId, "eth");
  assert.equal(profile.chains[0].startBlockNumber, undefined);
  assert.equal(
    profile.chains[0].layerZero.endpointV2,
    "0x6EDCE65403992e310A62460808c4b910D972f10f",
  );
  assert.equal(profile.chains[1].eid, 40449);
  assert.equal(profile.chains[1].nativeAssetId, "eth");
});

test("normalizeProfile requires external OApp addresses in external mode", () => {
  const input = externalProfile();
  delete (input.chains[0] as Record<string, unknown>).oapp;

  assert.throws(
    () => normalizeProfile(input),
    /profile\.chains\[0\]\.oapp is required for external-oapp mode/,
  );
});

test("normalizeProfile requires long-term price feed submitters", () => {
  const input = baseProfile();
  delete (input as Record<string, unknown>).priceFeedSubmitters;

  assert.throws(
    () => normalizeProfile(input),
    /profile\.priceFeedSubmitters is required/,
  );
});

test("normalizeProfile rejects owner as a long-term price feed submitter", () => {
  const input = baseProfile();
  input.priceFeedSubmitters = ["0x1111111111111111111111111111111111111111"];

  assert.throws(
    () => normalizeProfile(input),
    /profile\.priceFeedSubmitters must not include profile\.owner/,
  );
});

test("normalizeProfile rejects uppercase native asset ids", () => {
  const input = baseProfile();
  (input.chains[0] as Record<string, unknown>).nativeAssetId = "ETH";

  assert.throws(
    () => normalizeProfile(input),
    /profile\.chains\[0\]\.nativeAssetId must be lowercase/,
  );
});

test("normalizeProfile rejects legacy top-level canary token balance", () => {
  const input = {
    ...baseProfile(),
    minCanaryTokenBalance: "1000000000000000",
  };

  assert.throws(
    () => normalizeProfile(input),
    /profile\.minCanaryTokenBalance is not supported/,
  );
});

test("normalizeProfile requires chain canary token balance in rehearsal mode", () => {
  const input = baseProfile();
  delete (input.chains[1] as Record<string, unknown>).minCanaryTokenBalance;

  assert.throws(
    () => normalizeProfile(input),
    /profile\.chains\[1\]\.minCanaryTokenBalance must be a non-empty string/,
  );
});

test("normalizeProfile rejects tx roles that do not reference a configured signer", () => {
  const input = baseProfile();
  input.chains[0].txRoles.executor.signer =
    "0x9999999999999999999999999999999999999999";

  assert.throws(
    () => normalizeProfile(input),
    /txRoles\.executor\.signer must reference a configured signer/,
  );
});

test("normalizeProfile rejects zero tx role minimum native balance", () => {
  const input = baseProfile();
  input.chains[0].txRoles.executor.minNativeBalanceWei = "0";

  assert.throws(
    () => normalizeProfile(input),
    /txRoles\.executor\.minNativeBalanceWei must be positive/,
  );
});

test("normalizeProfile rejects Hardhat private key env injection", () => {
  const input = baseProfile();
  (input.chains[0] as Record<string, unknown>).privateKeyEnv =
    "SEPOLIA_PRIVATE_KEY";

  assert.throws(
    () => normalizeProfile(input),
    /profile\.chains\[0\]\.privateKeyEnv is not supported/,
  );
});

test("extractOpenWorkerContracts and buildDeploymentState require OpenWorkers price feed", () => {
  const profile = normalizeProfile(baseProfile());
  const workerDeployedAddresses = {
    sepolia: deployedWorkers(
      "0x1111111111111111111111111111111111111111",
      false,
    ),
    hoodi: deployedWorkers("0x2222222222222222222222222222222222222222", false),
  };
  const testOFTDeployedAddresses = {
    sepolia: deployedTestOFT("0x1111111111111111111111111111111111111111"),
    hoodi: deployedTestOFT("0x2222222222222222222222222222222222222222"),
  };

  assert.equal(
    extractOpenWorkerContracts(workerDeployedAddresses.sepolia, "sepolia")
      .openExecutor,
    "0x1111111111111111111111111111111111111112",
  );
  assert.throws(
    () =>
      buildDeploymentState({
        profile,
        workerDeployedAddresses,
        testOFTDeployedAddresses,
      }),
    /missing OpenWorkers#OpenPriceFeed/,
  );

  const state = buildDeploymentState({
    profile,
    workerDeployedAddresses,
    testOFTDeployedAddresses,
    priceFeedOverrides: {
      sepolia: "0x1111111111111111111111111111111111111114",
      hoodi: "0x2222222222222222222222222222222222222225",
    },
    generatedAt: "2026-07-05T00:00:00.000Z",
  });

  assert.equal(
    state.chains[0].workers.priceFeed,
    "0x1111111111111111111111111111111111111114",
  );
  assert.equal(
    state.chains[0].oapp,
    "0x1111111111111111111111111111111111111111",
  );
  assert.equal(
    state.directions[0].receiveLib,
    profile.chains[1].layerZero.receiveUln302,
  );
  assert.equal(
    state.directions[1].sourceWorkers.openDVN,
    state.chains[1].workers.openDVN,
  );
});

test("buildDeploymentState uses profile OApps in external mode without TestOFT state", () => {
  const profile = normalizeProfile(externalProfile());
  const state = buildDeploymentState({
    profile,
    workerDeployedAddresses: {
      sepolia: deployedWorkers("0x1111111111111111111111111111111111111111"),
      hoodi: deployedWorkers("0x2222222222222222222222222222222222222222"),
    },
    generatedAt: "2026-07-05T00:00:00.000Z",
  });

  assert.equal(state.mode, "external-oapp");
  assert.equal(
    state.chains[0].oapp,
    "0xaAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa",
  );
  assert.equal(
    state.chains[1].oapp,
    "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
  );
});

test("readPriceFeedFromWorkers hydrates only matching worker price feeds", async () => {
  const priceFeed = "0x3333333333333333333333333333333333333333";
  const publicClient = {
    readContract: async () => priceFeed,
  } as unknown as PublicClient;

  assert.equal(
    await readPriceFeedFromWorkers({
      publicClient,
      chainKey: "sepolia",
      openExecutor: "0x1111111111111111111111111111111111111112",
      openDVN: "0x1111111111111111111111111111111111111113",
      openExecutorAbi: [],
      openDVNAbi: [],
    }),
    priceFeed,
  );

  let count = 0;
  const mismatchedClient = {
    readContract: async () =>
      count++ === 0
        ? "0x3333333333333333333333333333333333333333"
        : "0x4444444444444444444444444444444444444444",
  } as unknown as PublicClient;

  await assert.rejects(
    readPriceFeedFromWorkers({
      publicClient: mismatchedClient,
      chainKey: "hoodi",
      openExecutor: "0x2222222222222222222222222222222222222222",
      openDVN: "0x2222222222222222222222222222222222222223",
      openExecutorAbi: [],
      openDVNAbi: [],
    }),
    /does not match/,
  );
});

test("oappEndpointParameterFile and openWorkersPathwayParameterFile split config surfaces", () => {
  const profile = normalizeProfile(baseProfile());
  const state = stateWithPriceFeeds(profile);
  const source = profile.chains[0];
  const destination = profile.chains[1];
  const oapp = oappEndpointParameterFile({
    profile,
    state,
    source,
    destination,
    priceSnapshotUpdatedAt: 1_800_000_000n,
  }).OAppEndpointConfig;
  const workers = openWorkersPathwayParameterFile({
    profile,
    state,
    source,
    destination,
    priceSnapshotUpdatedAt: 1_800_000_000n,
  }).OpenWorkersPathwayConfig;

  assert.equal(oapp.oapp, state.chains[0].oapp);
  assert.equal(oapp.endpoint, profile.chains[0].layerZero.endpointV2);
  assert.equal(oapp.remoteEid, 40449);
  assert.equal(oapp.sendConfig[0].configType, CONFIG_TYPE_EXECUTOR);
  assert.deepEqual(decodeExecutorConfig(oapp.sendConfig[0].config), {
    maxMessageSize: 10000,
    executor: state.chains[0].workers.openExecutor,
  });
  assert.equal(oapp.sendConfig[1].configType, CONFIG_TYPE_ULN);
  const uln = decodeUlnConfig(oapp.sendConfig[1].config);
  assert.equal(uln.confirmations, 12n);
  assert.equal(uln.requiredDVNCount, 2);
  assert.equal(uln.optionalDVNCount, NIL_DVN_COUNT);
  assert.deepEqual(
    uln.requiredDVNs.map((address) => address.toLowerCase()).sort(),
    [
      profile.chains[0].layerZero.layerZeroLabsDVN.toLowerCase(),
      state.chains[0].workers.openDVN.toLowerCase(),
    ].sort(),
  );
  assert.deepEqual(oapp.receiveConfig, [
    {
      eid: 40449,
      configType: CONFIG_TYPE_ULN,
      config: oapp.sendConfig[1].config,
    },
  ]);

  assert.equal(workers.oapp, state.chains[0].oapp);
  assert.equal(workers.remoteEid, 40449);
  assert.equal(workers.openExecutor, state.chains[0].workers.openExecutor);
  assert.equal(workers.openDVN, state.chains[0].workers.openDVN);
  assert.equal(workers.priceFeed, state.chains[0].workers.priceFeed);
  assert.equal(workers.bootstrapPriceSubmitter, profile.owner);
  assert.equal(workers.dvnVerifier, profile.chains[0].txRoles.dvn.signer);
  assert.deepEqual(workers.priceSnapshot, {
    dstGasPriceInSrcToken: "1",
    dstDataFeePerByteInSrcToken: "0",
    updatedAt: "1800000000",
    staleAfter: "1800",
  });
  assert.equal(Object.hasOwn(workers, "sendConfig"), false);
  assert.equal(Object.hasOwn(workers, "enforcedOptions"), false);
});

test("renderWorkerConfig emits external OApps, active DVN signer, and worker contracts", () => {
  const profile = normalizeProfile(externalProfile());
  const state = stateWithPriceFeeds(profile);
  const yaml = renderWorkerConfig({
    profile,
    state,
    rpcUrls: {
      sepolia: "https://sepolia.example.invalid/rpc?key=abc",
      hoodi: "https://hoodi.example.invalid/rpc?key=def",
    },
    workerStartBlocks: {
      sepolia: 123456,
      hoodi: 654321,
    },
  });

  assert.match(yaml, /database_url: "postgres:\/\/laz_worker/);
  assert.match(yaml, /src_oapp: "0xaAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa"/);
  assert.match(yaml, /dst_oapp: "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB"/);
  assert.match(yaml, /mode: active/);
  assert.match(
    yaml,
    /source_workers:\n      open_executor: "0x1111111111111111111111111111111111111112"/,
  );
  assert.match(
    yaml,
    /destination_workers:\n      open_dvn: "0x2222222222222222222222222222222222222223"/,
  );
  assert.match(yaml, /signer: "0x2222222222222222222222222222222222222222"/);
  assert.match(yaml, /pricing:\n  enabled: true/);
  assert.match(
    yaml,
    /signer: "0x2222222222222222222222222222222222222222"\n  interval_seconds: 300/,
  );
  assert.match(yaml, /native_asset_id: eth/);
  assert.match(yaml, /min_native_balance_wei: "100000000000000000"/);
  assert.match(yaml, /start_block_number: 123456/);
  assert.match(yaml, /start_block_number: 654321/);
});

test("resolveWorkerStartBlocks queries latest height only for missing profile overrides", async () => {
  const input = baseProfile();
  (input.chains[1] as Record<string, unknown>).startBlockNumber = 0;
  const profile = normalizeProfile(input);
  const queried: string[] = [];

  assert.deepEqual(
    await resolveWorkerStartBlocks({
      profile,
      rpcUrls: {
        sepolia: "https://sepolia.example.invalid/rpc",
        hoodi: "https://hoodi.example.invalid/rpc",
      },
      latestBlockNumber: async (chain, rpcURL) => {
        queried.push(`${chain.key}:${rpcURL}`);
        return 123456n;
      },
    }),
    {
      sepolia: 123456,
      hoodi: 0,
    },
  );
  assert.deepEqual(queried, ["sepolia:https://sepolia.example.invalid/rpc"]);
});

test("resolveWorkerStartBlocks preserves explicit non-zero profile override", async () => {
  const input = baseProfile();
  (input.chains[0] as Record<string, unknown>).startBlockNumber = 111;
  (input.chains[1] as Record<string, unknown>).startBlockNumber = 222;
  const profile = normalizeProfile(input);

  assert.deepEqual(
    await resolveWorkerStartBlocks({
      profile,
      rpcUrls: {
        sepolia: "https://sepolia.example.invalid/rpc",
        hoodi: "https://hoodi.example.invalid/rpc",
      },
      latestBlockNumber: async () => {
        throw new Error("latest block should not be queried");
      },
    }),
    {
      sepolia: 111,
      hoodi: 222,
    },
  );
});

test("parameter files split TestOFT rehearsal from OpenWorkers deployment", () => {
  const input = baseProfile();
  input.priceFeedSubmitters = [
    "0x2222222222222222222222222222222222222222",
    "0x2222222222222222222222222222222222222222",
  ];
  const profile = normalizeProfile(input);

  assert.deepEqual(openWorkersParameterFile(profile, profile.chains[0]), {
    OpenWorkers: {
      owner: "0x1111111111111111111111111111111111111111",
      priceFeedSubmitters: [
        "0x2222222222222222222222222222222222222222",
        "0x1111111111111111111111111111111111111111",
      ],
    },
  });
  assert.deepEqual(testOFTParameterFile(profile, profile.chains[0]), {
    TestOFT: {
      tokenName: "Oh My Lazier Test OFT",
      tokenSymbol: "OMLTOFT",
      endpoint: "0x6EDCE65403992e310A62460808c4b910D972f10f",
      delegate: "0x1111111111111111111111111111111111111111",
      initialRecipient: "0x1111111111111111111111111111111111111111",
      initialSupply: "1000000000000000000000000n",
    },
  });
});

test("deployment state paths use profile deployment ids", () => {
  const profile = normalizeProfile(baseProfile());

  assert.equal(
    workerDeploymentAddressPath(profile.chains[0]),
    "ignition/deployments/sepolia-open-workers/deployed_addresses.json",
  );
  assert.equal(
    testOFTDeploymentAddressPath(profile.chains[0]),
    "ignition/deployments/sepolia-test-oft/deployed_addresses.json",
  );
});

test("deployment preflight args use chain canary token balances", () => {
  const profile = normalizeProfile(baseProfile());
  const state = stateWithPriceFeeds(profile);
  const sepoliaArgs = deploymentPreflightArgs({
    profile,
    chain: profile.chains[0],
    current: state.chains[0],
    rpcURL: "https://sepolia.example.invalid/rpc",
  });
  const hoodiArgs = deploymentPreflightArgs({
    profile,
    chain: profile.chains[1],
    current: state.chains[1],
    rpcURL: "https://hoodi.example.invalid/rpc",
  });

  assert.equal(
    sepoliaArgs[sepoliaArgs.indexOf("--min-canary-token-balance") + 1],
    "1000000000000000",
  );
  assert.equal(
    hoodiArgs[hoodiArgs.indexOf("--min-canary-token-balance") + 1],
    "0",
  );
});

test("command plan and phase gates keep external OApp config explicit", () => {
  const profile = normalizeProfile(externalProfile());
  const plan = buildCommandPlan({ profile, outDir: "tmp/deploy-profile" });
  const commandText = plan.commands
    .map((command) => command.command)
    .join("\n");

  assert.doesNotMatch(commandText, /deploy:test-oft/);
  assert.doesNotMatch(commandText, /PRIVATE_KEY/);
  assert.match(commandText, /npm run deploy:open-workers/);
  assert.match(commandText, /npm run configure:open-workers-pathway/);
  assert.match(commandText, /npm run configure:oapp-endpoint/);
  assert.doesNotMatch(commandText, /--build-profile/);
  assert.doesNotMatch(commandText, /--verify/);
  assert.doesNotMatch(commandText, /HARDHAT_IGNITION_CONFIRM/);
  assert.equal(shouldRunConfigureOApp(profile, "all", true), false);
  assert.equal(shouldRunConfigureOApp(profile, "configure-oapp", true), true);
  assert.equal(shouldRunWorkerOnlyVerify(profile, "all"), true);
  assert.equal(shouldRunWorkerOnlyVerify(profile, "verify"), false);
});

test("command plan forwards Ignition verify, build profile, and auto-confirm flags", () => {
  const profile = normalizeProfile(baseProfile());
  const plan = buildCommandPlan({
    profile,
    outDir: "tmp/deploy-profile",
    ignition: { verify: true, autoConfirm: true, buildProfile: "production" },
  });
  const mutatingCommands = plan.commands
    .filter((command) => command.mutates)
    .map((command) => command.command);
  const readOnlyCommands = plan.commands
    .filter((command) => !command.mutates)
    .map((command) => command.command)
    .join("\n");

  assert.equal(mutatingCommands.length, 8);
  assert.match(
    mutatingCommands[0],
    /^SEPOLIA_RPC_URL=\.\.\. HARDHAT_IGNITION_CONFIRM_DEPLOYMENT=true HARDHAT_IGNITION_CONFIRM_RESET=true npm run deploy:test-oft -- --build-profile production --network sepolia /,
  );
  for (const command of mutatingCommands) {
    assert.match(command, /HARDHAT_IGNITION_CONFIRM_DEPLOYMENT=true/);
    assert.match(command, /HARDHAT_IGNITION_CONFIRM_RESET=true/);
    assert.match(command, /--build-profile production/);
    assert.match(command, /--verify(?:\s|$)/);
  }
  assert.doesNotMatch(readOnlyCommands, /--build-profile/);
  assert.doesNotMatch(readOnlyCommands, /--verify/);
  assert.doesNotMatch(readOnlyCommands, /HARDHAT_IGNITION_CONFIRM/);
});

test("command plan rejects invalid Ignition build profile values", () => {
  const profile = normalizeProfile(baseProfile());

  assert.throws(
    () =>
      buildCommandPlan({
        profile,
        outDir: "tmp/deploy-profile",
        ignition: { buildProfile: "" },
      }),
    /--build-profile requires a value/,
  );
  assert.throws(
    () =>
      buildCommandPlan({
        profile,
        outDir: "tmp/deploy-profile",
        ignition: { buildProfile: "prod profile" },
      }),
    /--build-profile cannot contain whitespace/,
  );
});

test("isBootstrapStateUnavailable only allows missing new deployment state", () => {
  const missingDeploymentState = new Error(
    "ENOENT: no such file or directory, open 'ignition/deployments/sepolia-open-workers/deployed_addresses.json'",
  ) as NodeJS.ErrnoException;
  missingDeploymentState.code = "ENOENT";
  missingDeploymentState.path =
    "ignition/deployments/sepolia-open-workers/deployed_addresses.json";
  const missingArtifact = new Error(
    "ENOENT: no such file or directory, open 'contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json'",
  ) as NodeJS.ErrnoException;
  missingArtifact.code = "ENOENT";
  missingArtifact.path =
    "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json";

  assert.equal(isBootstrapStateUnavailable(missingDeploymentState), true);
  assert.equal(isBootstrapStateUnavailable(missingArtifact), false);
  assert.equal(
    isBootstrapStateUnavailable(
      new Error(
        "sepolia deployed_addresses.json is missing OpenWorkers#OpenExecutor",
      ),
    ),
    true,
  );
  assert.equal(
    isBootstrapStateUnavailable(
      new Error("hoodi deployed_addresses.json is missing TestOFT#TestOFT"),
    ),
    true,
  );
  assert.equal(
    isBootstrapStateUnavailable(
      new Error(
        "hoodi deployed_addresses.json is missing OpenWorkers#OpenPriceFeed",
      ),
    ),
    false,
  );
});

function stateWithPriceFeeds(profile: DeploymentProfile) {
  return buildDeploymentState({
    profile,
    workerDeployedAddresses: {
      sepolia: deployedWorkers("0x1111111111111111111111111111111111111111"),
      hoodi: deployedWorkers("0x2222222222222222222222222222222222222222"),
    },
    testOFTDeployedAddresses:
      profile.mode === "test-oft-rehearsal"
        ? {
            sepolia: deployedTestOFT(
              "0x1111111111111111111111111111111111111111",
            ),
            hoodi: deployedTestOFT(
              "0x2222222222222222222222222222222222222222",
            ),
          }
        : undefined,
    generatedAt: "2026-07-05T00:00:00.000Z",
  });
}

function deployedWorkers(prefix: string, includePriceFeed = true) {
  const base = prefix.slice(0, -1);
  return {
    "OpenWorkers#OpenExecutor": `${base}2`,
    "OpenWorkers#OpenDVN": `${base}3`,
    ...(includePriceFeed ? { "OpenWorkers#OpenPriceFeed": `${base}4` } : {}),
  };
}

function deployedTestOFT(prefix: string) {
  const base = prefix.slice(0, -1);
  return {
    "TestOFT#TestOFT": `${base}1`,
  };
}

function externalProfile() {
  const profile = baseProfile();
  profile.mode = "external-oapp";
  const sepolia = profile.chains[0] as Record<string, unknown>;
  const hoodi = profile.chains[1] as Record<string, unknown>;
  sepolia.oapp = "0xaAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa";
  hoodi.oapp = "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB";
  delete sepolia.testOFTDeploymentId;
  delete hoodi.testOFTDeploymentId;
  return profile;
}

function baseProfile() {
  return {
    environment: "testnet",
    mode: "test-oft-rehearsal",
    databaseUrl:
      "postgres://laz_worker:laz_worker@localhost:5432/laz_worker?sslmode=disable",
    metricsListenAddress: ":9090",
    owner: "0x1111111111111111111111111111111111111111",
    priceFeedSubmitters: ["0x2222222222222222222222222222222222222222"],
    initialRecipient: "0x1111111111111111111111111111111111111111",
    canaryTreasury: "0x1111111111111111111111111111111111111111",
    minOwnerNativeBalanceWei: "10000000000000000",
    minCanaryNativeBalanceWei: "10000000000000000",
    dvnMode: "active",
    services: {
      executor: true,
      dvn: true,
    },
    signers: [
      {
        id: "0x2222222222222222222222222222222222222222",
        type: "keystore",
        keystore: {
          path: "/run/secrets/testnet-worker-keystore.json",
          passwordEnv: "KEYSTORE_PASSWORD",
        },
      },
    ],
    token: {
      name: "Oh My Lazier Test OFT",
      symbol: "OMLTOFT",
    },
    chains: [
      {
        key: "sepolia",
        network: "sepolia",
        name: "ethereum-sepolia",
        eid: 40161,
        chainId: 11155111,
        rpcUrlEnv: "SEPOLIA_RPC_URL",
        deploymentId: "sepolia-open-workers",
        testOFTDeploymentId: "sepolia-test-oft",
        initialSupply: "1000000000000000000000000",
        minCanaryTokenBalance: "1000000000000000",
        confirmations: 12,
        indexerQueryBlockRange: 500,
        txRoles: {
          executor: {
            signer: "0x2222222222222222222222222222222222222222",
            maxFeePerGasWei: "100000000000",
            maxPriorityFeePerGasWei: "1000000000",
            minNativeBalanceWei: "100000000000000000",
          },
          dvn: {
            signer: "0x2222222222222222222222222222222222222222",
            maxFeePerGasWei: "100000000000",
            maxPriorityFeePerGasWei: "1000000000",
            minNativeBalanceWei: "100000000000000000",
          },
        },
      },
      {
        key: "hoodi",
        network: "hoodi",
        name: "hoodi",
        eid: 40449,
        chainId: 560048,
        rpcUrlEnv: "HOODI_RPC_URL",
        deploymentId: "hoodi-open-workers",
        testOFTDeploymentId: "hoodi-test-oft",
        initialSupply: "0",
        minCanaryTokenBalance: "0",
        confirmations: 12,
        indexerQueryBlockRange: 500,
        txRoles: {
          executor: {
            signer: "0x2222222222222222222222222222222222222222",
            maxFeePerGasWei: "100000000000",
            maxPriorityFeePerGasWei: "1000000000",
            minNativeBalanceWei: "100000000000000000",
          },
          dvn: {
            signer: "0x2222222222222222222222222222222222222222",
            maxFeePerGasWei: "100000000000",
            maxPriorityFeePerGasWei: "1000000000",
            minNativeBalanceWei: "100000000000000000",
          },
        },
      },
    ],
    pathway: {
      maxMessageSize: 10000,
      enforcedLzReceiveGas: "200000",
      minLzReceiveGas: "200000",
      maxLzReceiveGas: "1000000",
      priceSnapshot: {
        dstGasPriceInSrcToken: "1",
        dstDataFeePerByteInSrcToken: "0",
        staleAfter: "1800",
        maxAgeSeconds: "1800",
      },
      executorFee: {
        fixedFeeWei: "0",
        dstGasOverhead: "50000",
        dataSizeOverheadBytes: "0",
        marginBps: 1000,
      },
      dvnFee: {
        fixedFeeWei: "0",
        dstGasOverhead: "150000",
        dataSizeOverheadBytes: "0",
        marginBps: 1000,
      },
    },
  };
}
