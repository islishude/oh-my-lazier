import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import {
  createPublicClient,
  createWalletClient,
  defineChain,
  getAddress,
  http,
  type Abi,
  type Address,
  type Hex,
  type PublicClient,
  type WalletClient,
} from "viem";
import { privateKeyToAccount } from "viem/accounts";
import {
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  encodeExecutorConfig,
  encodeUlnConfig,
  requiredDVNsConfig,
} from "./lz-config.js";
import {
  addressToBytes32,
  jsonStringify,
  loadArtifact,
  waitForContract,
} from "./lib.js";
import type {
  LocalChainDeployment as ChainDeployment,
  LocalE2EDeployment,
} from "./e2e-local-artifacts.js";

const tmpDir = process.env.E2E_TMP_DIR ?? "tmp/e2e";
const deployerPrivateKey = normalizePrivateKey(
  process.env.E2E_DEPLOYER_PRIVATE_KEY ??
    "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
);
const workerPrivateKey = normalizePrivateKey(
  process.env.E2E_WORKER_PRIVATE_KEY ??
    "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
);
const deployer = privateKeyToAccount(deployerPrivateKey);
const worker = privateKeyToAccount(workerPrivateKey);

const endpointArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/EndpointV2.sol/EndpointV2.json",
);
const sendUlnArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json",
);
const receiveUlnArtifact = loadArtifact(
  "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
);
const oftArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);
const openExecutorArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
);
const openDVNArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
);

const localChains = [
  {
    key: "a",
    name: "local-anvil-a",
    eid: 90101,
    chainId: 31337,
    hostRpcUrl: "http://127.0.0.1:18545",
    containerRpcUrl: "http://anvil-a:8545",
  },
  {
    key: "b",
    name: "local-anvil-b",
    eid: 90102,
    chainId: 31338,
    hostRpcUrl: "http://127.0.0.1:18546",
    containerRpcUrl: "http://anvil-b:8545",
  },
] as const;

const maxMessageSize = 10_000;
const minLzReceiveGas = 100_000n;
const lzReceiveGas = 250_000n;
const maxLzReceiveGas = 1_000_000n;
const confirmations = 1n;
const initialSupply = 1_000_000n * 10n ** 18n;

type LocalChainSpec = (typeof localChains)[number];
type ChainConnectionSpec = {
  name: string;
  chainId: number;
  hostRpcUrl: string;
};

type Clients = {
  account: typeof deployer;
  publicClient: PublicClient;
  walletClient: WalletClient;
};

await mkdir(tmpDir, { recursive: true });

const deployments = await Promise.all(
  localChains.map((chain) => deployChain(chain)),
);
const [chainA, chainB] = deployments;

await configureDirection(chainA, chainB);
await configureDirection(chainB, chainA);

const output: LocalE2EDeployment = {
  generatedAt: new Date().toISOString(),
  deployer: getAddress(deployer.address),
  worker: getAddress(worker.address),
  parameters: {
    confirmations: confirmations.toString(),
    maxMessageSize,
    minLzReceiveGas: minLzReceiveGas.toString(),
    lzReceiveGas: lzReceiveGas.toString(),
    maxLzReceiveGas: maxLzReceiveGas.toString(),
  },
  chains: { a: chainA, b: chainB },
};

await writeFile(
  path.join(tmpDir, "deployments.json"),
  `${jsonStringify(output)}\n`,
);
await writeFile(
  path.join(tmpDir, "worker.host.yaml"),
  workerConfig(output, "host"),
);
await writeFile(
  path.join(tmpDir, "worker.container.yaml"),
  workerConfig(output, "container"),
);

console.log(jsonStringify(output));

