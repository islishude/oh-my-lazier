import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";

const OpenDVNPathwayConfigModule = buildModule("OpenDVNPathwayConfig", (m) => {
  const defaultSender = m.getAccount(0);
  const oapp = m.getParameter<string>("oapp");
  const remoteEid = m.getParameter<number>("remoteEid");
  const sendUln = m.getParameter<string>("sendUln");
  const openDVNAddress = m.getParameter<string>("openDVN");
  const priceFeedAddress = m.getParameter<string>("priceFeed");
  const bootstrapPriceSubmitter = m.getParameter<string>(
    "bootstrapPriceSubmitter",
  );
  const dvnVerifier = m.getParameter("dvnVerifier", defaultSender);
  const workerPathwayConfig = m.getParameter("workerPathwayConfig");
  const priceSnapshot = m.getParameter("priceSnapshot");
  const dvnFeeModel = m.getParameter("dvnFeeModel");

  const openDVN = m.contractAt("OpenDVN", openDVNAddress);
  const priceFeed = m.contractAt("OpenPriceFeed", priceFeedAddress);

  const setOpenDVNPriceFeed = m.call(openDVN, "setPriceFeed", [priceFeedAddress], {
    id: "SetOpenDVNPriceFeed",
  });
  const setOpenDVNAllowedSendLib = m.call(
    openDVN,
    "setAllowedSendLib",
    [sendUln, true],
    {
      id: "SetOpenDVNAllowedSendLib",
      after: [setOpenDVNPriceFeed],
    },
  );
  const setOpenDVNPathwayConfig = m.call(
    openDVN,
    "setPathwayConfig",
    [remoteEid, oapp, workerPathwayConfig],
    { id: "SetOpenDVNPathwayConfig", after: [setOpenDVNAllowedSendLib] },
  );
  const setPriceSnapshot = m.call(
    priceFeed,
    "setPriceSnapshot",
    [[{ dstEid: remoteEid, snapshot: priceSnapshot }]],
    {
      id: "SetPriceFeedSnapshot",
      after: [setOpenDVNPathwayConfig],
    },
  );
  const revokeBootstrapPriceSubmitter = m.call(
    priceFeed,
    "setSubmitter",
    [bootstrapPriceSubmitter, false],
    {
      id: "RevokeBootstrapPriceSubmitter",
      after: [setPriceSnapshot],
    },
  );
  const setOpenDVNFeeModel = m.call(
    openDVN,
    "setFeeModel",
    [remoteEid, dvnFeeModel],
    { id: "SetOpenDVNFeeModel", after: [revokeBootstrapPriceSubmitter] },
  );
  m.call(openDVN, "setVerifier", [dvnVerifier, true], {
    id: "SetOpenDVNVerifier",
    after: [setOpenDVNFeeModel],
  });

  return { openDVN, priceFeed };
});

export default OpenDVNPathwayConfigModule;
