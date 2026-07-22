import assert from "node:assert/strict";
import test from "node:test";
import type { Future, IgnitionModule } from "@nomicfoundation/ignition-core";
import LocalE2EChainModule from "../../../ignition/modules/LocalE2EChain.js";
import LocalE2EPathwayModule from "../../../ignition/modules/LocalE2EPathway.js";

test("LocalE2EChain exposes the complete topology with stable DVN Future IDs", () => {
  assert.equal(LocalE2EChainModule.id, "LocalE2EChain");
  assert.deepEqual(resultIDs(LocalE2EChainModule), {
    endpoint: "LocalE2EChain#EndpointV2",
    sendUln: "LocalE2EChain#SendUln302",
    receiveUln: "LocalE2EChain#ReceiveUln302",
    oft: "LocalE2EChain#TestOFT",
    priceFeed: "LocalE2EChain#OpenPriceFeed",
    openExecutor: "LocalE2EChain#OpenExecutor",
    primaryOpenDVN: "LocalE2EChain#PrimaryOpenDVN",
    secondaryOpenDVN: "LocalE2EChain#SecondaryOpenDVN",
  });
  assert.deepEqual(futureIDs(LocalE2EChainModule), [
    "LocalE2EChain#EndpointV2",
    "LocalE2EChain#SendUln302",
    "LocalE2EChain#ReceiveUln302",
    "LocalE2EChain#TestOFT",
    "LocalE2EChain#OpenPriceFeed",
    "LocalE2EChain#OpenExecutor",
    "LocalE2EChain#PrimaryOpenDVN",
    "LocalE2EChain#SecondaryOpenDVN",
  ]);

  const dvnDeployments = [...LocalE2EChainModule.futures].filter(
    (future) =>
      future.id === "LocalE2EChain#PrimaryOpenDVN" ||
      future.id === "LocalE2EChain#SecondaryOpenDVN"
  );
  assert.equal(dvnDeployments.length, 2);
  assert.ok(
    dvnDeployments.every(
      (future) => "contractName" in future && future.contractName === "OpenDVN"
    )
  );
});

test("LocalE2EPathway retains every source and verifier configuration step", () => {
  assert.equal(LocalE2EPathwayModule.id, "LocalE2EPathway");
  assert.deepEqual(resultIDs(LocalE2EPathwayModule), {
    endpoint: "LocalE2EPathway#EndpointV2",
    sendUln: "LocalE2EPathway#SendUln302",
    receiveUln: "LocalE2EPathway#ReceiveUln302",
    oft: "LocalE2EPathway#TestOFT",
    priceFeed: "LocalE2EPathway#OpenPriceFeed",
    openExecutor: "LocalE2EPathway#OpenExecutor",
    primaryOpenDVN: "LocalE2EPathway#PrimaryOpenDVN",
    secondaryOpenDVN: "LocalE2EPathway#SecondaryOpenDVN",
  });

  const calls = [...LocalE2EPathwayModule.futures].filter(
    (future): future is Future & { functionName: string } =>
      "functionName" in future
  );
  assert.deepEqual(
    calls.map((future) => [future.id, future.functionName]),
    [
      ["LocalE2EPathway#RegisterSendUln302", "registerLibrary"],
      ["LocalE2EPathway#RegisterReceiveUln302", "registerLibrary"],
      ["LocalE2EPathway#SetDefaultSendUlnConfig", "setDefaultUlnConfigs"],
      ["LocalE2EPathway#SetDefaultReceiveUlnConfig", "setDefaultUlnConfigs"],
      ["LocalE2EPathway#SetDefaultExecutorConfig", "setDefaultExecutorConfigs"],
      ["LocalE2EPathway#SetDefaultSendLibrary", "setDefaultSendLibrary"],
      ["LocalE2EPathway#SetDefaultReceiveLibrary", "setDefaultReceiveLibrary"],
      ["LocalE2EPathway#SetOFTPeer", "setPeer"],
      ["LocalE2EPathway#SetEndpointSendConfig", "setConfig"],
      ["LocalE2EPathway#SetEndpointReceiveConfig", "setConfig"],
      ["LocalE2EPathway#SetOFTEnforcedOptions", "setEnforcedOptions"],
      ["LocalE2EPathway#SetPriceFeedSnapshot", "setPriceSnapshot"],
      ["LocalE2EPathway#SetOpenExecutorAllowedSendLib", "setAllowedSendLib"],
      ["LocalE2EPathway#SetOpenExecutorPathwayConfig", "setPathwayConfig"],
      ["LocalE2EPathway#SetOpenExecutorFeeModel", "setFeeModel"],
      ["LocalE2EPathway#SetPrimaryOpenDVNAllowedSendLib", "setAllowedSendLib"],
      ["LocalE2EPathway#SetPrimaryOpenDVNPathwayConfig", "setPathwayConfig"],
      ["LocalE2EPathway#SetPrimaryOpenDVNFeeModel", "setFeeModel"],
      [
        "LocalE2EPathway#SetSecondaryOpenDVNAllowedSendLib",
        "setAllowedSendLib",
      ],
      ["LocalE2EPathway#SetSecondaryOpenDVNPathwayConfig", "setPathwayConfig"],
      ["LocalE2EPathway#SetSecondaryOpenDVNFeeModel", "setFeeModel"],
      ["LocalE2EPathway#SetPrimaryOpenDVNVerifier", "setVerifier"],
      ["LocalE2EPathway#SetSecondaryOpenDVNVerifier", "setVerifier"],
    ]
  );

  for (let index = 1; index < calls.length; index++) {
    assert.ok(
      calls[index].dependencies.has(calls[index - 1]),
      `${calls[index].id} must run after ${calls[index - 1].id}`
    );
  }
});

function futureIDs(module: IgnitionModule): string[] {
  return [...module.futures].map((future) => future.id);
}

function resultIDs(module: IgnitionModule): Record<string, string> {
  return Object.fromEntries(
    Object.entries(module.results).map(([name, future]) => [name, future.id])
  );
}