async function deployChain(spec: LocalChainSpec): Promise<ChainDeployment> {
  const clients = clientsFor(spec);
  await assertChainID(clients.publicClient, spec);

  const endpoint = await deployContract(clients, `${spec.name} EndpointV2`, {
    abi: endpointArtifact.abi,
    bytecode: endpointArtifact.bytecode,
    args: [spec.eid, deployer.address],
  });
  const sendUln = await deployContract(clients, `${spec.name} SendUln302`, {
    abi: sendUlnArtifact.abi,
    bytecode: sendUlnArtifact.bytecode,
    args: [endpoint, 0n, 0n],
  });
  const receiveUln = await deployContract(
    clients,
    `${spec.name} ReceiveUln302`,
    {
      abi: receiveUlnArtifact.abi,
      bytecode: receiveUlnArtifact.bytecode,
      args: [endpoint],
    },
  );
  const oft = await deployContract(clients, `${spec.name} TestOFT`, {
    abi: oftArtifact.abi,
    bytecode: oftArtifact.bytecode,
    args: [
      `Local OFT ${spec.key.toUpperCase()}`,
      `LOFT${spec.key.toUpperCase()}`,
      endpoint,
      deployer.address,
      deployer.address,
      spec.key === "a" ? initialSupply : 0n,
    ],
  });
  const openExecutor = await deployContract(
    clients,
    `${spec.name} OpenExecutor`,
    {
      abi: openExecutorArtifact.abi,
      bytecode: openExecutorArtifact.bytecode,
      args: [deployer.address],
    },
  );
  const primaryOpenDVN = await deployContract(
    clients,
    `${spec.name} OpenDVN primary`,
    {
      abi: openDVNArtifact.abi,
      bytecode: openDVNArtifact.bytecode,
      args: [deployer.address],
    },
  );
  const secondaryOpenDVN = await deployContract(
    clients,
    `${spec.name} OpenDVN secondary`,
    {
      abi: openDVNArtifact.abi,
      bytecode: openDVNArtifact.bytecode,
      args: [deployer.address],
    },
  );

  return {
    ...spec,
    endpoint,
    sendUln,
    receiveUln,
    oft,
    openExecutor,
    primaryOpenDVN,
    secondaryOpenDVN,
  };
}

async function configureDirection(
  source: ChainDeployment,
  destination: ChainDeployment,
) {
  const sourceClients = clientsFor(source);
  const destinationClients = clientsFor(destination);
  const sourceDVNs = [source.primaryOpenDVN, source.secondaryOpenDVN];
  const sourceUlnConfig = requiredDVNsConfig(confirmations, sourceDVNs);
  const executorConfig = {
    maxMessageSize,
    executor: source.openExecutor,
  };

  await writeTx(
    sourceClients,
    `${source.name} Endpoint.registerLibrary SendUln302`,
    source.endpoint,
    endpointArtifact.abi,
    "registerLibrary",
    [source.sendUln],
  );
  await writeTx(
    sourceClients,
    `${source.name} Endpoint.registerLibrary ReceiveUln302`,
    source.endpoint,
    endpointArtifact.abi,
    "registerLibrary",
    [source.receiveUln],
  );
  await writeTx(
    sourceClients,
    `${source.name} SendUln302.setDefaultUlnConfigs`,
    source.sendUln,
    sendUlnArtifact.abi,
    "setDefaultUlnConfigs",
    [[{ eid: destination.eid, config: defaultUlnConfig(sourceUlnConfig) }]],
  );
  await writeTx(
    sourceClients,
    `${source.name} ReceiveUln302.setDefaultUlnConfigs`,
    source.receiveUln,
    receiveUlnArtifact.abi,
    "setDefaultUlnConfigs",
    [[{ eid: destination.eid, config: defaultUlnConfig(sourceUlnConfig) }]],
  );
  await writeTx(
    sourceClients,
    `${source.name} SendUln302.setDefaultExecutorConfigs`,
    source.sendUln,
    sendUlnArtifact.abi,
    "setDefaultExecutorConfigs",
    [[{ eid: destination.eid, config: executorConfig }]],
  );
  await writeTx(
    sourceClients,
    `${source.name} Endpoint.setDefaultSendLibrary`,
    source.endpoint,
    endpointArtifact.abi,
    "setDefaultSendLibrary",
    [destination.eid, source.sendUln],
  );
  await writeTx(
    sourceClients,
    `${source.name} Endpoint.setDefaultReceiveLibrary`,
    source.endpoint,
    endpointArtifact.abi,
    "setDefaultReceiveLibrary",
    [destination.eid, source.receiveUln, 0n],
  );
  await writeTx(
    sourceClients,
    `${source.name} TestOFT.setPeer`,
    source.oft,
    oftArtifact.abi,
    "setPeer",
    [destination.eid, addressToBytes32(destination.oft)],
  );
  await writeTx(
    sourceClients,
    `${source.name} Endpoint.setConfig SendUln302`,
    source.endpoint,
    endpointArtifact.abi,
    "setConfig",
    [
      source.oft,
      source.sendUln,
      [
        {
          eid: destination.eid,
          configType: CONFIG_TYPE_EXECUTOR,
          config: encodeExecutorConfig(executorConfig),
        },
        {
          eid: destination.eid,
          configType: CONFIG_TYPE_ULN,
          config: encodeUlnConfig(sourceUlnConfig),
        },
      ],
    ],
  );
  await writeTx(
    sourceClients,
    `${source.name} Endpoint.setConfig ReceiveUln302`,
    source.endpoint,
    endpointArtifact.abi,
    "setConfig",
    [
      source.oft,
      source.receiveUln,
      [
        {
          eid: destination.eid,
          configType: CONFIG_TYPE_ULN,
          config: encodeUlnConfig(sourceUlnConfig),
        },
      ],
    ],
  );

  await configureSourceWorkers(sourceClients, source, destination);
  await authorizeDestinationVerifiers(destinationClients, destination);
}

