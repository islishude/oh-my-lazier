import hardhatToolboxViemPlugin from "@nomicfoundation/hardhat-toolbox-viem";
import type { HardhatUserConfig } from "hardhat/config";

const config: HardhatUserConfig = {
  plugins: [hardhatToolboxViemPlugin],
  solidity: {
    version: "0.8.35",
    settings: {
      optimizer: {
        enabled: true,
        runs: 200
      },
      evmVersion: "prague"
    }
  },
  paths: {
    sources: "contracts/contracts",
    tests: "contracts/test",
    cache: "contracts/cache",
    artifacts: "contracts/artifacts"
  }
};

export default config;
