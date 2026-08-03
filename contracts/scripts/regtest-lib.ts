// Env/CLI helpers used only by the regtest harness scripts (regtest-deploy,
// regtest-send, set-uln-confs-oneside). These predate the OML_SCRIPT_PARAMS
// command envelope; the main script surface must not import from here.
import {
  createPublicClient,
  createWalletClient,
  defineChain,
  http,
  type Address,
  type Hex,
  type PublicClient,
} from "viem";
import { privateKeyToAccount } from "viem/accounts";

export type ChainClients = {
  account: ReturnType<typeof privateKeyToAccount>;
  publicClient: PublicClient;
  walletClient: ReturnType<typeof createWalletClient>;
};

export function requiredEnv(name: string): string {
  const value = process.env[name];
  if (value === undefined || value === "") {
    throw new Error(`${name} is required`);
  }
  return value;
}

export function optionalEnv(name: string, fallback: string): string {
  const value = process.env[name];
  return value === undefined || value === "" ? fallback : value;
}

export function envAddress(name: string): Address {
  return parseAddress(requiredEnv(name), name);
}

export function envAddressList(name: string): Address[] {
  return parseAddressList(requiredEnv(name), name);
}

export function parseAddressList(value: string, label: string): Address[] {
  const parts = value.split(",");
  if (parts.length === 0) {
    throw new Error(`${label} must contain at least one EVM address`);
  }
  return parts.map((part, index) =>
    parseAddress(part.trim(), `${label}[${index}]`)
  );
}

export function envBigInt(name: string): bigint {
  const value = requiredEnv(name);
  if (!/^[0-9]+$/.test(value)) {
    throw new Error(`${name} must be an unsigned integer`);
  }
  return BigInt(value);
}

export function envUint32(name: string): number {
  const value = envBigInt(name);
  if (value > 0xffffffffn) {
    throw new Error(`${name} exceeds uint32`);
  }
  return Number(value);
}

export function createClients(): ChainClients {
  const { rpcURL, chain } = chainFromEnv();
  const privateKey = normalizePrivateKey(requiredEnv("PRIVATE_KEY"));
  const account = privateKeyToAccount(privateKey);
  const transport = http(rpcURL);
  return {
    account,
    publicClient: createPublicClient({ chain, transport }),
    walletClient: createWalletClient({ account, chain, transport }),
  };
}

export async function assertConfiguredChain(
  publicClient: PublicClient
): Promise<void> {
  const configuredChainID = publicClient.chain?.id;
  if (configuredChainID === undefined) {
    throw new Error("public client is missing configured CHAIN_ID");
  }
  const rpcChainID = await publicClient.getChainId();
  if (rpcChainID !== configuredChainID) {
    throw new Error(
      `RPC chain id ${rpcChainID} does not match configured CHAIN_ID ${configuredChainID}`
    );
  }
}

export async function waitForContract(
  publicClient: PublicClient,
  hash: Hex
): Promise<Address> {
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`deployment transaction ${hash} failed`);
  }
  if (
    receipt.contractAddress === null ||
    receipt.contractAddress === undefined
  ) {
    throw new Error(`deployment transaction ${hash} did not create a contract`);
  }
  return receipt.contractAddress;
}

function chainFromEnv() {
  const rpcURL = requiredEnv("RPC_URL");
  const chainID = Number(envBigInt("CHAIN_ID"));
  const networkName = optionalEnv("NETWORK_NAME", `chain-${chainID}`);
  const chain = defineChain({
    id: chainID,
    name: networkName,
    nativeCurrency: { name: "Ether", symbol: "ETH", decimals: 18 },
    rpcUrls: { default: { http: [rpcURL] } },
  });
  return { rpcURL, chain };
}

function parseAddress(value: string, label: string): Address {
  if (!/^0x[0-9a-fA-F]{40}$/.test(value)) {
    throw new Error(`${label} must be an EVM address`);
  }
  return value as Address;
}

function normalizePrivateKey(value: string): Hex {
  const normalized = value.startsWith("0x") ? value : `0x${value}`;
  if (!/^0x[0-9a-fA-F]{64}$/.test(normalized)) {
    throw new Error("PRIVATE_KEY must be a 32-byte hex private key");
  }
  return normalized as Hex;
}

