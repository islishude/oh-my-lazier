import { defineConfig } from "hardhat/config";
import baseConfig from "./hardhat.config.js";

/**
 * Hardhat configuration used exclusively by isolated source-verification
 * subprocesses. Verification never needs a local signer, so every HTTP
 * network deliberately overrides the deployment config with remote accounts.
 */
export const readOnlyVerificationConfig = defineConfig({
  ...baseConfig,
  networks: Object.fromEntries(
    Object.entries(baseConfig.networks ?? {}).map(([name, network]) => [
      name,
      network.type === "http"
        ? { ...network, accounts: "remote" as const }
        : network,
    ])
  ),
});

export default readOnlyVerificationConfig;