async function configureSourceWorkers(
  clients: Clients,
  source: ChainDeployment,
  destination: ChainDeployment,
) {
  const pathwayConfig = {
    enabled: true,
    maxMessageSize: BigInt(maxMessageSize),
    minLzReceiveGas,
    maxLzReceiveGas,
  };
  const timestamp = BigInt(
    (await clients.publicClient.getBlock()).timestamp,
  );
  const executorPriceConfig = {
    baseFee: 1n,
    dstGasPriceInSrcToken: 1n,
    bufferBps: 0,
    updatedAt: timestamp,
    staleAfter: 86_400n,
  };
  const dvnPriceConfig = {
    baseFee: 0n,
    dstGasPriceInSrcToken: 0n,
    bufferBps: 0,
    updatedAt: timestamp,
    staleAfter: 86_400n,
  };
  for (const workerAddress of [
    source.openExecutor,
    source.primaryOpenDVN,
    source.secondaryOpenDVN,
  ]) {
    const abi =
      workerAddress === source.openExecutor
        ? openExecutorArtifact.abi
        : openDVNArtifact.abi;
    await writeTx(
      clients,
      `${source.name} worker.setAllowedSendLib ${workerAddress}`,
      workerAddress,
      abi,
      "setAllowedSendLib",
      [source.sendUln, true],
    );
    await writeTx(
      clients,
      `${source.name} worker.setPathwayConfig ${workerAddress}`,
      workerAddress,
      abi,
      "setPathwayConfig",
      [destination.eid, source.oft, pathwayConfig],
    );
    await writeTx(
      clients,
      `${source.name} worker.setPriceConfig ${workerAddress}`,
      workerAddress,
      abi,
      "setPriceConfig",
      [
        destination.eid,
        workerAddress === source.openExecutor
          ? executorPriceConfig
          : dvnPriceConfig,
      ],
    );
  }
}

async function authorizeDestinationVerifiers(
  clients: Clients,
  chain: ChainDeployment,
) {
  for (const openDVN of [chain.primaryOpenDVN, chain.secondaryOpenDVN]) {
    await writeTx(
      clients,
      `${chain.name} OpenDVN.setVerifier worker ${openDVN}`,
      openDVN,
      openDVNArtifact.abi,
      "setVerifier",
      [worker.address, true],
    );
    await writeTx(
      clients,
      `${chain.name} OpenDVN.setVerifier deployer ${openDVN}`,
      openDVN,
      openDVNArtifact.abi,
      "setVerifier",
      [deployer.address, true],
    );
  }
}

async function deployContract(
  clients: Clients,
  label: string,
  artifact: { abi: Abi; bytecode: Hex; args: readonly unknown[] },
): Promise<Address> {
  const hash = await clients.walletClient.deployContract({
    abi: artifact.abi,
    bytecode: artifact.bytecode,
    args: [...artifact.args],
    account: clients.account,
    chain: null,
  });
  const address = await waitForContract(clients.publicClient, hash);
  console.log(`${label}: ${address}`);
  return getAddress(address);
}

async function writeTx(
  clients: Clients,
  label: string,
  address: Address,
  abi: Abi,
  functionName: string,
  args: readonly unknown[],
): Promise<void> {
  const hash = await clients.walletClient.writeContract({
    address,
    abi,
    functionName,
    args: [...args],
    account: clients.account,
    chain: null,
  });
  const receipt = await clients.publicClient.waitForTransactionReceipt({
    hash,
  });
  if (receipt.status !== "success") {
    throw new Error(`${label} transaction ${hash} failed`);
  }
  console.log(`${label}: ${hash}`);
}

