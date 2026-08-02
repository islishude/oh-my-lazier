import test from "node:test";
import assert from "node:assert/strict";
import {
  expectedLayerZeroChains,
  layerZeroLabsDVNForLibraries,
  requireLayerZeroLabsDVNForLibraries,
  verifyLayerZeroAddresses,
  type DeploymentRecord,
} from "../../scripts/lz-addresses.js";

test("verifyLayerZeroAddresses accepts matching metadata", () => {
  const deployments = matchingFixtures();
  assert.deepEqual(verifyLayerZeroAddresses({ deployments }), []);
});

test("verifyLayerZeroAddresses reports mismatched protocol addresses", () => {
  const deployments = matchingFixtures();
  deployments[0] = {
    ...deployments[0],
    sendUln302: { address: "0x1111111111111111111111111111111111111111" },
  };

  const errors = verifyLayerZeroAddresses({ deployments });

  assert.equal(errors.length, 1);
  assert.match(errors[0], /SendUln302/);
});

test("layerZeroLabsDVNForLibraries resolves optional push DVN metadata", () => {
  assert.equal(
    layerZeroLabsDVNForLibraries({
      endpointV2: expectedLayerZeroChains[0].endpointV2,
      sendUln302: expectedLayerZeroChains[0].sendUln302,
      receiveUln302: expectedLayerZeroChains[0].receiveUln302,
    }),
    expectedLayerZeroChains[0].layerZeroLabsDVN
  );
  assert.equal(
    layerZeroLabsDVNForLibraries({
      endpointV2: expectedLayerZeroChains[1].endpointV2,
      sendUln302: expectedLayerZeroChains[1].sendUln302,
      receiveUln302: expectedLayerZeroChains[1].receiveUln302,
    }),
    expectedLayerZeroChains[1].layerZeroLabsDVN
  );
});

test("requireLayerZeroLabsDVNForLibraries rejects unknown local chain metadata", () => {
  assert.throws(
    () =>
      requireLayerZeroLabsDVNForLibraries(
        {
          endpointV2: "0x3333333333333333333333333333333333333333",
          sendUln302: "0x4444444444444444444444444444444444444444",
          receiveUln302: "0x5555555555555555555555555555555555555555",
        },
        "test.includeLayerZeroLabsDVN"
      ),
    /test\.includeLayerZeroLabsDVN has no repo-known LayerZero Labs DVN metadata/
  );
});

function matchingFixtures(): DeploymentRecord[] {
  return expectedLayerZeroChains.map((chain) => ({
    chainKey: chain.chainKey,
    nativeChainId: chain.nativeChainId,
    eid: chain.eid,
    endpointV2: { address: chain.endpointV2 },
    sendUln302: { address: chain.sendUln302 },
    receiveUln302: { address: chain.receiveUln302 },
    executor: { address: chain.executor },
    lzExecutor: { address: chain.lzExecutor },
    deadDVN: { address: chain.deadDVN },
  }));
}
