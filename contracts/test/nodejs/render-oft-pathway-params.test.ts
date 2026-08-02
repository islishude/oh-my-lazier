import assert from "node:assert/strict";
import test from "node:test";
import type { Address } from "viem";
import { parseRenderOftPathwayParamsInput } from "../../scripts/commands/render-oft-pathway-params-input.js";
import { CONFIG_TYPE_ULN, decodeUlnConfig } from "../../scripts/lz-config.js";
import { expectedLayerZeroChains } from "../../scripts/lz-addresses.js";
import {
  renderOftPathwayParams,
  type RenderOftPathwayParamsInput,
} from "../../scripts/render-oft-pathway-params.js";

test("renderOftPathwayParams appends an opted-in LayerZero Labs DVN", () => {
  const openDVN = address(3);
  const sepolia = expectedLayerZeroChains[0];
  const params = renderOftPathwayParams(
    renderInput({
      endpoint: sepolia.endpointV2,
      sendUln: sepolia.sendUln302,
      receiveUln: sepolia.receiveUln302,
      openDVN,
      includeLayerZeroLabsDVN: true,
    }),
    { now: 1_700_000_000n }
  );
  const ulnConfig = params.OAppEndpointConfig.sendConfig.find(
    (config) => config.configType === CONFIG_TYPE_ULN
  );
  assert.notEqual(ulnConfig, undefined);
  const decoded = decodeUlnConfig(ulnConfig!.config);

  assert.deepEqual(
    decoded.requiredDVNs.map((item) => item.toLowerCase()).sort(),
    [openDVN, sepolia.layerZeroLabsDVN!]
      .map((item) => item.toLowerCase())
      .sort()
  );
  assert.equal(
    params.OpenWorkersPathwayConfig.priceSnapshot.updatedAt,
    "1700000000"
  );
});

test("renderOftPathwayParams defaults minimum gas to enforced receive gas", () => {
  const input = renderInput({ requiredDVNs: [address(3), address(12)] });
  const params = renderOftPathwayParams(input, { now: 55n });
  const ulnConfig = params.OAppEndpointConfig.sendConfig.find(
    (config) => config.configType === CONFIG_TYPE_ULN
  );

  assert.deepEqual(
    decodeUlnConfig(ulnConfig!.config).requiredDVNs.map((item) =>
      item.toLowerCase()
    ),
    input.requiredDVNs!.map((item) => item.toLowerCase())
  );
  assert.equal(
    params.OpenWorkersPathwayConfig.workerPathwayConfig.minLzReceiveGas,
    input.enforcedLzReceiveGas.toString()
  );
});

test("parseRenderOftPathwayParamsInput accepts strict camelCase JSON fields", () => {
  const parsed = parseRenderOftPathwayParamsInput(
    jsonInput({ requiredDVNs: [address(3), address(12)] }),
    "input"
  );

  assert.equal(parsed.remoteEid, 40_149);
  assert.equal(parsed.maxMessageSize, 10_000);
  assert.equal(parsed.priceSnapshot.dstGasPriceInSrcToken, 1n);
  assert.deepEqual(parsed.requiredDVNs, [address(3), address(12)]);
});

test("parseRenderOftPathwayParamsInput rejects unknown nested fields", () => {
  const input = jsonInput() as Record<string, unknown>;
  input.priceSnapshot = {
    ...(input.priceSnapshot as Record<string, unknown>),
    stale_after: "1800",
  };

  assert.throws(
    () => parseRenderOftPathwayParamsInput(input, "input"),
    /input\.priceSnapshot contains unknown field: stale_after/
  );
});

function renderInput(
  overrides: Partial<RenderOftPathwayParamsInput> = {}
): RenderOftPathwayParamsInput {
  return {
    oapp: address(1),
    endpoint: address(2),
    remoteEid: 40_149,
    remoteOApp: address(4),
    sendUln: address(5),
    receiveUln: address(6),
    openExecutor: address(7),
    openDVN: address(3),
    priceFeed: address(8),
    bootstrapPriceSubmitter: address(9),
    confirmations: 12n,
    maxMessageSize: 10_000,
    maxLzReceiveGas: 1_000_000n,
    priceSnapshot: {
      dstGasPriceInSrcToken: 1n,
      dstDataFeePerByteInSrcToken: 0n,
      staleAfter: 1_800n,
    },
    executorFeeModel: {
      baseFee: 0n,
      dstGasOverhead: 50_000n,
      dataSizeOverheadBytes: 0n,
      marginBps: 1_000n,
    },
    dvnFeeModel: {
      baseFee: 0n,
      dstGasOverhead: 150_000n,
      dataSizeOverheadBytes: 0n,
      marginBps: 1_000n,
    },
    enforcedLzReceiveGas: 200_000n,
    ...overrides,
  };
}

function jsonInput(overrides: Record<string, unknown> = {}) {
  return {
    oapp: address(1),
    endpoint: address(2),
    remoteEid: "40149",
    remoteOApp: address(4),
    sendUln: address(5),
    receiveUln: address(6),
    openExecutor: address(7),
    openDVN: address(3),
    priceFeed: address(8),
    bootstrapPriceSubmitter: address(9),
    confirmations: "12",
    maxMessageSize: "10000",
    maxLzReceiveGas: "1000000",
    priceSnapshot: {
      dstGasPriceInSrcToken: "1",
      dstDataFeePerByteInSrcToken: "0",
      staleAfter: "1800",
    },
    executorFeeModel: {
      baseFee: "0",
      dstGasOverhead: "50000",
      dataSizeOverheadBytes: "0",
      marginBps: "1000",
    },
    dvnFeeModel: {
      baseFee: "0",
      dstGasOverhead: "150000",
      dataSizeOverheadBytes: "0",
      marginBps: "1000",
    },
    enforcedLzReceiveGas: "200000",
    ...overrides,
  };
}

function address(value: number): Address {
  return `0x${value.toString(16).padStart(40, "0")}` as Address;
}
