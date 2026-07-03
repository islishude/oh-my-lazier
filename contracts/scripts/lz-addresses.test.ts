import test from "node:test";
import assert from "node:assert/strict";
import {
  expectedLayerZeroChains,
  verifyLayerZeroAddresses,
  type DVNDeploymentRecord,
  type DeploymentRecord,
} from "./lz-addresses.js";

test("verifyLayerZeroAddresses accepts matching metadata", () => {
  const { deployments, dvns } = matchingFixtures();
  assert.deepEqual(verifyLayerZeroAddresses({ deployments, dvns }), []);
});

test("verifyLayerZeroAddresses reports mismatched protocol and DVN addresses", () => {
  const { deployments, dvns } = matchingFixtures();
  deployments[0] = {
    ...deployments[0],
    sendUln302: { address: "0x1111111111111111111111111111111111111111" },
  };
  dvns[1] = {
    ...dvns[1],
    dvnAddress: "0x2222222222222222222222222222222222222222",
  };

  const errors = verifyLayerZeroAddresses({ deployments, dvns });

  assert.equal(errors.length, 2);
  assert.match(errors[0], /SendUln302/);
  assert.match(errors[1], /LayerZero Labs lzRead DVN/);
});

function matchingFixtures(): {
  deployments: DeploymentRecord[];
  dvns: DVNDeploymentRecord[];
} {
  return {
    deployments: expectedLayerZeroChains.map((chain) => ({
      chainKey: chain.chainKey,
      nativeChainId: chain.nativeChainId,
      eid: chain.eid,
      endpointV2: { address: chain.endpointV2 },
      sendUln302: { address: chain.sendUln302 },
      receiveUln302: { address: chain.receiveUln302 },
      executor: { address: chain.executor },
      lzExecutor: { address: chain.lzExecutor },
      deadDVN: { address: chain.deadDVN },
    })),
    dvns: expectedLayerZeroChains.flatMap((chain) => {
      const records: DVNDeploymentRecord[] = [
        {
          chainKey: chain.chainKey,
          nativeChainId: chain.nativeChainId,
          eid: chain.eid,
          version: 2,
          id: "layerzero-labs",
          dvnAddress: chain.layerZeroLabsDVN,
        },
      ];
      if (chain.layerZeroLabsReadDVN !== undefined) {
        records.push({
          chainKey: chain.chainKey,
          nativeChainId: chain.nativeChainId,
          eid: chain.eid,
          version: 2,
          id: "layerzero-labs",
          lzReadCompatible: true,
          dvnAddress: chain.layerZeroLabsReadDVN,
        });
      }
      return records;
    }),
  };
}
