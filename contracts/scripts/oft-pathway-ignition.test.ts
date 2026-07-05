import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  decodeExecutorConfig,
  decodeUlnConfig,
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  NIL_DVN_COUNT,
} from "./lz-config.js";
import {
  buildOAppEndpointConfigParameters,
  buildOpenWorkersPathwayConfigParameters,
  OFT_MSG_TYPE_SEND,
} from "./oft-pathway-ignition.js";

function basePathwayInput() {
  return {
    oapp: "0x1111111111111111111111111111111111111111",
    endpoint: "0x2222222222222222222222222222222222222222",
    remoteEid: 40449,
    remoteOApp: "0x4444444444444444444444444444444444444444",
    sendUln: "0x5555555555555555555555555555555555555555",
    receiveUln: "0x6666666666666666666666666666666666666666",
    openExecutor: "0x7777777777777777777777777777777777777777",
    openDVN: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    priceFeed: "0xcccccccccccccccccccccccccccccccccccccccc",
    layerZeroLabsDVN: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    confirmations: 12n,
    maxMessageSize: 10_000,
    minLzReceiveGas: 200_000n,
    maxLzReceiveGas: 1_000_000n,
    priceSnapshot: {
      dstGasPriceInSrcToken: 2n,
      updatedAt: 1_700_000_000n,
      staleAfter: 1800n,
    },
    executorFeeModel: {
      baseFee: 1000n,
      dstGasOverhead: 50_000n,
      marginBps: 1000,
    },
    dvnFeeModel: {
      baseFee: 2000n,
      dstGasOverhead: 150_000n,
      marginBps: 500n,
    },
    enforcedLzReceiveGas: 200_000n,
  } as const;
}

test("buildOAppEndpointConfigParameters renders generic OApp endpoint config", () => {
  const rendered = buildOAppEndpointConfigParameters({
    ...basePathwayInput(),
    delegate: "0x3333333333333333333333333333333333333333",
  }).OAppEndpointConfig;

  assert.equal(rendered.oapp, "0x1111111111111111111111111111111111111111");
  assert.equal(rendered.endpoint, "0x2222222222222222222222222222222222222222");
  assert.equal(rendered.delegate, "0x3333333333333333333333333333333333333333");
  assert.equal(rendered.remoteEid, 40449);
  assert.equal(
    rendered.remotePeer,
    "0x0000000000000000000000004444444444444444444444444444444444444444",
  );
  assert.equal(rendered.sendUln, "0x5555555555555555555555555555555555555555");
  assert.equal(
    rendered.receiveUln,
    "0x6666666666666666666666666666666666666666",
  );
  assert.equal(rendered.receiveLibraryGracePeriod, 0);
  assert.deepEqual(rendered.enforcedOptions, [
    {
      eid: 40449,
      msgType: OFT_MSG_TYPE_SEND,
      options: "0x00030100110100000000000000000000000000030d40",
    },
  ]);

  assert.equal(rendered.sendConfig.length, 2);
  assert.equal(rendered.sendConfig[0].eid, 40449);
  assert.equal(rendered.sendConfig[0].configType, CONFIG_TYPE_EXECUTOR);
  assert.deepEqual(decodeExecutorConfig(rendered.sendConfig[0].config), {
    maxMessageSize: 10_000,
    executor: "0x7777777777777777777777777777777777777777",
  });
  assert.equal(rendered.sendConfig[1].eid, 40449);
  assert.equal(rendered.sendConfig[1].configType, CONFIG_TYPE_ULN);
  const decodedSendUlnConfig = decodeUlnConfig(rendered.sendConfig[1].config);
  assert.deepEqual(
    {
      ...decodedSendUlnConfig,
      requiredDVNs: decodedSendUlnConfig.requiredDVNs.map((address) =>
        address.toLowerCase(),
      ),
    },
    {
      confirmations: 12n,
      requiredDVNCount: 2,
      optionalDVNCount: NIL_DVN_COUNT,
      optionalDVNThreshold: 0,
      requiredDVNs: [
        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      ],
      optionalDVNs: [],
    },
  );

  assert.deepEqual(rendered.receiveConfig, [
    {
      eid: 40449,
      configType: CONFIG_TYPE_ULN,
      config: rendered.sendConfig[1].config,
    },
  ]);
});

