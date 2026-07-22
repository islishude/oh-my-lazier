import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import path from "node:path";
import test from "node:test";

const offlineWrappers = [
  "deploy-profile.ts",
  "render-oft-pathway-params.ts",
  "generate-lzabi.ts",
  "check-lzabi.ts",
  "generate-pricing-abi.ts",
  "check-pricing-abi.ts",
  "check-lz-addresses.ts",
  "check-migration-evidence.ts",
  "check-npm-audit-disposition.ts",
  "check-security-review.ts",
  "check-runbooks.ts",
] as const;

test("every offline Hardhat wrapper executes after dynamic import", async () => {
  const hardhatCli = path.resolve("node_modules/hardhat/dist/src/cli.js");
  const missingParams = path.resolve(
    "tmp/wrapper-smoke/parameters-must-not-exist.json"
  );

  for (const wrapper of offlineWrappers) {
    const result = await runProcess(
      process.execPath,
      [
        hardhatCli,
        "run",
        "--no-compile",
        path.resolve("contracts/scripts/commands", wrapper),
      ],
      {
        ...process.env,
        OML_SCRIPT_PARAMS: missingParams,
      }
    );

    assert.notEqual(result.exitCode, 0, `${wrapper} silently did not execute`);
    assert.match(
      `${result.stdout}\n${result.stderr}`,
      /OML_SCRIPT_PARAMS file could not be read/,
      `${wrapper} did not reach the command harness`
    );
  }
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
