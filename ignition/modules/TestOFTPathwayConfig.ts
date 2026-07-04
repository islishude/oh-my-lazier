import { createRequire } from "node:module";
import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";
import type { Artifact } from "@nomicfoundation/ignition-core";

const require = createRequire(import.meta.url);
const endpointArtifact = require(
  "../../node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
) as Artifact;

const TestOFTPathwayConfigModule = buildModule(
  "TestOFTPathwayConfig",
  (m) => {
    const defaultSender = m.getAccount(0);
    const testOFTAddress = m.getParameter<string>("testOFT");
    const endpointAddress = m.getParameter<string>("endpoint");
    const delegate = m.getParameter("delegate", defaultSender);
    const remoteEid = m.getParameter<number>("remoteEid");
    const remotePeer = m.getParameter<string>("remotePeer");
    const sendUln = m.getParameter<string>("sendUln");
    const receiveUln = m.getParameter<string>("receiveUln");
    const openExecutorAddress = m.getParameter<string>("openExecutor");
    const openDVNAddress = m.getParameter<string>("openDVN");
    const priceFeedAddress = m.getParameter<string>("priceFeed");
    const dvnVerifier = m.getParameter("dvnVerifier", defaultSender);
    const workerPathwayConfig = m.getParameter("workerPathwayConfig");
    const priceSnapshot = m.getParameter("priceSnapshot");
    const executorFeeModel = m.getParameter("executorFeeModel");
    const dvnFeeModel = m.getParameter("dvnFeeModel");
    const receiveLibraryGracePeriod = m.getParameter(
      "receiveLibraryGracePeriod",
      0,
    );
    const sendConfig = m.getParameter("sendConfig");
    const receiveConfig = m.getParameter("receiveConfig");
    const enforcedOptions = m.getParameter("enforcedOptions");

    const testOFT = m.contractAt("TestOFT", testOFTAddress);
    const openExecutor = m.contractAt("OpenExecutor", openExecutorAddress);
    const openDVN = m.contractAt("OpenDVN", openDVNAddress);
    const priceFeed = m.contractAt("OpenPriceFeed", priceFeedAddress);
    const endpoint = m.contractAt(
      "ILayerZeroEndpointV2",
      endpointArtifact,
      endpointAddress,
    );

    const setDelegate = m.call(testOFT, "setDelegate", [delegate], {
      id: "SetOFTDelegate",
    });
    const setPeer = m.call(testOFT, "setPeer", [remoteEid, remotePeer], {
      id: "SetOFTPeer",
      after: [setDelegate],
    });
    const setSendLibrary = m.call(
      endpoint,
      "setSendLibrary",
      [testOFT, remoteEid, sendUln],
      { id: "SetEndpointSendLibrary", after: [setPeer] },
    );
    const setReceiveLibrary = m.call(
      endpoint,
      "setReceiveLibrary",
      [testOFT, remoteEid, receiveUln, receiveLibraryGracePeriod],
      { id: "SetEndpointReceiveLibrary", after: [setSendLibrary] },
    );
    const setSendConfig = m.call(
      endpoint,
      "setConfig",
      [testOFT, sendUln, sendConfig],
      { id: "SetEndpointSendConfig", after: [setReceiveLibrary] },
    );
    const setReceiveConfig = m.call(
      endpoint,
      "setConfig",
      [testOFT, receiveUln, receiveConfig],
      { id: "SetEndpointReceiveConfig", after: [setSendConfig] },
    );
    const setEnforcedOptions = m.call(
      testOFT,
      "setEnforcedOptions",
      [enforcedOptions],
      {
        id: "SetOFTEnforcedOptions",
        after: [setReceiveConfig],
      },
    );
    const setOpenExecutorAllowedSendLib = m.call(
      openExecutor,
      "setAllowedSendLib",
      [sendUln, true],
      { id: "SetOpenExecutorAllowedSendLib", after: [setEnforcedOptions] },
    );
    const setOpenExecutorPathwayConfig = m.call(
      openExecutor,
      "setPathwayConfig",
      [remoteEid, testOFT, workerPathwayConfig],
      {
        id: "SetOpenExecutorPathwayConfig",
        after: [setOpenExecutorAllowedSendLib],
      },
    );
    const setPriceSnapshot = m.call(
      priceFeed,
      "setPriceSnapshot",
      [remoteEid, priceSnapshot],
      {
        id: "SetPriceFeedSnapshot",
        after: [setOpenExecutorPathwayConfig],
      },
    );
    const setOpenExecutorFeeModel = m.call(
      openExecutor,
      "setFeeModel",
      [remoteEid, executorFeeModel],
      {
        id: "SetOpenExecutorFeeModel",
        after: [setPriceSnapshot],
      },
    );
    const setOpenDVNAllowedSendLib = m.call(
      openDVN,
      "setAllowedSendLib",
      [sendUln, true],
      {
        id: "SetOpenDVNAllowedSendLib",
        after: [setOpenExecutorFeeModel],
      },
    );
    const setOpenDVNPathwayConfig = m.call(
      openDVN,
      "setPathwayConfig",
      [remoteEid, testOFT, workerPathwayConfig],
      { id: "SetOpenDVNPathwayConfig", after: [setOpenDVNAllowedSendLib] },
    );
    const setOpenDVNFeeModel = m.call(
      openDVN,
      "setFeeModel",
      [remoteEid, dvnFeeModel],
      { id: "SetOpenDVNFeeModel", after: [setOpenDVNPathwayConfig] },
    );
    m.call(openDVN, "setVerifier", [dvnVerifier, true], {
      id: "SetOpenDVNVerifier",
      after: [setOpenDVNFeeModel],
    });

    return { testOFT, endpoint, openExecutor, openDVN, priceFeed };
  },
);

export default TestOFTPathwayConfigModule;
