import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { NetworkConnection } from "hardhat/types/network";
import type { Address, PublicClient, WalletClient } from "viem";
import {
  ApplyGate,
  CommandOutputError,
  assertConnectionChainId,
  assertExpectedSigner,
  expectObject,
  expectOnlyKeys,
  loadScriptRunFile,
  optionalField,
  parseArray,
  parseBoolean,
  parseNonEmptyString,
  parseSafeInteger,
  parseString,
  parseUnsignedDecimal,
  requiredField,
  runCommand,
  sanitizeCommandErrorMessage,
  type ApplyConfirmationIO,
  type ApplySummary,
  withReadOnlyConnection,
  withWriteConnection,
} from "../../scripts/command-harness.js";

type ExampleInput = {
  name: string;
  amount: string;
  enabled?: boolean;
};

const applySummary: ApplySummary = {
  command: "configure:example",
  targets: [
    {
      network: "sepolia",
      chainId: 11155111,
      deploymentIds: ["sepolia-open-workers"],
    },
  ],
  actions: ["set worker configuration"],
};

test("loadScriptRunFile loads a strict envelope without losing integer precision", () => {
  const directory = mkdtempSync(join(tmpdir(), "oml-command-harness-"));
  const path = join(directory, "params.json");
  writeFileSync(
    path,
    JSON.stringify({
      input: {
        name: "example",
        amount: "9007199254740993123456789",
        enabled: true,
      },
      apply: false,
      confirmation: "interactive",
    })
  );

  const loaded = loadScriptRunFile(parseExampleInput, {
    environment: { OML_SCRIPT_PARAMS: path },
  });

  assert.deepEqual(loaded, {
    input: {
      name: "example",
      amount: "9007199254740993123456789",
      enabled: true,
    },
    apply: false,
    confirmation: "interactive",
  });
});

test("loadScriptRunFile permits an omitted file only when explicitly allowed", () => {
  assert.throws(
    () =>
      loadScriptRunFile(parseEmptyInput, {
        environment: {},
      }),
    /OML_SCRIPT_PARAMS is required/
  );
  assert.deepEqual(
    loadScriptRunFile(parseEmptyInput, {
      allowMissing: true,
      environment: {},
    }),
    { input: {} }
  );
});

test("loadScriptRunFile rejects non-object and unknown envelope fields", () => {
  const directory = mkdtempSync(join(tmpdir(), "oml-command-harness-"));
  const arrayPath = join(directory, "array.json");
  const unknownPath = join(directory, "unknown.json");
  writeFileSync(arrayPath, "[]");
  writeFileSync(unknownPath, JSON.stringify({ input: {}, legacy: true }));

  assert.throws(
    () =>
      loadScriptRunFile(parseEmptyInput, {
        environment: { OML_SCRIPT_PARAMS: arrayPath },
      }),
    /script parameters must be a JSON object/
  );
  assert.throws(
    () =>
      loadScriptRunFile(parseEmptyInput, {
        environment: { OML_SCRIPT_PARAMS: unknownPath },
      }),
    /script parameters contains unknown field: legacy/
  );
});

test("loadScriptRunFile rejects secret fields without echoing their value", () => {
  const directory = mkdtempSync(join(tmpdir(), "oml-command-harness-"));
  for (const field of [
    "private_key",
    "keystorePassword",
    "rpcCredentials",
    "etherscanApiKey",
  ]) {
    const path = join(directory, `${field}.json`);
    const secret = `do-not-print-${field}`;
    writeFileSync(
      path,
      JSON.stringify({ input: { nested: { [field]: secret } } })
    );

    assert.throws(
      () =>
        loadScriptRunFile(parseEmptyInput, {
          environment: { OML_SCRIPT_PARAMS: path },
        }),
      (error: unknown) => {
        assert(error instanceof Error);
        assert.match(error.message, new RegExp(`${field} is not allowed`));
        assert.doesNotMatch(error.message, new RegExp(secret));
        return true;
      }
    );
  }
});

test("loadScriptRunFile hides invalid JSON excerpts", () => {
  const directory = mkdtempSync(join(tmpdir(), "oml-command-harness-"));
  const path = join(directory, "invalid.json");
  writeFileSync(path, '{"privateKey":"do-not-echo",');

  assert.throws(
    () =>
      loadScriptRunFile(parseEmptyInput, {
        environment: { OML_SCRIPT_PARAMS: path },
      }),
    (error: unknown) => {
      assert(error instanceof Error);
      assert.equal(
        error.message,
        "OML_SCRIPT_PARAMS file contains invalid JSON"
      );
      return true;
    }
  );
});

