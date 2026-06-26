import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  createPublicClient,
  createWalletClient,
  defineChain,
  http,
  type Abi,
  type Address,
  type Hex,
  type PublicClient,
  type WalletClient,
} from "viem";
import { privateKeyToAccount } from "viem/accounts";

export type Artifact = {
  abi: Abi;
  bytecode: Hex;
};

export type ABIArtifact = {
  abi: Abi;
};

export type ChainClients = {
  account: ReturnType<typeof privateKeyToAccount>;
  publicClient: PublicClient;
  walletClient: WalletClient;
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
  const value = requiredEnv(name);
  if (!/^0x[0-9a-fA-F]{40}$/.test(value)) {
    throw new Error(`${name} must be an EVM address`);
  }
  return value as Address;
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

export function optionalBigInt(name: string): bigint | undefined {
  const value = process.env[name];
  if (value === undefined || value === "") {
    return undefined;
  }
  if (!/^[0-9]+$/.test(value)) {
    throw new Error(`${name} must be an unsigned integer`);
  }
  return BigInt(value);
}

export function optionalUint64(name: string, fallback: bigint): bigint {
  const value = optionalBigInt(name);
  return value === undefined ? fallback : value;
}

export function loadArtifact(relativePath: string): Artifact {
  const path = join(process.cwd(), relativePath);
  const artifact = JSON.parse(readFileSync(path, "utf8")) as Artifact;
  if (artifact.bytecode === "0x") {
    throw new Error(`${relativePath} has empty bytecode`);
  }
  return artifact;
}

export function loadABIArtifact(relativePath: string): ABIArtifact {
  const path = join(process.cwd(), relativePath);
  return JSON.parse(readFileSync(path, "utf8")) as ABIArtifact;
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

export function createPublicClientFromEnv(): PublicClient {
  const { rpcURL, chain } = chainFromEnv();
  return createPublicClient({ chain, transport: http(rpcURL) });
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

export async function waitForContract(
  publicClient: PublicClient,
  hash: Hex,
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

export async function waitForTx(
  publicClient: PublicClient,
  label: string,
  hash: Hex,
): Promise<void> {
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`${label} transaction ${hash} failed`);
  }
  console.log(`${label}: ${hash}`);
}

export function addressToBytes32(address: Address): Hex {
  return `0x${address.slice(2).padStart(64, "0")}` as Hex;
}

export function jsonStringify(value: unknown): string {
  return JSON.stringify(
    value,
    (_key, item) => (typeof item === "bigint" ? item.toString() : item),
    2,
  );
}

function normalizePrivateKey(value: string): Hex {
  const normalized = value.startsWith("0x") ? value : `0x${value}`;
  if (!/^0x[0-9a-fA-F]{64}$/.test(normalized)) {
    throw new Error("PRIVATE_KEY must be a 32-byte hex private key");
  }
  return normalized as Hex;
}
