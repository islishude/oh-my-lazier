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

let cachedCLIParams: Map<string, string> | undefined;

export function requiredEnv(name: string): string {
  const value = optionalParam(name);
  if (value === undefined || value === "") {
    throw new Error(`${name} or --${flagName(name)} is required`);
  }
  return value;
}

export function optionalEnv(name: string, fallback: string): string {
  const value = optionalParam(name);
  return value === undefined || value === "" ? fallback : value;
}

export function optionalParam(name: string): string | undefined {
  const flagValue = cliParams().get(flagName(name));
  if (flagValue !== undefined) {
    return flagValue;
  }
  return process.env[name];
}

export function envAddress(name: string): Address {
  return parseAddress(requiredEnv(name), name);
}

export function optionalAddress(name: string): Address | undefined {
  const value = optionalParam(name);
  if (value === undefined || value === "") {
    return undefined;
  }
  return parseAddress(value, name);
}

export function envAddressList(name: string): Address[] {
  return parseAddressList(requiredEnv(name), name);
}

export function optionalAddressList(name: string): Address[] | undefined {
  const value = optionalParam(name);
  if (value === undefined || value === "") {
    return undefined;
  }
  return parseAddressList(value, name);
}

export function parseAddressList(value: string, label: string): Address[] {
  const parts = value.split(",");
  if (parts.length === 0) {
    throw new Error(`${label} must contain at least one EVM address`);
  }
  return parts.map((part, index) =>
    parseAddress(part.trim(), `${label}[${index}]`),
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

export function optionalBigInt(name: string): bigint | undefined {
  const value = optionalParam(name);
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

export function optionalBool(name: string): boolean | undefined {
  const value = optionalParam(name);
  if (value === undefined || value === "") {
    return undefined;
  }
  switch (value.toLowerCase()) {
    case "1":
    case "true":
    case "yes":
      return true;
    case "0":
    case "false":
    case "no":
      return false;
    default:
      throw new Error(`${name} must be a boolean`);
  }
}

export function parseCLIParams(args: readonly string[]): Map<string, string> {
  const params = new Map<string, string>();
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === "--") {
      break;
    }
    if (!arg.startsWith("--")) {
      continue;
    }
    const withoutPrefix = arg.slice(2);
    if (withoutPrefix === "") {
      continue;
    }
    const equalsIndex = withoutPrefix.indexOf("=");
    if (equalsIndex >= 0) {
      const key = normalizeFlagName(withoutPrefix.slice(0, equalsIndex));
      params.set(key, withoutPrefix.slice(equalsIndex + 1));
      continue;
    }
    const key = normalizeFlagName(withoutPrefix);
    const next = args[i + 1];
    if (next !== undefined && !next.startsWith("--")) {
      params.set(key, next);
      i += 1;
    } else {
      params.set(key, "true");
    }
  }
  return params;
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

function cliParams(): Map<string, string> {
  if (cachedCLIParams === undefined) {
    cachedCLIParams = parseCLIParams(process.argv.slice(2));
  }
  return cachedCLIParams;
}

function flagName(name: string): string {
  return normalizeFlagName(name.replaceAll("_", "-"));
}

function normalizeFlagName(name: string): string {
  return name.toLowerCase().replaceAll("_", "-");
}
