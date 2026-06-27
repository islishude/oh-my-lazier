import assert from "node:assert/strict";
import test from "node:test";
import {
  decodeExecutorConfig,
  decodeUlnConfig,
  encodeExecutorConfig,
  encodeUlnConfig,
  NIL_DVN_COUNT,
  requiredDVNsConfig,
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
  const layerZeroLabsDVN = "0x1111111111111111111111111111111111111111" as const;

  const config = requiredDVNsConfig(12n, [openDVN, layerZeroLabsDVN]);

  assert.equal(config.confirmations, 12n);
  assert.equal(config.requiredDVNCount, 2);
  assert.equal(config.optionalDVNCount, NIL_DVN_COUNT);
  assert.equal(config.optionalDVNThreshold, 0);
  assert.deepEqual(config.requiredDVNs, [layerZeroLabsDVN, openDVN]);
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
