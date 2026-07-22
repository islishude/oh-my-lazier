import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";

const TestOFTModule = buildModule("TestOFT", (m) => {
  const deployer = m.getAccount(0);
  const tokenName = m.getParameter("tokenName", "Oh My Lazier Test OFT");
  const tokenSymbol = m.getParameter("tokenSymbol", "OMLTOFT");
  const endpoint = m.getParameter("endpoint");
  const delegate = m.getParameter("delegate", deployer);
  const initialRecipient = m.getParameter("initialRecipient", deployer);
  const initialSupply = m.getParameter("initialSupply", 0n);

  const testOFT = m.contract("TestOFT", [
    tokenName,
    tokenSymbol,
    endpoint,
    delegate,
    initialRecipient,
    initialSupply,
  ]);

  return { testOFT };
});

export default TestOFTModule;
