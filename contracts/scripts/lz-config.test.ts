import assert from "node:assert/strict";
import test from "node:test";
import {
  decodeExecutorConfig,
  decodeUlnConfig,
  encodeExecutorConfig,
  encodeUlnConfig,
  NIL_DVN_COUNT,
  requiredDVNsConfig,
  rollbackConfigBatches,
  rollbackConfigPlan,
  CONFIG_TYPE_EXECUTOR,
  CONFIG_TYPE_ULN,
  type LzConfigSnapshot,
} from "./lz-config.js";

test("executor config ABI round trips", () => {
  const config = {
    maxMessageSize: 10_000,
    executor: "0x2222222222222222222222222222222222222222" as const,
  };

  assert.deepEqual(decodeExecutorConfig(encodeExecutorConfig(config)), config);
});

test("required DVN config sorts addresses and disables optional DVNs", () => {
  const openDVN = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" as const;
  const externalDVN = "0x1111111111111111111111111111111111111111" as const;

  const config = requiredDVNsConfig(12n, [openDVN, externalDVN]);

  assert.equal(config.confirmations, 12n);
  assert.equal(config.requiredDVNCount, 2);
  assert.equal(config.optionalDVNCount, NIL_DVN_COUNT);
  assert.equal(config.optionalDVNThreshold, 0);
  assert.deepEqual(config.requiredDVNs, [externalDVN, openDVN]);
  assert.deepEqual(config.optionalDVNs, []);
});

test("ULN config ABI round trips required DVNs", () => {
  const config = requiredDVNsConfig(12n, [
    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "0x1111111111111111111111111111111111111111",
  ]);

  const decoded = decodeUlnConfig(encodeUlnConfig(config));
  assert.equal(decoded.confirmations, config.confirmations);
  assert.equal(decoded.requiredDVNCount, config.requiredDVNCount);
  assert.equal(decoded.optionalDVNCount, config.optionalDVNCount);
  assert.equal(decoded.optionalDVNThreshold, config.optionalDVNThreshold);
  assert.deepEqual(
    decoded.requiredDVNs.map((address) => address.toLowerCase()),
    config.requiredDVNs,
  );
  assert.deepEqual(decoded.optionalDVNs, config.optionalDVNs);
});

test("required DVN config rejects duplicate addresses case-insensitively", () => {
  assert.throws(
    () =>
      requiredDVNsConfig(12n, [
        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
      ]),
    /duplicate DVN address/,
  );
});

test("required DVN config rejects self-only required sets", () => {
  assert.throws(
    () =>
      requiredDVNsConfig(12n, [
        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      ]),
    /required DVNs must include at least two addresses/,
  );
});

test("rollback config batches restore executor and both ULN configs", () => {
  const snapshot = rollbackSnapshot();

  const batches = rollbackConfigBatches(snapshot);

  assert.deepEqual(batches, [
    {
      label: "Endpoint.setConfig SendUln302 rollback",
      library: snapshot.inspectedLibraries.sendUln,
      configs: [
        {
          eid: snapshot.remoteEid,
          configType: CONFIG_TYPE_EXECUTOR,
          config: encodeExecutorConfig(snapshot.executorConfig),
        },
        {
          eid: snapshot.remoteEid,
          configType: CONFIG_TYPE_ULN,
          config: encodeUlnConfig(snapshot.sendUlnConfig),
        },
      ],
    },
    {
      label: "Endpoint.setConfig ReceiveUln302 rollback",
      library: snapshot.inspectedLibraries.receiveUln,
      configs: [
        {
          eid: snapshot.remoteEid,
          configType: CONFIG_TYPE_ULN,
          config: encodeUlnConfig(snapshot.receiveUlnConfig),
        },
      ],
    },
  ]);
});

test("rollback config plan exposes dry-run review payload", () => {
  const snapshot = rollbackSnapshot();

  const plan = rollbackConfigPlan(snapshot);

  assert.equal(plan.endpoint, snapshot.endpoint);
  assert.equal(plan.oapp, snapshot.oapp);
  assert.equal(plan.remoteEid, snapshot.remoteEid);
  assert.deepEqual(plan.restoredLibraries, {
    sendUln: snapshot.inspectedLibraries.sendUln,
    receiveUln: snapshot.inspectedLibraries.receiveUln,
  });
  assert.deepEqual(plan.batches, rollbackConfigBatches(snapshot));
  assert.deepEqual(plan.restoredExecutorConfig, snapshot.executorConfig);
  assert.deepEqual(plan.restoredSendUlnConfig, snapshot.sendUlnConfig);
  assert.deepEqual(plan.restoredReceiveUlnConfig, snapshot.receiveUlnConfig);
});

test("rollback config batches reject mismatched DVN counts", () => {
  const snapshot: LzConfigSnapshot = {
    endpoint: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
    oapp: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    remoteEid: 40449,
    inspectedLibraries: {
      sendUln: "0x1111111111111111111111111111111111111111",
      receiveUln: "0x2222222222222222222222222222222222222222",
    },
    executorConfig: {
      maxMessageSize: 12_345,
      executor: "0x3333333333333333333333333333333333333333",
    },
    sendUlnConfig: {
      confirmations: 12n,
      requiredDVNCount: 2,
      optionalDVNCount: NIL_DVN_COUNT,
      optionalDVNThreshold: 0,
      requiredDVNs: ["0x4444444444444444444444444444444444444444"],
      optionalDVNs: [],
    },
    receiveUlnConfig: requiredDVNsConfig(12n, [
      "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "0x1111111111111111111111111111111111111111",
    ]),
  };

  assert.throws(
    () => rollbackConfigBatches(snapshot),
    /requiredDVNCount does not match requiredDVNs/,
  );
});

function rollbackSnapshot(): LzConfigSnapshot {
  return {
    endpoint: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
    oapp: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    remoteEid: 40449,
    inspectedLibraries: {
      sendUln: "0x1111111111111111111111111111111111111111",
      receiveUln: "0x2222222222222222222222222222222222222222",
    },
    executorConfig: {
      maxMessageSize: 12_345,
      executor: "0x3333333333333333333333333333333333333333",
    },
    sendUlnConfig: requiredDVNsConfig(12n, [
      "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "0x1111111111111111111111111111111111111111",
    ]),
    receiveUlnConfig: {
      confirmations: 15n,
      requiredDVNCount: 1,
      optionalDVNCount: 1,
      optionalDVNThreshold: 1,
      requiredDVNs: ["0x4444444444444444444444444444444444444444"],
      optionalDVNs: ["0x5555555555555555555555555555555555555555"],
    },
  };
}
