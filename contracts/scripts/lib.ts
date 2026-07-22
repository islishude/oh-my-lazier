import { readFileSync } from "node:fs";
import { join } from "node:path";
import type {
  Abi,
  Account,
  Address,
  Hex,
  PublicClient,
  WalletClient,
} from "viem";

export type Artifact = {
  abi: Abi;
  bytecode: Hex;
};

export type ABIArtifact = {
  abi: Abi;
};

export type ChainClients = {
  account: Account;
  publicClient: PublicClient;
  walletClient: WalletClient;
};

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

export async function waitForTx(
  publicClient: PublicClient,
  label: string,
  hash: Hex
): Promise<void> {
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`${label} transaction ${hash} failed`);
  }
  console.error(`${label}: ${hash}`);
}

export function addressToBytes32(address: Address): Hex {
  return `0x${address.slice(2).padStart(64, "0")}` as Hex;
}

export function jsonStringify(value: unknown): string {
  return JSON.stringify(
    value,
    (_key, item) => (typeof item === "bigint" ? item.toString() : item),
    2
  );
}