test("schema primitives reject unknown fields and incorrect JSON types", () => {
  assert.throws(
    () => parseExampleInput({ name: "x", amount: "1", extra: true }, "input"),
    /input contains unknown field: extra/
  );
  assert.throws(
    () => parseExampleInput({ name: "x", amount: 1 }, "input"),
    /input.amount must be a string/
  );
  assert.throws(
    () => parseUnsignedDecimal("01", "input.amount"),
    /input.amount must be an unsigned decimal string/
  );
  assert.throws(
    () => parseSafeInteger("9007199254740992", "input.count"),
    /input.count exceeds the safe integer range/
  );
  assert.deepEqual(
    parseArray(["one", "two"], "input.values", parseString, { minLength: 1 }),
    ["one", "two"]
  );
});

test("ApplyGate keeps apply:false read-only without prompting", async () => {
  const confirmation = fakeConfirmationIO(false, []);
  const gate = new ApplyGate({ apply: false }, confirmation.io);

  assert.equal(await gate.authorize(applySummary), false);
  assert.equal(confirmation.questionCount(), 0);
  assert.equal(confirmation.output(), "");
});

test("ApplyGate requires recorded approval outside a TTY", async () => {
  const confirmation = fakeConfirmationIO(false, []);
  const gate = new ApplyGate(
    { apply: true, confirmation: "interactive" },
    confirmation.io
  );

  await assert.rejects(
    () => gate.authorize(applySummary),
    /apply requires confirmation:"approved"/
  );
  assert.equal(confirmation.questionCount(), 0);
});

test("ApplyGate accepts non-interactive approval without prompting", async () => {
  const confirmation = fakeConfirmationIO(false, []);
  const gate = new ApplyGate(
    { apply: true, confirmation: "approved" },
    confirmation.io
  );

  assert.equal(await gate.authorize(applySummary), true);
  assert.equal(await gate.authorize(applySummary), true);
  assert.equal(confirmation.questionCount(), 0);
});

test("ApplyGate prompts and renders the plan exactly once", async () => {
  const confirmation = fakeConfirmationIO(true, ["yes"]);
  const gate = new ApplyGate(
    { apply: true, confirmation: "interactive" },
    confirmation.io
  );

  assert.equal(await gate.authorize(applySummary), true);
  assert.equal(await gate.authorize(applySummary), true);
  assert.equal(confirmation.questionCount(), 1);
  assert.match(confirmation.output(), /configure:example/);
  assert.match(confirmation.output(), /sepolia \(chain id 11155111\)/);
  assert.match(confirmation.output(), /sepolia-open-workers/);
});

test("ApplyGate rejects an interactive response other than yes", async () => {
  const confirmation = fakeConfirmationIO(true, ["no"]);
  const gate = new ApplyGate({ apply: true }, confirmation.io);

  await assert.rejects(
    () => gate.authorize(applySummary),
    /apply cancelled by user/
  );
});

test("runCommand reports a concise error and sets exit code", async () => {
  const messages: string[] = [];
  const exitCodes: number[] = [];

  const succeeded = await runCommand(
    () => {
      throw new Error("bad input");
    },
    {
      writeError: (message) => messages.push(message),
      setExitCode: (exitCode) => exitCodes.push(exitCode),
    }
  );

  assert.equal(succeeded, false);
  assert.deepEqual(messages, ["error: bad input"]);
  assert.deepEqual(exitCodes, [1]);
});

test("runCommand preserves structured JSON failures without an error prefix", async () => {
  const messages: string[] = [];
  const exitCodes: number[] = [];
  const output = JSON.stringify({ ok: false, errors: ["invalid"] });

  await runCommand(
    () => {
      throw new CommandOutputError(output);
    },
    {
      writeError: (message) => messages.push(message),
      setExitCode: (exitCode) => exitCodes.push(exitCode),
    }
  );

  assert.deepEqual(messages, [output]);
  assert.deepEqual(exitCodes, [1]);
  assert.deepEqual(JSON.parse(messages[0]), {
    ok: false,
    errors: ["invalid"],
  });
});

test("command errors redact credential-bearing URLs", () => {
  const message = sanitizeCommandErrorMessage(
    "RPC https://alice:password@example.invalid/v3/project?apiKey=secret#fragment failed; access_token=also-secret"
  );

  assert.doesNotMatch(
    message,
    /alice|password@example|apiKey=secret|also-secret/
  );
  assert.match(message, /https:\/\/example\.invalid\/<redacted>/);
  assert.doesNotMatch(message, /v3\/project|fragment/);
  assert.match(message, /access_token=<redacted>/);
});

test("withReadOnlyConnection overrides accounts, validates, and closes", async () => {
  const fixture = fakeNetworkFixture({ chainId: 11155111 });
  let walletRequested = false;
  fixture.connection.viem.getWalletClients = async () => {
    walletRequested = true;
    return [];
  };

  const result = await withReadOnlyConnection(
    fixture.hre,
    { network: "sepolia", expectedChainId: 11155111 },
    async ({ networkName, chainId }) => `${networkName}:${chainId}`
  );

  assert.equal(result, "sepolia:11155111");
  assert.deepEqual(fixture.createArguments(), [
    { network: "sepolia", override: { accounts: "remote" } },
  ]);
  assert.equal(walletRequested, false);
  assert.equal(fixture.closeCount(), 1);
});

