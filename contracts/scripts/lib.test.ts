import assert from "node:assert/strict";
import test from "node:test";
import { parseCLIParams } from "./lib.js";

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
