import assert from "node:assert/strict";
import test from "node:test";
import type { Address, Hex } from "viem";
import { parseConfigureWorkersCommandInput } from "../../scripts/commands/configure-workers-input.js";
import {
  configureWorkers,
  type ConfigureWorkersInput,
} from "../../scripts/configure-workers.js";
import type { ChainClients } from "../../scripts/lib.js";

test("configureWorkers performs the existing worker transaction sequence", async () => {
  const input = configureInput({
    testOFT: address(1),
    srcOApp: undefined,
    rateLimit: { capacity: 1_000n, refillPerSecond: 10n },
  });
  const fixture = clientFixture(input.priceFeed);
  const result = await configureWorkers(input, fixture.clients, {
    now: 1_700_000_000n,
  });

  assert.deepEqual(
    fixture.writes.map((write) => write.functionName),
    [
      "setOutboundRateLimit",
      "setPriceSnapshot",
      "setAllowedSendLib",
      "setPathwayConfig",
      "setFeeModel",
      "setPriceFeed",
      "setAllowedSendLib",
      "setPathwayConfig",
      "setFeeModel",
    ]
  );
  assert.deepEqual(fixture.writes[0].args, [input.remoteEid, input.rateLimit]);
  assert.deepEqual(fixture.writes[1].args, [
    [
      {
        dstEid: input.remoteEid,
        snapshot: { ...input.priceSnapshot, updatedAt: 1_700_000_000n },
      },
    ],
  ]);
  assert.deepEqual(fixture.writes[3].args, [
    input.remoteEid,
    input.testOFT,
    {
      enabled: true,
      maxMessageSize: input.maxMessageSize,
      minLzReceiveGas: input.minLzReceiveGas,
      maxLzReceiveGas: input.maxLzReceiveGas,
    },
  ]);
  assert.deepEqual(fixture.writes[4].args, [
    input.remoteEid,
    input.executorFeeModel,
  ]);
  assert.deepEqual(fixture.writes[8].args, [
    input.remoteEid,
    input.dvnFeeModel,
  ]);
  assert.deepEqual(result, {
    chainId: 11_155_111,
    sender: address(20),
    testOFT: input.testOFT,
    openExecutor: input.openExecutor,
    openDVN: input.openDVN,
    priceFeed: input.priceFeed,
    remoteEid: input.remoteEid,
    sendLib: input.sendLib,
    srcOApp: input.testOFT,
  });
});

test("configureWorkers validates source OApp before sending transactions", async () => {
  const input = configureInput();
  delete input.srcOApp;
  const fixture = clientFixture(input.priceFeed);

  await assert.rejects(
    () => configureWorkers(input, fixture.clients),
    /input\.srcOApp or input\.testOFT is required/
  );
  assert.equal(fixture.writes.length, 0);
});

test("configureWorkers requires TestOFT for rate-limit changes", async () => {
  const input = configureInput({
    rateLimit: { capacity: 100n, refillPerSecond: 1n },
  });
  const fixture = clientFixture(input.priceFeed);

  await assert.rejects(
    () => configureWorkers(input, fixture.clients),
    /testOFT is required for TestOFT rate-limit changes/
  );
  assert.equal(fixture.writes.length, 0);
});

test("parseConfigureWorkersCommandInput preserves decimal precision", () => {
  const parsed = parseConfigureWorkersCommandInput(
    jsonInput({
      maxMessageSize: "9007199254740993",
      expectedSigner: address(20),
    }),
    "input"
  );

  assert.equal(parsed.maxMessageSize, 9_007_199_254_740_993n);
  assert.equal(parsed.expectedSigner, address(20));
  assert.deepEqual(parsed.rateLimit, {
    capacity: 1_000n,
    refillPerSecond: 10n,
  });
});

test("parseConfigureWorkersCommandInput rejects partial nested rate limits", () => {
  assert.throws(
    () =>
      parseConfigureWorkersCommandInput(
        jsonInput({ rateLimit: { capacity: "1000" } }),
        "input"
      ),
    /input\.rateLimit\.refillPerSecond is required/
  );
});

function configureInput(
  overrides: Partial<ConfigureWorkersInput> = {}
): ConfigureWorkersInput {
  return {
    openExecutor: address(2),
    openDVN: address(3),
    priceFeed: address(4),
    remoteEid: 40_149,
    sendLib: address(5),
    srcOApp: address(6),
    maxMessageSize: 10_000n,
    minLzReceiveGas: 200_000n,
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
    ...overrides,
  };
}

function jsonInput(overrides: Record<string, unknown> = {}) {
  return {
    testOFT: address(1),
    openExecutor: address(2),
    openDVN: address(3),
    priceFeed: address(4),
    remoteEid: "40149",
    sendLib: address(5),
    maxMessageSize: "10000",
    minLzReceiveGas: "200000",
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
    rateLimit: { capacity: "1000", refillPerSecond: "10" },
    ...overrides,
  };
}

function clientFixture(configuredPriceFeed: Address) {
  const writes: Array<{
    functionName: string;
    args?: readonly unknown[];
  }> = [];
  let nextHash = 1;
  let readCount = 0;
  const clients = {
    account: { address: address(20) },
    publicClient: {
      getChainId: async () => 11_155_111,
      readContract: async () => {
        readCount += 1;
        return readCount === 1 ? configuredPriceFeed : address(99);
      },
      waitForTransactionReceipt: async () => ({ status: "success" }),
    },
    walletClient: {
      chain: undefined,
      writeContract: async (request: {
        functionName: string;
        args?: readonly unknown[];
      }) => {
        writes.push(request);
        const hash = `0x${nextHash.toString(16).padStart(64, "0")}` as Hex;
        nextHash += 1;
        return hash;
      },
    },
  } as unknown as ChainClients;
  return { clients, writes };
}

function address(value: number): Address {
  return `0x${value.toString(16).padStart(40, "0")}` as Address;
}
