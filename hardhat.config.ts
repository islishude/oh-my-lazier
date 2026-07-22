import hardhatToolboxViemPlugin from "@nomicfoundation/hardhat-toolbox-viem";
import { configVariable, defineConfig } from "hardhat/config";

const config = defineConfig({
  plugins: [hardhatToolboxViemPlugin],
  solidity: {
    profiles: {
      default: {
        version: "0.8.35",
        settings: {
          optimizer: {
            enabled: true,
            runs: 200,
          },
          evmVersion: "osaka",
          viaIR: true,
        },
      },
      production: {
        version: "0.8.35",
        settings: {
          optimizer: {
            enabled: true,
            runs: 1000,
          },
          evmVersion: "osaka",
          metadata: {
            bytecodeHash: "none",
            useLiteralContent: true,
          },
          viaIR: true,
        },
      },
    },
  },
  paths: {
    sources: "contracts/contracts",
    tests: {
      solidity: "contracts/test",
      nodejs: "contracts/test/nodejs",
    },
    cache: "contracts/cache",
    artifacts: "contracts/artifacts",
    ignition: process.env.OML_IGNITION_DIR ?? "ignition",
  },
  networks: {
    hardhat: {
      type: "edr-simulated",
      chainType: "l1",
    },
    sepolia: {
      type: "http",
      chainType: "l1",
      chainId: 11155111,
      url: configVariable("SEPOLIA_RPC_URL"),
      accounts: [configVariable("SEPOLIA_PRIVATE_KEY")],
    },
    hoodi: {
      type: "http",
      chainType: "l1",
      chainId: 560048,
      url: configVariable("HOODI_RPC_URL"),
      accounts: [configVariable("HOODI_PRIVATE_KEY")],
    },
    "local-anvil-a": {
      type: "http",
      chainType: "l1",
      chainId: 31337,
      url: configVariable("E2E_CHAIN_A_HOST_RPC_URL"),
      accounts: [configVariable("E2E_DEPLOYER_PRIVATE_KEY")],
    },
    "local-anvil-b": {
      type: "http",
      chainType: "l1",
      chainId: 31338,
      url: configVariable("E2E_CHAIN_B_HOST_RPC_URL"),
      accounts: [configVariable("E2E_DEPLOYER_PRIVATE_KEY")],
    },
  },
  ignition: {
    requiredConfirmations: 1,
  },
  verify: {
    etherscan: {
      apiKey: configVariable("ETHERSCAN_API_KEY"),
    },
    blockscout: {
      enabled: false,
    },
    sourcify: {
      enabled: false,
    },
  },
});

export default config;
