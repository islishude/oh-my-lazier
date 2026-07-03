import { ChainKey, EndpointId } from "@layerzerolabs/lz-definitions";
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
  layerZeroLabsReadDVN?: Address;
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
    chainKey: ChainKey.SEPOLIA,
    nativeChainId: 11155111,
    eid: String(EndpointId.SEPOLIA_V2_TESTNET),
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
    chainKey: ChainKey.HOODI_TESTNET,
    nativeChainId: 560048,
    eid: String(EndpointId.HOODI_V2_TESTNET),
    endpointV2: getAddress("0x3aCAAf60502791D199a5a5F0B173D78229eBFe32"),
    sendUln302: getAddress("0x45841dd1ca50265Da7614fC43A361e526c0e6160"),
    receiveUln302: getAddress("0xd682ECF100f6F4284138AA925348633B0611Ae21"),
    executor: getAddress("0x701f3927871EfcEa1235dB722f9E608aE120d243"),
    lzExecutor: getAddress("0x4Cf1B3Fa61465c2c907f82fC488B43223BA0CF93"),
    deadDVN: getAddress("0x88B27057A9e00c5F05DDa29241027afF63f9e6e0"),
    layerZeroLabsDVN: getAddress("0xa78a78a13074ed93ad447a26ec57121f29e8fec2"),
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
    if (chain.layerZeroLabsReadDVN !== undefined) {
      const readDVN = findLayerZeroLabsDVN(input.dvns, chain, true);
      compareAddress(
        errors,
        chain.chainKey,
        "LayerZero Labs lzRead DVN",
        readDVN?.dvnAddress,
        chain.layerZeroLabsReadDVN,
      );
    }
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
