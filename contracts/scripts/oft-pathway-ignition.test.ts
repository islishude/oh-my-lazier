import assert from "node:assert/strict";
import test from "node:test";
import {
  decodeExecutorConfig,
  decodeUlnConfig,
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  NIL_DVN_COUNT,
} from "./lz-config.js";
import {
  buildTestOFTPathwayConfigParameters,
  OFT_MSG_TYPE_SEND,
} from "./oft-pathway-ignition.js";

function basePathwayInput() {
  return {
    testOFT: "0x1111111111111111111111111111111111111111",
    endpoint: "0x2222222222222222222222222222222222222222",
    remoteEid: 40449,
    remoteOFT: "0x4444444444444444444444444444444444444444",
    sendUln: "0x5555555555555555555555555555555555555555",
    receiveUln: "0x6666666666666666666666666666666666666666",
    openExecutor: "0x7777777777777777777777777777777777777777",
    openDVN: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    layerZeroLabsDVN: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    confirmations: 12n,
    maxMessageSize: 10_000,
    minLzReceiveGas: 200_000n,
    maxLzReceiveGas: 1_000_000n,
    executorPriceConfig: {
      baseFee: 1000n,
      dstGasPriceInSrcToken: 2n,
      bufferBps: 1000,
      updatedAt: 1_700_000_000n,
      staleAfter: 1800n,
    },
    dvnPriceConfig: {
      baseFee: 2000n,
      dstGasPriceInSrcToken: 3n,
      bufferBps: 500n,
      updatedAt: 1_700_000_001n,
      staleAfter: 1801n,
    },
    enforcedLzReceiveGas: 200_000n,
  } as const;
}

test("buildTestOFTPathwayConfigParameters renders endpoint and OFT config", () => {
  const rendered = buildTestOFTPathwayConfigParameters({
    ...basePathwayInput(),
    delegate: "0x3333333333333333333333333333333333333333",
    dvnVerifier: "0x9999999999999999999999999999999999999999",
  }).TestOFTPathwayConfig;

  assert.equal(rendered.testOFT, "0x1111111111111111111111111111111111111111");
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
  assert.equal(rendered.openExecutor, "0x7777777777777777777777777777777777777777");
  assert.equal(rendered.openDVN, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
  assert.equal(rendered.dvnVerifier, "0x9999999999999999999999999999999999999999");
  assert.deepEqual(rendered.workerPathwayConfig, {
    enabled: true,
    maxMessageSize: "10000",
    minLzReceiveGas: "200000",
    maxLzReceiveGas: "1000000",
  });
  assert.deepEqual(rendered.executorPriceConfig, {
    baseFee: "1000",
    dstGasPriceInSrcToken: "2",
    bufferBps: 1000,
    updatedAt: "1700000000",
    staleAfter: "1800",
  });
  assert.deepEqual(rendered.dvnPriceConfig, {
    baseFee: "2000",
    dstGasPriceInSrcToken: "3",
    bufferBps: 500,
    updatedAt: "1700000001",
    staleAfter: "1801",
  });
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

test("buildTestOFTPathwayConfigParameters validates uint32 fields", () => {
  assert.throws(
    () =>
      buildTestOFTPathwayConfigParameters({
        ...basePathwayInput(),
        remoteEid: 2 ** 32,
      }),
    /remoteEid must be a uint32/,
  );
});

test("buildTestOFTPathwayConfigParameters validates worker gas bounds", () => {
  assert.throws(
    () =>
      buildTestOFTPathwayConfigParameters({
        ...basePathwayInput(),
        minLzReceiveGas: 1_000_001n,
        maxLzReceiveGas: 1_000_000n,
      }),
    /minLzReceiveGas must not exceed maxLzReceiveGas/,
  );
});

test("buildTestOFTPathwayConfigParameters validates worker price config", () => {
  assert.throws(
    () =>
      buildTestOFTPathwayConfigParameters({
        ...basePathwayInput(),
        executorPriceConfig: {
          ...basePathwayInput().executorPriceConfig,
          bufferBps: 10_001,
        },
      }),
    /executorPriceConfig\.bufferBps must be between 0 and 10000 bps/,
  );
});
