import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";

const OpenDVNWorkerModule = buildModule("OpenDVNWorker", (m) => {
  const deployer = m.getAccount(0);
  const owner = m.getParameter("owner", deployer);
  const priceFeedSubmitters = m.getParameter<string[]>("priceFeedSubmitters");

  const priceFeed = m.contract("OpenPriceFeed", [owner, priceFeedSubmitters]);
  const openDVN = m.contract("OpenDVN", [owner, priceFeed]);

  return { priceFeed, openDVN };
});

export default OpenDVNWorkerModule;
