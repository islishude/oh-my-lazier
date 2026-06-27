import { getAddress, isAddressEqual, type Address } from "viem";

export const DEPLOYMENTS_V2_URL =
  "https://docs.layerzero.network/public/data/deploymentsV2.json";
export const DVN_DEPLOYMENTS_URL =
  "https://docs.layerzero.network/public/data/dvnDeployments.json";

export type ExpectedLayerZeroChain = {
  chainKey: string;
  nativeChainId: number;
  eid: string;
  endpointV2: Address;
  sendUln302: Address;
  receiveUln302: Address;
  executor: Address;
  lzExecutor: Address;
  deadDVN: Address;
  layerZeroLabsDVN: Address;
  layerZeroLabsReadDVN: Address;
};

export type DeploymentRecord = {
  chainKey?: string;
  nativeChainId?: number;
  eid?: string | number;
  endpointV2?: { address?: string };
  sendUln302?: { address?: string };
  receiveUln302?: { address?: string };
  executor?: { address?: string };
  lzExecutor?: { address?: string };
  deadDVN?: { address?: string };
};

export type DVNDeploymentRecord = {
  chainKey?: string;
  nativeChainId?: number;
  eid?: string | number;
  version?: number;
  id?: string;
  lzReadCompatible?: boolean;
  dvnAddress?: string;
};

export const expectedLayerZeroChains: readonly ExpectedLayerZeroChain[] = [
  {
    chainKey: "sepolia",
    nativeChainId: 11155111,
    eid: "40161",
    endpointV2: getAddress("0x6EDCE65403992e310A62460808c4b910D972f10f"),
    sendUln302: getAddress("0xcc1ae8Cf5D3904Cef3360A9532B477529b177cCE"),
    receiveUln302: getAddress("0xdAf00F5eE2158dD58E0d3857851c432E34A3A851"),
    executor: getAddress("0x718B92b5CB0a5552039B593faF724D182A881eDA"),
    lzExecutor: getAddress("0x34a561197e4eAe356D41B0B02C59F12a5C576C5A"),
    deadDVN: getAddress("0x8b450b0acF56E1B0e25C581bB04FBAbeeb0644b8"),
    layerZeroLabsDVN: getAddress("0x8eebf8b423b73bfca51a1db4b7354aa0bfca9193"),
    layerZeroLabsReadDVN: getAddress(
      "0x530fbe405189204ef459fa4b767167e4d41e3a37",
    ),
  },
  {
    chainKey: "base-sepolia",
    nativeChainId: 84532,
    eid: "40245",
    endpointV2: getAddress("0x6EDCE65403992e310A62460808c4b910D972f10f"),
    sendUln302: getAddress("0xC1868e054425D378095A003EcbA3823a5D0135C9"),
    receiveUln302: getAddress("0x12523de19dc41c91F7d2093E0CFbB76b17012C8d"),
    executor: getAddress("0x8A3D588D9f6AC041476b094f97FF94ec30169d3D"),
    lzExecutor: getAddress("0xD8C74c92a59c2b5b6390eD54f13193C59249e561"),
    deadDVN: getAddress("0x78551ADC2553EF1858a558F5300F7018Aad2FA7e"),
    layerZeroLabsDVN: getAddress("0xe1a12515f9ab2764b887bf60b923ca494ebbb2d6"),
    layerZeroLabsReadDVN: getAddress(
      "0xbf6ff58f60606edb2f190769b951d825bcb214e2",
    ),
  },
] as const;

export function verifyLayerZeroAddresses(input: {
  deployments: readonly DeploymentRecord[];
  dvns: readonly DVNDeploymentRecord[];
  expected?: readonly ExpectedLayerZeroChain[];
}): string[] {
  const expected = input.expected ?? expectedLayerZeroChains;
  const errors: string[] = [];

  for (const chain of expected) {
    const deployment = input.deployments.find(
      (record) =>
        record.chainKey?.toLowerCase() === chain.chainKey &&
        String(record.eid) === chain.eid,
    );
    if (deployment === undefined) {
      errors.push(
        `${chain.chainKey}: deployment record for EID ${chain.eid} is missing`,
      );
      continue;
    }
    if (deployment.nativeChainId !== chain.nativeChainId) {
      errors.push(
        `${chain.chainKey}: native chain id ${deployment.nativeChainId} != ${chain.nativeChainId}`,
      );
    }
    compareAddress(
      errors,
      chain.chainKey,
      "EndpointV2",
      deployment.endpointV2?.address,
      chain.endpointV2,
    );
    compareAddress(
      errors,
      chain.chainKey,
      "SendUln302",
      deployment.sendUln302?.address,
      chain.sendUln302,
    );
    compareAddress(
      errors,
      chain.chainKey,
      "ReceiveUln302",
      deployment.receiveUln302?.address,
      chain.receiveUln302,
    );
    compareAddress(
      errors,
      chain.chainKey,
      "Executor",
      deployment.executor?.address,
      chain.executor,
    );
    compareAddress(
      errors,
      chain.chainKey,
      "lzExecutor",
      deployment.lzExecutor?.address,
      chain.lzExecutor,
    );
    compareAddress(
      errors,
      chain.chainKey,
      "Dead DVN",
      deployment.deadDVN?.address,
      chain.deadDVN,
    );

    const pushDVN = findLayerZeroLabsDVN(input.dvns, chain, false);
    compareAddress(
      errors,
      chain.chainKey,
      "LayerZero Labs DVN",
      pushDVN?.dvnAddress,
      chain.layerZeroLabsDVN,
    );
    const readDVN = findLayerZeroLabsDVN(input.dvns, chain, true);
    compareAddress(
      errors,
      chain.chainKey,
      "LayerZero Labs lzRead DVN",
      readDVN?.dvnAddress,
      chain.layerZeroLabsReadDVN,
    );
  }

  return errors;
}

function findLayerZeroLabsDVN(
  records: readonly DVNDeploymentRecord[],
  chain: ExpectedLayerZeroChain,
  lzReadCompatible: boolean,
): DVNDeploymentRecord | undefined {
  return records.find(
    (record) =>
      record.chainKey?.toLowerCase() === chain.chainKey &&
      String(record.eid) === chain.eid &&
      record.version === 2 &&
      record.id === "layerzero-labs" &&
      Boolean(record.lzReadCompatible) === lzReadCompatible,
  );
}

function compareAddress(
  errors: string[],
  chainKey: string,
  label: string,
  actual: string | undefined,
  expected: Address,
) {
  if (actual === undefined || actual === "") {
    errors.push(`${chainKey}: ${label} is missing`);
    return;
  }
  if (!isAddressEqual(getAddress(actual), expected)) {
    errors.push(`${chainKey}: ${label} ${getAddress(actual)} != ${expected}`);
  }
}
