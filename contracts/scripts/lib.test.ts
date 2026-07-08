import assert from "node:assert/strict";
import test from "node:test";
import { optionalAddressList, parseAddressList, parseCLIParams } from "./lib.js";

test("parseCLIParams accepts kebab-case flags with separate values", () => {
  const params = parseCLIParams([
    "--rpc-url",
    "https://example.invalid",
    "--chain-id",
    "11155111",
  ]);

  assert.equal(params.get("rpc-url"), "https://example.invalid");
  assert.equal(params.get("chain-id"), "11155111");
});

test("parseCLIParams accepts equals syntax and normalizes underscores", () => {
  const params = parseCLIParams([
    "--test_oft=0x1111111111111111111111111111111111111111",
    "--dry-run",
  ]);

  assert.equal(
    params.get("test-oft"),
    "0x1111111111111111111111111111111111111111",
  );
  assert.equal(params.get("dry-run"), "true");
});

test("parseAddressList parses comma-separated EVM addresses", () => {
  assert.deepEqual(parseAddressList(
    "0x1111111111111111111111111111111111111111, 0x2222222222222222222222222222222222222222",
    "REQUIRED_DVNS",
  ), [
    "0x1111111111111111111111111111111111111111",
    "0x2222222222222222222222222222222222222222",
  ]);
});

test("optionalAddressList parses env input only when present", () => {
  const previous = process.env.TEST_OPTIONAL_DVNS;
  delete process.env.TEST_OPTIONAL_DVNS;
  try {
    assert.equal(optionalAddressList("TEST_OPTIONAL_DVNS"), undefined);
    process.env.TEST_OPTIONAL_DVNS =
      "0x1111111111111111111111111111111111111111,0x2222222222222222222222222222222222222222";
    assert.deepEqual(optionalAddressList("TEST_OPTIONAL_DVNS"), [
      "0x1111111111111111111111111111111111111111",
      "0x2222222222222222222222222222222222222222",
    ]);
  } finally {
    if (previous === undefined) {
      delete process.env.TEST_OPTIONAL_DVNS;
    } else {
      process.env.TEST_OPTIONAL_DVNS = previous;
    }
  }
});

test("parseAddressList rejects empty address entries", () => {
  assert.throws(
    () =>
      parseAddressList(
        "0x1111111111111111111111111111111111111111,",
        "REQUIRED_DVNS",
      ),
    /REQUIRED_DVNS\[1\] must be an EVM address/,
  );
});
