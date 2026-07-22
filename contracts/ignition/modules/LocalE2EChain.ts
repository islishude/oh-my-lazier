import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";
import type { Artifact } from "@nomicfoundation/ignition-core";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const endpointArtifact =
  require("../../../node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/EndpointV2.sol/EndpointV2.json") as Artifact;
const sendUlnArtifact =
  require("../../../node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json") as Artifact;
const receiveUlnArtifact =
  require("../../../node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json") as Artifact;

/**
 * Deploys all contracts owned by one side of the local two-chain E2E topology.
 *
 * The two OpenDVN deployments use explicit Future IDs because they share the
 * same contract artifact. Keep those IDs stable so a local Ignition journal can
 * be resumed safely within its isolated E2E deployment directory.
 */
const LocalE2EChainModule = buildModule("LocalE2EChain", (m) => {
  const deployer = m.getAccount(0);
  const eid = m.getParameter<number>("eid");
  const owner = m.getParameter("owner", deployer);
  const tokenName = m.getParameter<string>("tokenName");
  const tokenSymbol = m.getParameter<string>("tokenSymbol");
  const delegate = m.getParameter("delegate", deployer);
  const initialRecipient = m.getParameter("initialRecipient", deployer);
  const initialSupply = m.getParameter<bigint>("initialSupply");
  const priceFeedSubmitters = m.getParameter<string[]>("priceFeedSubmitters");

  const endpoint = m.contract("EndpointV2", endpointArtifact, [eid, owner], {
    id: "EndpointV2",
  });
  const sendUln = m.contract(
    "SendUln302",
    sendUlnArtifact,
    [endpoint, 0n, 0n],
    { id: "SendUln302" }
  );
  const receiveUln = m.contract(
    "ReceiveUln302",
    receiveUlnArtifact,
    [endpoint],
    { id: "ReceiveUln302" }
  );
  const oft = m.contract("TestOFT", [
    tokenName,
    tokenSymbol,
    endpoint,
    delegate,
    initialRecipient,
    initialSupply,
  ]);
  const priceFeed = m.contract("OpenPriceFeed", [owner, priceFeedSubmitters]);
  const openExecutor = m.contract("OpenExecutor", [owner, priceFeed]);
  const primaryOpenDVN = m.contract("OpenDVN", [owner, priceFeed], {
    id: "PrimaryOpenDVN",
  });
  const secondaryOpenDVN = m.contract("OpenDVN", [owner, priceFeed], {
    id: "SecondaryOpenDVN",
  });

  return {
    endpoint,
    sendUln,
    receiveUln,
    oft,
    priceFeed,
    openExecutor,
    primaryOpenDVN,
    secondaryOpenDVN,
  };
});

export default LocalE2EChainModule;
