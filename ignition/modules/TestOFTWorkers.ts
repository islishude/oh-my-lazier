import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";

const TestOFTWorkersModule = buildModule("TestOFTWorkers", (m) => {
  const deployer = m.getAccount(0);
  const tokenName = m.getParameter("tokenName", "Oh My Lazier Test OFT");
  const tokenSymbol = m.getParameter("tokenSymbol", "OMLTOFT");
  const endpoint = m.getParameter("endpoint");
  const owner = m.getParameter("owner", deployer);
  const initialRecipient = m.getParameter("initialRecipient", deployer);
  const initialSupply = m.getParameter("initialSupply", 0n);

  const testOFT = m.contract("TestOFT", [
    tokenName,
    tokenSymbol,
    endpoint,
    owner,
    initialRecipient,
    initialSupply,
  ]);
  const priceFeed = m.contract("OpenPriceFeed", [owner]);
  const openExecutor = m.contract("OpenExecutor", [owner, priceFeed]);
  const openDVN = m.contract("OpenDVN", [owner, priceFeed]);

  return { testOFT, priceFeed, openExecutor, openDVN };
});

export default TestOFTWorkersModule;
