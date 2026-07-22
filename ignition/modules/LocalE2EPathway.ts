import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";
import type { Artifact } from "@nomicfoundation/ignition-core";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const endpointArtifact =
  require("../../node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/EndpointV2.sol/EndpointV2.json") as Artifact;
const sendUlnArtifact =
  require("../../node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json") as Artifact;
const receiveUlnArtifact =
  require("../../node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json") as Artifact;

/**
 * Configures one local chain as the source of an E2E pathway.
 *
 * The verifier calls also prepare this chain to receive messages from the
 * opposite direction. Deploy this module once on each local chain after both
 * LocalE2EChain deployments are available.
 */
const LocalE2EPathwayModule = buildModule("LocalE2EPathway", (m) => {
  const defaultSender = m.getAccount(0);
  const endpointAddress = m.getParameter<string>("endpoint");
  const sendUlnAddress = m.getParameter<string>("sendUln");
  const receiveUlnAddress = m.getParameter<string>("receiveUln");
  const oftAddress = m.getParameter<string>("oft");
  const priceFeedAddress = m.getParameter<string>("priceFeed");
  const openExecutorAddress = m.getParameter<string>("openExecutor");
  const primaryOpenDVNAddress = m.getParameter<string>("primaryOpenDVN");
  const secondaryOpenDVNAddress = m.getParameter<string>("secondaryOpenDVN");
  const remoteEid = m.getParameter<number>("remoteEid");
  const remotePeer = m.getParameter<string>("remotePeer");
  const receiveLibraryGracePeriod = m.getParameter<bigint>(
    "receiveLibraryGracePeriod",
    0n
  );
  const defaultUlnConfig = m.getParameter("defaultUlnConfig");
  const defaultExecutorConfig = m.getParameter("defaultExecutorConfig");
  const sendConfig = m.getParameter("sendConfig");
  const receiveConfig = m.getParameter("receiveConfig");
  const enforcedOptions = m.getParameter("enforcedOptions");
  const workerPathwayConfig = m.getParameter("workerPathwayConfig");
  const priceSnapshot = m.getParameter("priceSnapshot");
  const executorFeeModel = m.getParameter("executorFeeModel");
  const dvnFeeModel = m.getParameter("dvnFeeModel");
  const primaryDVNVerifier = m.getParameter<string>("primaryDVNVerifier");
  const secondaryDVNVerifier = m.getParameter(
    "secondaryDVNVerifier",
    defaultSender
  );

  const endpoint = m.contractAt(
    "EndpointV2",
    endpointArtifact,
    endpointAddress
  );
  const sendUln = m.contractAt("SendUln302", sendUlnArtifact, sendUlnAddress);
  const receiveUln = m.contractAt(
    "ReceiveUln302",
    receiveUlnArtifact,
    receiveUlnAddress
  );
  const oft = m.contractAt("TestOFT", oftAddress);
  const priceFeed = m.contractAt("OpenPriceFeed", priceFeedAddress);
  const openExecutor = m.contractAt("OpenExecutor", openExecutorAddress);
  const primaryOpenDVN = m.contractAt("OpenDVN", primaryOpenDVNAddress, {
    id: "PrimaryOpenDVN",
  });
  const secondaryOpenDVN = m.contractAt("OpenDVN", secondaryOpenDVNAddress, {
    id: "SecondaryOpenDVN",
  });

  const registerSendUln = m.call(endpoint, "registerLibrary", [sendUln], {
    id: "RegisterSendUln302",
  });
  const registerReceiveUln = m.call(endpoint, "registerLibrary", [receiveUln], {
    id: "RegisterReceiveUln302",
    after: [registerSendUln],
  });
  const setDefaultSendUlnConfig = m.call(
    sendUln,
    "setDefaultUlnConfigs",
    [[{ eid: remoteEid, config: defaultUlnConfig }]],
    { id: "SetDefaultSendUlnConfig", after: [registerReceiveUln] }
  );
  const setDefaultReceiveUlnConfig = m.call(
    receiveUln,
    "setDefaultUlnConfigs",
    [[{ eid: remoteEid, config: defaultUlnConfig }]],
    { id: "SetDefaultReceiveUlnConfig", after: [setDefaultSendUlnConfig] }
  );
  const setDefaultExecutorConfig = m.call(
    sendUln,
    "setDefaultExecutorConfigs",
    [[{ eid: remoteEid, config: defaultExecutorConfig }]],
    { id: "SetDefaultExecutorConfig", after: [setDefaultReceiveUlnConfig] }
  );
  const setDefaultSendLibrary = m.call(
    endpoint,
    "setDefaultSendLibrary",
    [remoteEid, sendUln],
    { id: "SetDefaultSendLibrary", after: [setDefaultExecutorConfig] }
  );
  const setDefaultReceiveLibrary = m.call(
    endpoint,
    "setDefaultReceiveLibrary",
    [remoteEid, receiveUln, receiveLibraryGracePeriod],
    { id: "SetDefaultReceiveLibrary", after: [setDefaultSendLibrary] }
  );
  const setPeer = m.call(oft, "setPeer", [remoteEid, remotePeer], {
    id: "SetOFTPeer",
    after: [setDefaultReceiveLibrary],
  });
  const setSendConfig = m.call(
    endpoint,
    "setConfig",
    [oft, sendUln, sendConfig],
    { id: "SetEndpointSendConfig", after: [setPeer] }
  );
  const setReceiveConfig = m.call(
    endpoint,
    "setConfig",
    [oft, receiveUln, receiveConfig],
    { id: "SetEndpointReceiveConfig", after: [setSendConfig] }
  );
  const setEnforcedOptions = m.call(
    oft,
    "setEnforcedOptions",
    [enforcedOptions],
    { id: "SetOFTEnforcedOptions", after: [setReceiveConfig] }
  );
  const setPriceSnapshot = m.call(
    priceFeed,
    "setPriceSnapshot",
    [[{ dstEid: remoteEid, snapshot: priceSnapshot }]],
    { id: "SetPriceFeedSnapshot", after: [setEnforcedOptions] }
  );

  const setExecutorAllowedSendLib = m.call(
    openExecutor,
    "setAllowedSendLib",
    [sendUln, true],
    { id: "SetOpenExecutorAllowedSendLib", after: [setPriceSnapshot] }
  );
  const setExecutorPathwayConfig = m.call(
    openExecutor,
    "setPathwayConfig",
    [remoteEid, oft, workerPathwayConfig],
    {
      id: "SetOpenExecutorPathwayConfig",
      after: [setExecutorAllowedSendLib],
    }
  );
  const setExecutorFeeModel = m.call(
    openExecutor,
    "setFeeModel",
    [remoteEid, executorFeeModel],
    { id: "SetOpenExecutorFeeModel", after: [setExecutorPathwayConfig] }
  );

  const setPrimaryDVNAllowedSendLib = m.call(
    primaryOpenDVN,
    "setAllowedSendLib",
    [sendUln, true],
    { id: "SetPrimaryOpenDVNAllowedSendLib", after: [setExecutorFeeModel] }
  );
  const setPrimaryDVNPathwayConfig = m.call(
    primaryOpenDVN,
    "setPathwayConfig",
    [remoteEid, oft, workerPathwayConfig],
    {
      id: "SetPrimaryOpenDVNPathwayConfig",
      after: [setPrimaryDVNAllowedSendLib],
    }
  );
  const setPrimaryDVNFeeModel = m.call(
    primaryOpenDVN,
    "setFeeModel",
    [remoteEid, dvnFeeModel],
    { id: "SetPrimaryOpenDVNFeeModel", after: [setPrimaryDVNPathwayConfig] }
  );

  const setSecondaryDVNAllowedSendLib = m.call(
    secondaryOpenDVN,
    "setAllowedSendLib",
    [sendUln, true],
    { id: "SetSecondaryOpenDVNAllowedSendLib", after: [setPrimaryDVNFeeModel] }
  );
  const setSecondaryDVNPathwayConfig = m.call(
    secondaryOpenDVN,
    "setPathwayConfig",
    [remoteEid, oft, workerPathwayConfig],
    {
      id: "SetSecondaryOpenDVNPathwayConfig",
      after: [setSecondaryDVNAllowedSendLib],
    }
  );
  const setSecondaryDVNFeeModel = m.call(
    secondaryOpenDVN,
    "setFeeModel",
    [remoteEid, dvnFeeModel],
    {
      id: "SetSecondaryOpenDVNFeeModel",
      after: [setSecondaryDVNPathwayConfig],
    }
  );
  const setPrimaryDVNVerifier = m.call(
    primaryOpenDVN,
    "setVerifier",
    [primaryDVNVerifier, true],
    { id: "SetPrimaryOpenDVNVerifier", after: [setSecondaryDVNFeeModel] }
  );
  m.call(secondaryOpenDVN, "setVerifier", [secondaryDVNVerifier, true], {
    id: "SetSecondaryOpenDVNVerifier",
    after: [setPrimaryDVNVerifier],
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

export default LocalE2EPathwayModule;