test("withReadOnlyConnection closes after callback or chain validation failure", async () => {
  const callbackFailure = fakeNetworkFixture({ chainId: 1 });
  await assert.rejects(
    () =>
      withReadOnlyConnection(callbackFailure.hre, {}, async () => {
        throw new Error("callback failed");
      }),
    /callback failed/
  );
  assert.equal(callbackFailure.closeCount(), 1);

  const mismatch = fakeNetworkFixture({ chainId: 1, rpcChainId: 2 });
  await assert.rejects(
    () => withReadOnlyConnection(mismatch.hre, {}, async () => undefined),
    /RPC chain id 2 does not match configured chain id 1/
  );
  assert.equal(mismatch.closeCount(), 1);
});

test("withWriteConnection selects the first configured signer and closes", async () => {
  const signer = "0x1111111111111111111111111111111111111111" as Address;
  const fixture = fakeNetworkFixture({ chainId: 1, signer });

  const actual = await withWriteConnection(
    fixture.hre,
    { network: "write-network", expectedChainId: 1 },
    async ({ signerAddress }) => signerAddress
  );

  assert.equal(actual, signer);
  assert.deepEqual(fixture.createArguments(), ["write-network"]);
  assert.equal(fixture.closeCount(), 1);
});

test("withWriteConnection fails closed when no signer is configured", async () => {
  const fixture = fakeNetworkFixture({ chainId: 1 });

  await assert.rejects(
    () => withWriteConnection(fixture.hre, {}, async () => undefined),
    /has no configured signer/
  );
  assert.equal(fixture.closeCount(), 1);
});

test("assertConnectionChainId requires configured and expected chain IDs", async () => {
  const client = fakePublicClient(1);
  await assert.rejects(
    () =>
      assertConnectionChainId(
        {
          networkName: "missing",
          networkConfig: {} as NetworkConnection["networkConfig"],
        },
        client
      ),
    /must configure chainId/
  );
  await assert.rejects(
    () =>
      assertConnectionChainId(
        {
          networkName: "wrong",
          networkConfig: { chainId: 1 } as NetworkConnection["networkConfig"],
        },
        client,
        2
      ),
    /does not match expected chain id 2/
  );
});

test("assertExpectedSigner compares addresses case-insensitively", () => {
  const actual = "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" as Address;
  const expected = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" as Address;
  assert.doesNotThrow(() => assertExpectedSigner(actual, expected, "owner"));
  assert.throws(
    () =>
      assertExpectedSigner(
        actual,
        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "delegate"
      ),
    /does not match delegate/
  );
});

function parseExampleInput(value: unknown, label: string): ExampleInput {
  const object = expectObject(value, label);
  expectOnlyKeys(object, ["name", "amount", "enabled"], label);
  const enabled = optionalField(object, "enabled", parseBoolean, label);
  return {
    name: requiredField(object, "name", parseNonEmptyString, label),
    amount: requiredField(object, "amount", parseUnsignedDecimal, label),
    ...(enabled === undefined ? {} : { enabled }),
  };
}

function parseEmptyInput(value: unknown, label: string): Record<string, never> {
  const object = expectObject(value, label);
  expectOnlyKeys(object, [], label);
  return {};
}

function fakeConfirmationIO(isTTY: boolean, answers: string[]) {
  let questions = 0;
  let output = "";
  const io: ApplyConfirmationIO = {
    isTTY,
    write(message) {
      output += message;
    },
    async question() {
      const answer = answers[questions];
      questions += 1;
      return answer ?? "";
    },
  };
  return {
    io,
    output: () => output,
    questionCount: () => questions,
  };
}

function fakePublicClient(chainId: number, rpcChainId = chainId): PublicClient {
  return {
    chain: { id: chainId },
    getChainId: async () => rpcChainId,
  } as unknown as PublicClient;
}

function fakeNetworkFixture(options: {
  chainId: number;
  rpcChainId?: number;
  signer?: Address;
}) {
  let closes = 0;
  const createArguments: unknown[] = [];
  const publicClient = fakePublicClient(
    options.chainId,
    options.rpcChainId ?? options.chainId
  );
  const walletClients =
    options.signer === undefined
      ? []
      : ([
          { account: { address: options.signer } },
        ] as unknown as WalletClient[]);
  const connection = {
    networkName: options.chainId === 11155111 ? "sepolia" : "test",
    networkConfig: { chainId: options.chainId },
    viem: {
      getPublicClient: async () => publicClient,
      getWalletClients: async () => walletClients,
    },
    close: async () => {
      closes += 1;
    },
  } as unknown as NetworkConnection;
  const hre = {
    network: {
      create: async (argument?: unknown) => {
        createArguments.push(argument);
        return connection;
      },
    },
  } as unknown as HardhatRuntimeEnvironment;
  return {
    connection,
    hre,
    closeCount: () => closes,
    createArguments: () => createArguments,
  };
}
