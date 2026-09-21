import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { readVerifiedABIFile } from "../../scripts/generate-pricing-abi.js";

const original = '[{"type":"function","name":"decimals"}]\n';
const sha256 = createHash("sha256").update(original).digest("hex");

for (const scenario of [
  { name: "valid file", content: original, hash: sha256, error: undefined },
  { name: "tampered content", content: `${original} `, hash: sha256, error: /source SHA-256 mismatch/ },
  { name: "missing file", content: undefined, hash: sha256, error: /ENOENT/ },
  { name: "missing hash", content: original, hash: undefined, error: /missing or invalid source SHA-256/ },
  { name: "invalid hash", content: original, hash: "invalid", error: /missing or invalid source SHA-256/ },
]) {
  test(`local pricing ABI source: ${scenario.name}`, async () => {
    const dir = await mkdtemp(path.join(tmpdir(), "pricing-abi-"));
    try {
      const file = path.join(dir, "abi.json");
      if (scenario.content !== undefined) {
        await writeFile(file, scenario.content);
      }
      if (scenario.error !== undefined) {
        await assert.rejects(readVerifiedABIFile(file, scenario.hash), scenario.error);
      } else {
        assert.equal(await readVerifiedABIFile(file, scenario.hash), original);
      }
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
}
