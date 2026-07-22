import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

test("migration evidence wrapper preserves JSON stderr and exit code", async (t) => {
  const directory = await mkdtemp(path.join(tmpdir(), "oml-output-contract-"));
  t.after(() => rm(directory, { recursive: true, force: true }));

  const evidence = JSON.parse(
    await readFile(
      "docs/deployments/sepolia-hoodi/migration-evidence.json",
      "utf8"
    )
  ) as Record<string, unknown>;
  evidence.evidenceType = "invalid";
  const evidencePath = path.join(directory, "evidence.json");
  const paramsPath = path.join(directory, "params.json");
  await writeFile(evidencePath, JSON.stringify(evidence));
  await writeFile(
    paramsPath,
    JSON.stringify({ input: { migrationEvidence: evidencePath } })
  );

  const result = await runProcess(
    process.execPath,
    [
      path.resolve("node_modules/hardhat/dist/src/cli.js"),
      "run",
      "--no-compile",
      path.resolve("contracts/scripts/commands/check-migration-evidence.ts"),
    ],
    {
      ...process.env,
      OML_SCRIPT_PARAMS: paramsPath,
    }
  );

  assert.equal(result.exitCode, 1);
  assert.equal(result.stdout, "");
  const output = JSON.parse(result.stderr.trim()) as {
    ok: boolean;
    errors: string[];
  };
  assert.equal(output.ok, false);
  assert.match(output.errors[0], /evidenceType/);
});

function runProcess(
  executable: string,
  args: readonly string[],
  environment: NodeJS.ProcessEnv
): Promise<{ exitCode: number | null; stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    const child = spawn(executable, [...args], {
      cwd: process.cwd(),
      env: environment,
      shell: false,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk: string) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk: string) => {
      stderr += chunk;
    });
    child.once("error", reject);
    child.once("close", (exitCode) => {
      resolve({ exitCode, stdout, stderr });
    });
  });
}
