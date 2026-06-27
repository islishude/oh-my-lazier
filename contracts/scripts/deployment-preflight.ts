import {
  createPublicClientFromEnv,
  envAddress,
  jsonStringify,
  loadArtifact,
  optionalBigInt,
} from "./lib.js";
import type { Abi, Address, PublicClient } from "viem";

export type OwnedContractCheck = {
  label: string;
  address: Address;
  owner: Address;
};

export type BalanceCheck = {
  address: Address;
  balance: bigint;
  minBalance: bigint;
};

export type DeploymentPreflightReport = {
  chainId: number;
  expectedOwner: Address;
  contracts: OwnedContractCheck[];
  ownerNativeBalance: BalanceCheck;
  canaryTreasury?: {
    nativeBalance: BalanceCheck;
    tokenBalance: BalanceCheck;
  };
  testOFTTotalSupply?: {
    actual: bigint;
    expected: bigint;
  };
};

export type DeploymentPreflightInput = {
  publicClient: PublicClient;
  testOFT: Address;
  openExecutor: Address;
  openDVN: Address;
  expectedOwner: Address;
  minOwnerNativeBalance: bigint;
  canaryTreasury?: Address;
  minCanaryNativeBalance: bigint;
  minCanaryTokenBalance: bigint;
  expectedTestOFTTotalSupply?: bigint;
  testOFTAbi: Abi;
  openExecutorAbi: Abi;
  openDVNAbi: Abi;
};

export async function readDeploymentPreflight(
  input: DeploymentPreflightInput,
): Promise<DeploymentPreflightReport> {
  const [chainId, testOFTOwner, openExecutorOwner, openDVNOwner, ownerBalance] =
    await Promise.all([
      input.publicClient.getChainId(),
      readOwner(input.publicClient, input.testOFT, input.testOFTAbi),
      readOwner(input.publicClient, input.openExecutor, input.openExecutorAbi),
      readOwner(input.publicClient, input.openDVN, input.openDVNAbi),
      input.publicClient.getBalance({ address: input.expectedOwner }),
    ]);

  const report: DeploymentPreflightReport = {
    chainId,
    expectedOwner: input.expectedOwner,
    contracts: [
      { label: "TestOFT", address: input.testOFT, owner: testOFTOwner },
      {
        label: "OpenExecutor",
        address: input.openExecutor,
        owner: openExecutorOwner,
      },
      { label: "OpenDVN", address: input.openDVN, owner: openDVNOwner },
    ],
    ownerNativeBalance: {
      address: input.expectedOwner,
      balance: ownerBalance,
      minBalance: input.minOwnerNativeBalance,
    },
  };

  if (input.canaryTreasury !== undefined) {
    const [nativeBalance, tokenBalance] = await Promise.all([
      input.publicClient.getBalance({ address: input.canaryTreasury }),
      readBalanceOf(input.publicClient, input.testOFT, input.testOFTAbi, [
        input.canaryTreasury,
      ]),
    ]);
    report.canaryTreasury = {
      nativeBalance: {
        address: input.canaryTreasury,
        balance: nativeBalance,
        minBalance: input.minCanaryNativeBalance,
      },
      tokenBalance: {
        address: input.canaryTreasury,
        balance: tokenBalance,
        minBalance: input.minCanaryTokenBalance,
      },
    };
  }

  if (input.expectedTestOFTTotalSupply !== undefined) {
    report.testOFTTotalSupply = {
      actual: await readTotalSupply(
        input.publicClient,
        input.testOFT,
        input.testOFTAbi,
      ),
      expected: input.expectedTestOFTTotalSupply,
    };
  }

  return report;
}

export function validateDeploymentPreflight(
  report: DeploymentPreflightReport,
): string[] {
  const errors: string[] = [];
  for (const contract of report.contracts) {
    if (contract.owner.toLowerCase() !== report.expectedOwner.toLowerCase()) {
      errors.push(
        `${contract.label} owner ${contract.owner} does not match EXPECTED_OWNER ${report.expectedOwner}`,
      );
    }
  }
  appendBalanceError(errors, "EXPECTED_OWNER", report.ownerNativeBalance);
  if (report.canaryTreasury !== undefined) {
    appendBalanceError(
      errors,
      "CANARY_TREASURY native",
      report.canaryTreasury.nativeBalance,
    );
    appendBalanceError(
      errors,
      "CANARY_TREASURY TestOFT",
      report.canaryTreasury.tokenBalance,
    );
  }
  if (
    report.testOFTTotalSupply !== undefined &&
    report.testOFTTotalSupply.actual !== report.testOFTTotalSupply.expected
  ) {
    errors.push(
      `TestOFT totalSupply ${report.testOFTTotalSupply.actual} does not match EXPECTED_TOTAL_SUPPLY ${report.testOFTTotalSupply.expected}`,
    );
  }
  return errors;
}

function appendBalanceError(
  errors: string[],
  label: string,
  check: BalanceCheck,
): void {
  if (check.balance < check.minBalance) {
    errors.push(
      `${label} balance ${check.balance} is below required minimum ${check.minBalance} at ${check.address}`,
    );
  }
}

async function readOwner(
  publicClient: PublicClient,
  address: Address,
  abi: Abi,
): Promise<Address> {
  return (await publicClient.readContract({
    address,
    abi,
    functionName: "owner",
  })) as Address;
}

async function readBalanceOf(
  publicClient: PublicClient,
  address: Address,
  abi: Abi,
  args: readonly [Address],
): Promise<bigint> {
  return (await publicClient.readContract({
    address,
    abi,
    functionName: "balanceOf",
    args,
  })) as bigint;
}

async function readTotalSupply(
  publicClient: PublicClient,
  address: Address,
  abi: Abi,
): Promise<bigint> {
  return (await publicClient.readContract({
    address,
    abi,
    functionName: "totalSupply",
  })) as bigint;
}

function optionalAddress(name: string): Address | undefined {
  const value = process.env[name];
  if (value === undefined || value === "") {
    return undefined;
  }
  if (!/^0x[0-9a-fA-F]{40}$/.test(value)) {
    throw new Error(`${name} must be an EVM address`);
  }
  return value as Address;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const testOFTArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
  );
  const openExecutorArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
  );
  const openDVNArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
  );

  const report = await readDeploymentPreflight({
    publicClient: createPublicClientFromEnv(),
    testOFT: envAddress("TEST_OFT"),
    openExecutor: envAddress("OPEN_EXECUTOR"),
    openDVN: envAddress("OPEN_DVN"),
    expectedOwner: envAddress("EXPECTED_OWNER"),
    minOwnerNativeBalance: optionalBigInt("MIN_OWNER_NATIVE_BALANCE") ?? 0n,
    canaryTreasury: optionalAddress("CANARY_TREASURY"),
    minCanaryNativeBalance: optionalBigInt("MIN_CANARY_NATIVE_BALANCE") ?? 0n,
    minCanaryTokenBalance: optionalBigInt("MIN_CANARY_TOKEN_BALANCE") ?? 0n,
    expectedTestOFTTotalSupply: optionalBigInt("EXPECTED_TOTAL_SUPPLY"),
    testOFTAbi: testOFTArtifact.abi,
    openExecutorAbi: openExecutorArtifact.abi,
    openDVNAbi: openDVNArtifact.abi,
  });
  const errors = validateDeploymentPreflight(report);
  console.log(jsonStringify({ ok: errors.length === 0, ...report, errors }));
  if (errors.length > 0) {
    process.exitCode = 1;
  }
}
