import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";

const OpenWorkersModule = buildModule("OpenWorkers", (m) => {
  const deployer = m.getAccount(0);
  const owner = m.getParameter("owner", deployer);
  const priceFeedSubmitters = m.getParameter<string[]>("priceFeedSubmitters");

  const priceFeed = m.contract("OpenPriceFeed", [owner, priceFeedSubmitters]);
  const openExecutor = m.contract("OpenExecutor", [owner, priceFeed]);
  const openDVN = m.contract("OpenDVN", [owner, priceFeed]);

  return { priceFeed, openExecutor, openDVN };
});

export default OpenWorkersModule;
