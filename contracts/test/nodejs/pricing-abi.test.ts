import assert from "node:assert/strict";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { verifyVendoredPricingABIs } from "../../scripts/generate-pricing-abi.js";

for (const scenario of [
  "valid",
  "tampered",
  "missing",
  "missing manifest entry",
] as const) {
  test(`vendored pricing ABI verification: ${scenario}`, async () => {
    const directory = await mkdtemp(path.join(tmpdir(), "pricing-abi-"));
    try {
      await cp("contracts/vendor/abis", directory, { recursive: true });
      const abi = path.join(directory, "chainlink_aggregator_v3.json");
      if (scenario === "tampered") await writeFile(abi, "[]\n");
      if (scenario === "missing") await rm(abi);
      if (scenario === "missing manifest entry") {
        const manifest = path.join(directory, "sources.json");
        const sources = JSON.parse(await readFile(manifest, "utf8"));
        await writeFile(manifest, JSON.stringify(sources.slice(1)));
      }
      if (scenario === "valid") {
        await verifyVendoredPricingABIs(directory);
      } else {
        await assert.rejects(
          verifyVendoredPricingABIs(directory),
          scenario === "tampered"
            ? /SHA-256 mismatch/
            : scenario === "missing"
              ? /ENOENT/
              : /exactly the expected sources/
        );
      }
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });
}
