import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";

const OpenWorkersPathwayConfigModule = buildModule(
  "OpenWorkersPathwayConfig",
  (m) => {
    const defaultSender = m.getAccount(0);
    const oapp = m.getParameter<string>("oapp");
    const remoteEid = m.getParameter<number>("remoteEid");
    const sendUln = m.getParameter<string>("sendUln");
    const openExecutorAddress = m.getParameter<string>("openExecutor");
    const openDVNAddress = m.getParameter<string>("openDVN");
    const priceFeedAddress = m.getParameter<string>("priceFeed");
    const dvnVerifier = m.getParameter("dvnVerifier", defaultSender);
    const workerPathwayConfig = m.getParameter("workerPathwayConfig");
    const priceSnapshot = m.getParameter("priceSnapshot");
    const executorFeeModel = m.getParameter("executorFeeModel");
    const dvnFeeModel = m.getParameter("dvnFeeModel");

    const openExecutor = m.contractAt("OpenExecutor", openExecutorAddress);
    const openDVN = m.contractAt("OpenDVN", openDVNAddress);
    const priceFeed = m.contractAt("OpenPriceFeed", priceFeedAddress);

    const setOpenExecutorAllowedSendLib = m.call(
      openExecutor,
      "setAllowedSendLib",
      [sendUln, true],
      { id: "SetOpenExecutorAllowedSendLib" },
    );
    const setOpenExecutorPathwayConfig = m.call(
      openExecutor,
      "setPathwayConfig",
      [remoteEid, oapp, workerPathwayConfig],
      {
        id: "SetOpenExecutorPathwayConfig",
        after: [setOpenExecutorAllowedSendLib],
      },
    );
    const setPriceSnapshot = m.call(
      priceFeed,
      "setPriceSnapshot",
      [[{ dstEid: remoteEid, snapshot: priceSnapshot }]],
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
      [remoteEid, oapp, workerPathwayConfig],
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

    return { openExecutor, openDVN, priceFeed };
  },
);

export default OpenWorkersPathwayConfigModule;
