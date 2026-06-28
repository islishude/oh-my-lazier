import assert from "node:assert/strict";
import test from "node:test";
import {
  findSecretLoggingErrors,
  validateSecurityReview,
} from "./security-review.js";

test("security review check accepts current repository documents and logs", () => {
  assert.deepEqual(validateSecurityReview(), []);
});

test("secret logging guard rejects secret-bearing log calls", () => {
  const sources = new Map([
    [
      "go/internal/example.go",
      'logger.Info("loaded signer", "private_key", privateKey)\n',
    ],
    [
      "contracts/scripts/example.ts",
      'console.log("kms signature", signature);\n',
    ],
  ]);

  assert.deepEqual(findSecretLoggingErrors(sources), [
    "go/internal/example.go:1: log call mentions secret-bearing material",
    "contracts/scripts/example.ts:1: log call mentions secret-bearing material",
  ]);
});

test("secret logging guard allows non-log security validation text", () => {
  const sources = new Map([
    [
      "go/internal/example.go",
      'return errors.New("keystore password source is required")\n',
    ],
  ]);

  assert.deepEqual(findSecretLoggingErrors(sources), []);
});