function clientsFor(spec: ChainConnectionSpec): Clients {
  const chain = defineChain({
    id: spec.chainId,
    name: spec.name,
    nativeCurrency: { name: "Ether", symbol: "ETH", decimals: 18 },
    rpcUrls: { default: { http: [spec.hostRpcUrl] } },
  });
  const transport = http(spec.hostRpcUrl);
  return {
    account: deployer,
    publicClient: createPublicClient({ chain, transport }),
    walletClient: createWalletClient({ account: deployer, chain, transport }),
  };
}

async function assertChainID(publicClient: PublicClient, spec: LocalChainSpec) {
  const chainId = await publicClient.getChainId();
  if (chainId !== spec.chainId) {
    throw new Error(
      `${spec.name} chain_id ${chainId} does not match expected ${spec.chainId}`,
    );
  }
}

function workerConfig(output: LocalE2EDeployment, mode: "host" | "container") {
  const rpcURL = (chain: ChainDeployment) =>
    mode === "host" ? chain.hostRpcUrl : chain.containerRpcUrl;
  const keystorePath =
    mode === "host"
      ? path.join(tmpDir, "worker-keystore.json")
      : "/app/tmp/e2e/worker-keystore.json";
  const databaseURL =
    mode === "host"
      ? "postgres://laz_worker:laz_worker@127.0.0.1:55433/laz_worker?sslmode=disable"
      : "postgres://laz_worker:laz_worker@postgres:5432/laz_worker?sslmode=disable";
  const chainList = [output.chains.a, output.chains.b];
  const pathways = [
    [output.chains.a, output.chains.b],
    [output.chains.b, output.chains.a],
  ] as const;
  return `database_url: ${databaseURL}
metrics:
  listen_address: :9090
services:
  executor:
    enabled: true
  dvn:
    enabled: true
signers:
  - id: "${output.worker}"
    type: keystore
    keystore:
      path: ${keystorePath}
      password_env: E2E_KEYSTORE_PASSWORD
pricing:
  enabled: false
chains:
${chainList
  .map(
    (chain) => `  - eid: ${chain.eid}
    name: ${chain.name}
    family: evm
    chain_id: ${chain.chainId}
    endpoint_address: "${chain.endpoint}"
    confirmations: ${output.parameters.confirmations}
    start_block_number: 0
    indexer_query_block_range: 500
    rpc_urls:
      - ${rpcURL(chain)}
    tx_roles:
      executor:
        signer: "${output.worker}"
        max_fee_per_gas_wei: "100000000000"
        max_priority_fee_per_gas_wei: "1000000000"
      dvn:
        signer: "${output.worker}"
        max_fee_per_gas_wei: "100000000000"
        max_priority_fee_per_gas_wei: "1000000000"`,
  )
  .join("\n")}
pathways:
${pathways
  .map(
    ([source, destination]) => `  - src_eid: ${source.eid}
    dst_eid: ${destination.eid}
    src_oapp: "${source.oft}"
    dst_oapp: "${destination.oft}"
    send_lib: "${source.sendUln}"
    receive_lib: "${destination.receiveUln}"
    source_workers:
      open_executor: "${source.openExecutor}"
      open_dvn: "${source.primaryOpenDVN}"
    destination_workers:
      open_dvn: "${destination.primaryOpenDVN}"
    dvn:
      mode: active
    enabled: true
    max_message_size: ${output.parameters.maxMessageSize}
    min_lz_receive_gas: ${output.parameters.minLzReceiveGas}
    max_lz_receive_gas: ${output.parameters.maxLzReceiveGas}`,
  )
  .join("\n")}
`;
}

function normalizePrivateKey(value: string): Hex {
  const normalized = value.startsWith("0x") ? value : `0x${value}`;
  if (!/^0x[0-9a-fA-F]{64}$/.test(normalized)) {
    throw new Error("private key must be a 32-byte hex value");
  }
  return normalized as Hex;
}

function defaultUlnConfig(config: ReturnType<typeof requiredDVNsConfig>) {
  return {
    ...config,
    optionalDVNCount: 0,
  };
}
