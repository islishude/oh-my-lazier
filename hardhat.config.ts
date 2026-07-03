import hardhatToolboxViemPlugin from "@nomicfoundation/hardhat-toolbox-viem";
import { configVariable, defineConfig } from "hardhat/config";

const config = defineConfig({
  plugins: [hardhatToolboxViemPlugin],
  solidity: {
    version: "0.8.35",
    settings: {
      optimizer: {
        enabled: true,
        runs: 200,
      },
      evmVersion: "prague",
    },
  },
  paths: {
    sources: "contracts/contracts",
    tests: "contracts/test",
    cache: "contracts/cache",
    artifacts: "contracts/artifacts",
  },
  networks: {
    sepolia: {
      type: "http",
      chainType: "l1",
      chainId: 11155111,
      url: configVariable("SEPOLIA_RPC_URL"),
      accounts: [configVariable("PRIVATE_KEY")],
    },
    hoodi: {
      type: "http",
      chainType: "l1",
      chainId: 560048,
      url: configVariable("HOODI_RPC_URL"),
      accounts: [configVariable("PRIVATE_KEY")],
    },
  },
});

export default config;
