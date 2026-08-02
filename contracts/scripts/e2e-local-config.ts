export type LocalE2EChainKey = "a" | "b";

export type LocalE2EChainSpec = {
  key: LocalE2EChainKey;
  name: string;
  eid: number;
  chainId: number;
  hostRpcUrl: string;
  containerRpcUrl: string;
};

export type LocalE2EKMSConfig = {
  hostEndpoint: string;
  containerEndpoint: string;
  region: string;
};

type ResolveValue = (name: string, fallback: string) => string;

const environmentValue: ResolveValue = (name, fallback) =>
  process.env[name] ?? fallback;

const defaultChains = [
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
] as const satisfies readonly LocalE2EChainSpec[];

const defaultDatabaseURLs = {
  host: "postgres://laz_worker:laz_worker@127.0.0.1:55433/laz_worker?sslmode=disable",
  container:
    "postgres://laz_worker:laz_worker@postgres:5432/laz_worker?sslmode=disable",
} as const;

const defaultKMS = {
  hostEndpoint: "http://127.0.0.1:4566",
  containerEndpoint: "http://localstack:4566",
  region: "us-east-1",
} as const;

export function localE2EChains(
  resolve: ResolveValue = environmentValue
): readonly LocalE2EChainSpec[] {
  return defaultChains.map((chain) => ({
    ...chain,
    hostRpcUrl: resolve(
      `E2E_CHAIN_${chain.key.toUpperCase()}_HOST_RPC_URL`,
      chain.hostRpcUrl
    ),
    containerRpcUrl: resolve(
      `E2E_CHAIN_${chain.key.toUpperCase()}_CONTAINER_RPC_URL`,
      chain.containerRpcUrl
    ),
  }));
}

export function localE2EDatabaseURL(
  mode: "host" | "container",
  resolve: ResolveValue = environmentValue
): string {
  return resolve(
    mode === "host" ? "E2E_HOST_DATABASE_URL" : "E2E_CONTAINER_DATABASE_URL",
    defaultDatabaseURLs[mode]
  );
}

export function localE2EKMS(
  resolve: ResolveValue = environmentValue
): LocalE2EKMSConfig {
  return {
    hostEndpoint: resolve("E2E_KMS_HOST_ENDPOINT", defaultKMS.hostEndpoint),
    containerEndpoint: resolve(
      "E2E_KMS_CONTAINER_ENDPOINT",
      defaultKMS.containerEndpoint
    ),
    region: resolve("E2E_KMS_REGION", defaultKMS.region),
  };
}
