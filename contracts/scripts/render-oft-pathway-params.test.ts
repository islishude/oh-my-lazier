import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";
import { join } from "node:path";
import test from "node:test";
import { type Address } from "viem";
import {
  CONFIG_TYPE_ULN,
  decodeUlnConfig,
  type SetConfigEntry,
} from "./lz-config.js";
import { expectedLayerZeroChains } from "./lz-addresses.js";

test("render-oft-pathway-params appends opt-in LayerZero Labs DVN", () => {
  const openDVN = "0x1111111111111111111111111111111111111113";
  const sepolia = expectedLayerZeroChains[0];
  const result = spawnSync(
    join(process.cwd(), "node_modules", ".bin", "tsx"),
    [
      "contracts/scripts/render-oft-pathway-params.ts",
      "--include-layerzero-labs-dvn",
    ],
    {
      cwd: process.cwd(),
      env: {
        ...childProcessEnv(),
        ...renderEnv({
          endpoint: sepolia.endpointV2,
          sendUln: sepolia.sendUln302,
          receiveUln: sepolia.receiveUln302,
          openDVN,
        }),
      },
      encoding: "utf8",
    },
  );

  assert.equal(result.status, 0, result.stderr);
  const params = JSON.parse(result.stdout) as {
    OAppEndpointConfig: { sendConfig: SetConfigEntry[] };
  };
  const ulnConfig = params.OAppEndpointConfig.sendConfig.find(
    (config) => config.configType === CONFIG_TYPE_ULN,
  );
  assert.notEqual(ulnConfig, undefined);
  const decoded = decodeUlnConfig(ulnConfig!.config);

  assert.deepEqual(
    decoded.requiredDVNs.map((address) => address.toLowerCase()).sort(),
    [openDVN, sepolia.layerZeroLabsDVN!]
      .map((address) => address.toLowerCase())
      .sort(),
  );
});

function childProcessEnv(): NodeJS.ProcessEnv {
  const { REQUIRED_DVNS: _requiredDVNs, ...env } = process.env;
  return env;
}

function renderEnv(input: {
  endpoint: Address;
  sendUln: Address;
  receiveUln: Address;
  openDVN: Address;
}): NodeJS.ProcessEnv {
  return {
    OAPP: "0x1111111111111111111111111111111111111111",
    ENDPOINT: input.endpoint,
    REMOTE_EID: "40449",
    REMOTE_OAPP: "0x2222222222222222222222222222222222222221",
    SEND_ULN: input.sendUln,
    RECEIVE_ULN: input.receiveUln,
    OPEN_EXECUTOR: "0x1111111111111111111111111111111111111112",
    OPEN_DVN: input.openDVN,
    PRICE_FEED: "0x1111111111111111111111111111111111111114",
    BOOTSTRAP_PRICE_SUBMITTER: "0x3333333333333333333333333333333333333333",
    CONFIRMATIONS: "12",
    EXECUTOR_MAX_MESSAGE_SIZE: "10000",
    ENFORCED_LZ_RECEIVE_GAS: "200000",
    MAX_LZ_RECEIVE_GAS: "1000000",
    PRICE_SNAPSHOT_DST_GAS_PRICE_IN_SRC_TOKEN: "1",
    PRICE_SNAPSHOT_DST_DATA_FEE_PER_BYTE_IN_SRC_TOKEN: "0",
    PRICE_SNAPSHOT_STALE_AFTER: "1800",
    EXECUTOR_FEE_FIXED_FEE_WEI: "0",
    EXECUTOR_FEE_DST_GAS_OVERHEAD: "50000",
    EXECUTOR_FEE_DATA_SIZE_OVERHEAD_BYTES: "0",
    EXECUTOR_FEE_MARGIN_BPS: "1000",
    DVN_FEE_FIXED_FEE_WEI: "0",
    DVN_FEE_DST_GAS_OVERHEAD: "150000",
    DVN_FEE_DATA_SIZE_OVERHEAD_BYTES: "0",
    DVN_FEE_MARGIN_BPS: "1000",
  };
}