test("buildOpenWorkersPathwayConfigParameters renders worker-only config", () => {
  const rendered = buildOpenWorkersPathwayConfigParameters({
    ...basePathwayInput(),
    dvnVerifier: "0x9999999999999999999999999999999999999999",
  }).OpenWorkersPathwayConfig;

  assert.equal(rendered.oapp, "0x1111111111111111111111111111111111111111");
  assert.equal(rendered.remoteEid, 40449);
  assert.equal(rendered.sendUln, "0x5555555555555555555555555555555555555555");
  assert.equal(rendered.openExecutor, "0x7777777777777777777777777777777777777777");
  assert.equal(rendered.openDVN, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
  assert.equal(rendered.priceFeed, "0xcccccccccccccccccccccccccccccccccccccccc");
  assert.equal(rendered.dvnVerifier, "0x9999999999999999999999999999999999999999");
  assert.deepEqual(rendered.workerPathwayConfig, {
    enabled: true,
    maxMessageSize: "10000",
    minLzReceiveGas: "200000",
    maxLzReceiveGas: "1000000",
  });
  assert.deepEqual(rendered.priceSnapshot, {
    dstGasPriceInSrcToken: "2",
    updatedAt: "1700000000",
    staleAfter: "1800",
  });
  assert.deepEqual(rendered.executorFeeModel, {
    baseFee: "1000",
    dstGasOverhead: "50000",
    marginBps: 1000,
  });
  assert.deepEqual(rendered.dvnFeeModel, {
    baseFee: "2000",
    dstGasOverhead: "150000",
    marginBps: 500,
  });
  assert.equal(Object.hasOwn(rendered, "sendConfig"), false);
  assert.equal(Object.hasOwn(rendered, "enforcedOptions"), false);
});

test("committed ignition parameter examples use split modules", () => {
  for (const file of [
    "ignition/parameters/sepolia.json",
    "ignition/parameters/hoodi.json",
  ]) {
    const parameters = readParameters(file);
    assert.deepEqual(Object.keys(parameters).sort(), ["OpenWorkers", "TestOFT"]);
  }

  for (const file of [
    "ignition/parameters/sepolia-to-hoodi.example.json",
    "ignition/parameters/hoodi-to-sepolia.example.json",
  ]) {
    const parameters = readParameters(file);
    assert.deepEqual(Object.keys(parameters).sort(), [
      "OAppEndpointConfig",
      "OpenWorkersPathwayConfig",
    ]);
    const oapp = parameters.OAppEndpointConfig as {
      sendConfig: { configType: number; config: `0x${string}` }[];
    };
    const workers = parameters.OpenWorkersPathwayConfig as {
      openExecutor: string;
      openDVN: string;
    };
    const executorConfig = oapp.sendConfig.find(
      (entry) => entry.configType === CONFIG_TYPE_EXECUTOR,
    );
    const ulnConfig = oapp.sendConfig.find(
      (entry) => entry.configType === CONFIG_TYPE_ULN,
    );
    assert.ok(executorConfig);
    assert.ok(ulnConfig);
    assert.equal(
      decodeExecutorConfig(executorConfig.config).executor.toLowerCase(),
      workers.openExecutor.toLowerCase(),
    );
    assert.ok(
      decodeUlnConfig(ulnConfig.config).requiredDVNs.some(
        (dvn) => dvn.toLowerCase() === workers.openDVN.toLowerCase(),
      ),
    );
  }
});

test("pathway config builders validate uint32 fields", () => {
  assert.throws(
    () =>
      buildOAppEndpointConfigParameters({
        ...basePathwayInput(),
        remoteEid: 2 ** 32,
      }),
    /remoteEid must be a uint32/,
  );
});

test("pathway config builders validate worker gas bounds", () => {
  assert.throws(
    () =>
      buildOpenWorkersPathwayConfigParameters({
        ...basePathwayInput(),
        minLzReceiveGas: 1_000_001n,
        maxLzReceiveGas: 1_000_000n,
      }),
    /minLzReceiveGas must not exceed maxLzReceiveGas/,
  );
});

test("pathway config builders validate worker fee model", () => {
  assert.throws(
    () =>
      buildOpenWorkersPathwayConfigParameters({
        ...basePathwayInput(),
        executorFeeModel: {
          ...basePathwayInput().executorFeeModel,
          marginBps: 10_001,
        },
      }),
    /executorFeeModel\.marginBps must be between 0 and 10000 bps/,
  );
});

function readParameters(file: string): Record<string, unknown> {
  return JSON.parse(readFileSync(file, "utf8")) as Record<string, unknown>;
}
