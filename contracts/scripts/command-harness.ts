import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createInterface } from "node:readline/promises";
import type { HardhatRuntimeEnvironment } from "hardhat/types/hre";
import type { NetworkConnection } from "hardhat/types/network";
import type { Address, PublicClient, WalletClient } from "viem";

export const SCRIPT_PARAMS_ENV = "OML_SCRIPT_PARAMS";

export type JsonObject = Record<string, unknown>;

export type ValueParser<T> = (value: unknown, label: string) => T;

export type ScriptRunFile<T> = {
  input: T;
  apply?: boolean;
  confirmation?: "interactive" | "approved";
};

export type LoadScriptRunFileOptions = {
  allowMissing?: boolean;
  cwd?: string;
  environment?: Readonly<Record<string, string | undefined>>;
};

const secretFieldFragments = [
  "apikey",
  "authorization",
  "authtoken",
  "bearertoken",
  "credential",
  "credentials",
  "mnemonic",
  "password",
  "privatekey",
  "rpcurl",
  "secret",
  "secretkey",
  "signingkey",
] as const;

/**
 * Load and validate the common command envelope from OML_SCRIPT_PARAMS.
 *
 * The command-specific parser is responsible for defining the exact fields in
 * `input`. Missing parameter files are accepted only for commands that opt in.
 */
export function loadScriptRunFile<T>(
  parseInput: ValueParser<T>,
  options: LoadScriptRunFileOptions = {}
): ScriptRunFile<T> {
  const environment = options.environment ?? process.env;
  const paramsPath = environment[SCRIPT_PARAMS_ENV];

  if (paramsPath === undefined) {
    if (options.allowMissing !== true) {
      throw new Error(`${SCRIPT_PARAMS_ENV} is required`);
    }
    return { input: parseInput({}, "input") };
  }
  if (paramsPath.trim() === "") {
    throw new Error(`${SCRIPT_PARAMS_ENV} must not be empty`);
  }

  const absolutePath = resolve(options.cwd ?? process.cwd(), paramsPath);
  let source: string;
  try {
    source = readFileSync(absolutePath, "utf8");
  } catch {
    throw new Error(
      `${SCRIPT_PARAMS_ENV} file could not be read: ${absolutePath}`
    );
  }

  let value: unknown;
  try {
    value = JSON.parse(source) as unknown;
  } catch {
    // JSON.parse can include excerpts from the input in its error. Do not echo
    // them because the file is an untrusted boundary for secret material.
    throw new Error(`${SCRIPT_PARAMS_ENV} file contains invalid JSON`);
  }

  assertNoSecretFields(value);
  const envelope = expectObject(value, "script parameters");
  expectOnlyKeys(
    envelope,
    ["input", "apply", "confirmation"],
    "script parameters"
  );

  const input = requiredField(
    envelope,
    "input",
    parseInput,
    "script parameters"
  );
  const apply = optionalField(
    envelope,
    "apply",
    parseBoolean,
    "script parameters"
  );
  const confirmation = optionalField(
    envelope,
    "confirmation",
    (item, label) => parseEnum(item, label, ["interactive", "approved"]),
    "script parameters"
  );

  return {
    input,
    ...(apply === undefined ? {} : { apply }),
    ...(confirmation === undefined ? {} : { confirmation }),
  };
}

/** Reject fields that could carry raw credentials before command validation. */
export function assertNoSecretFields(
  value: unknown,
  label = "script parameters",
  allowedReferenceFields: readonly string[] = []
): void {
  const allowed = new Set(allowedReferenceFields.map(normalizeFieldName));
  assertNoSecretFieldsRecursive(value, label, allowed);
}

function assertNoSecretFieldsRecursive(
  value: unknown,
  label: string,
  allowedReferenceFields: ReadonlySet<string>
): void {
  if (Array.isArray(value)) {
    value.forEach((item, index) =>
      assertNoSecretFieldsRecursive(
        item,
        `${label}[${index}]`,
        allowedReferenceFields
      )
    );
    return;
  }
  if (!isObject(value)) {
    return;
  }

  for (const [key, item] of Object.entries(value)) {
    const normalizedKey = normalizeFieldName(key);
    if (
      !allowedReferenceFields.has(normalizedKey) &&
      secretFieldFragments.some((fragment) => normalizedKey.includes(fragment))
    ) {
      throw new Error(
        `${label}.${key} is not allowed to contain secret material`
      );
    }
    assertNoSecretFieldsRecursive(
      item,
      `${label}.${key}`,
      allowedReferenceFields
    );
  }
}

export function expectObject(value: unknown, label: string): JsonObject {
  if (!isObject(value)) {
    throw new Error(`${label} must be a JSON object`);
  }
  return value;
}

export function expectOnlyKeys(
  object: JsonObject,
  allowedKeys: readonly string[],
  label: string
): void {
  const allowed = new Set(allowedKeys);
  const unknownKeys = Object.keys(object).filter((key) => !allowed.has(key));
  if (unknownKeys.length > 0) {
    throw new Error(
      `${label} contains unknown field${
        unknownKeys.length === 1 ? "" : "s"
      }: ${unknownKeys.join(", ")}`
    );
  }
}

export function requiredField<T>(
  object: JsonObject,
  key: string,
  parser: ValueParser<T>,
  objectLabel = "input"
): T {
  if (!Object.hasOwn(object, key)) {
    throw new Error(`${objectLabel}.${key} is required`);
  }
  return parser(object[key], `${objectLabel}.${key}`);
}

export function optionalField<T>(
  object: JsonObject,
  key: string,
  parser: ValueParser<T>,
  objectLabel = "input"
): T | undefined {
  if (!Object.hasOwn(object, key)) {
    return undefined;
  }
  return parser(object[key], `${objectLabel}.${key}`);
}

export function parseString(value: unknown, label: string): string {
  if (typeof value !== "string") {
    throw new Error(`${label} must be a string`);
  }
  return value;
}

export function parseNonEmptyString(value: unknown, label: string): string {
  const parsed = parseString(value, label);
  if (parsed.trim() === "") {
    throw new Error(`${label} must not be empty`);
  }
  return parsed;
}

export function parseBoolean(value: unknown, label: string): boolean {
  if (typeof value !== "boolean") {
    throw new Error(`${label} must be a boolean`);
  }
  return value;
}

/** Parse a canonical, unsigned base-10 integer without losing precision. */
export function parseUnsignedDecimal(value: unknown, label: string): string {
  const parsed = parseString(value, label);
  if (!/^(0|[1-9][0-9]*)$/.test(parsed)) {
    throw new Error(`${label} must be an unsigned decimal string`);
  }
  return parsed;
}

/** Parse an unsigned decimal string that is representable as a JS integer. */
export function parseSafeInteger(value: unknown, label: string): number {
  const decimal = parseUnsignedDecimal(value, label);
  const parsed = Number(decimal);
  if (!Number.isSafeInteger(parsed)) {
    throw new Error(`${label} exceeds the safe integer range`);
  }
  return parsed;
}

export function parseAddress(value: unknown, label: string): Address {
  const parsed = parseString(value, label);
  if (!/^0x[0-9a-fA-F]{40}$/.test(parsed)) {
    throw new Error(`${label} must be an EVM address`);
  }
  return parsed as Address;
}

export function parseArray<T>(
  value: unknown,
  label: string,
  parseItem: ValueParser<T>,
  options: { minLength?: number } = {}
): T[] {
  if (!Array.isArray(value)) {
    throw new Error(`${label} must be an array`);
  }
  const minLength = options.minLength ?? 0;
  if (value.length < minLength) {
    throw new Error(`${label} must contain at least ${minLength} item(s)`);
  }
  return value.map((item, index) => parseItem(item, `${label}[${index}]`));
}

export function parseEnum<const T extends readonly string[]>(
  value: unknown,
  label: string,
  choices: T
): T[number] {
  const parsed = parseString(value, label);
  if (!choices.includes(parsed)) {
    throw new Error(`${label} must be one of: ${choices.join(", ")}`);
  }
  return parsed as T[number];
}

export type ApplyTarget = {
  network: string;
  chainId: number;
  deploymentIds?: readonly string[];
};

export type ApplySummary = {
  command: string;
  targets: readonly ApplyTarget[];
  actions: readonly string[];
};

export type ApplyConfirmationIO = {
  isTTY: boolean;
  write(message: string): void;
  question(prompt: string): Promise<string>;
};

/**
 * A per-command apply gate. Once approved, subsequent authorization calls on
 * the same gate do not prompt again.
 */
export class ApplyGate {
  readonly #apply: boolean;
  readonly #confirmation: "interactive" | "approved";
  readonly #io: ApplyConfirmationIO;
  #authorized = false;

  public constructor(
    runFile: Pick<ScriptRunFile<unknown>, "apply" | "confirmation">,
    io: ApplyConfirmationIO = defaultConfirmationIO()
  ) {
    this.#apply = runFile.apply ?? false;
    this.#confirmation = runFile.confirmation ?? "interactive";
    this.#io = io;
  }

  public get shouldApply(): boolean {
    return this.#apply;
  }

  public async authorize(summary: ApplySummary): Promise<boolean> {
    if (!this.#apply) {
      return false;
    }
    if (this.#authorized) {
      return true;
    }
    if (this.#confirmation === "approved") {
      this.#authorized = true;
      return true;
    }
    if (!this.#io.isTTY) {
      throw new Error(
        'apply requires confirmation:"approved" when stdin/stderr are not TTYs'
      );
    }

    this.#io.write(renderApplySummary(summary));
    const answer = await this.#io.question('Type "yes" to apply: ');
    if (answer.trim().toLowerCase() !== "yes") {
      throw new Error("apply cancelled by user");
    }
    this.#authorized = true;
    return true;
  }
}

export function createApplyGate(
  runFile: Pick<ScriptRunFile<unknown>, "apply" | "confirmation">,
  io?: ApplyConfirmationIO
): ApplyGate {
  return new ApplyGate(runFile, io);
}

export function renderApplySummary(summary: ApplySummary): string {
  const lines = [`Apply command: ${summary.command}`, "Targets:"];
  for (const target of summary.targets) {
    lines.push(`- ${target.network} (chain id ${target.chainId})`);
    for (const deploymentId of target.deploymentIds ?? []) {
      lines.push(`  deployment: ${deploymentId}`);
    }
  }
  lines.push("Actions:");
  for (const action of summary.actions) {
    lines.push(`- ${action}`);
  }
  return `${lines.join("\n")}\n`;
}

export type RunCommandOptions = {
  writeError?: (message: string) => void;
  setExitCode?: (exitCode: number) => void;
};

/** A machine-readable failure payload that must be emitted without decoration. */
export class CommandOutputError extends Error {
  public readonly output: string;

  public constructor(output: string) {
    super("command reported a structured failure");
    this.name = "CommandOutputError";
    this.output = output;
  }
}

/** Run a top-level command without printing a stack or forcing process exit. */
export async function runCommand(
  main: () => void | Promise<void>,
  options: RunCommandOptions = {}
): Promise<boolean> {
  try {
    await main();
    return true;
  } catch (error) {
    const writeError =
      options.writeError ?? ((message) => console.error(message));
    const setExitCode =
      options.setExitCode ?? ((exitCode) => (process.exitCode = exitCode));
    writeError(
      error instanceof CommandOutputError
        ? error.output
        : `error: ${sanitizeCommandErrorMessage(commandErrorMessage(error))}`
    );
    setExitCode(1);
    return false;
  }
}

/** Redact credentials and query or fragment data from URLs in thrown errors. */
export function sanitizeCommandErrorMessage(message: string): string {
  const urlsRedacted = message.replace(
    /\bhttps?:\/\/[^\s"'<>()[\]{}]+/giu,
    (candidate) => redactURL(candidate)
  );
  return urlsRedacted.replace(
    /\b(api[_-]?key|access[_-]?token|password|secret)=([^\s&,;]+)/giu,
    "$1=<redacted>"
  );
}

export type NetworkSelection = {
  network?: string;
  expectedChainId?: number;
};

export type ReadOnlyNetworkContext = {
  publicClient: PublicClient;
  networkName: string;
  chainId: number;
};

export type WriteNetworkContext = ReadOnlyNetworkContext & {
  connection: NetworkConnection;
  walletClient: WalletClient;
  signerAddress: Address;
};

/**
 * Create a connection whose configured accounts are overridden with `remote`,
 * validate its chain ID, and always close it after the callback finishes.
 */
export async function withReadOnlyConnection<T>(
  hre: HardhatRuntimeEnvironment,
  selection: NetworkSelection,
  callback: (context: ReadOnlyNetworkContext) => Promise<T>
): Promise<T> {
  const connection = await hre.network.create({
    ...(selection.network === undefined ? {} : { network: selection.network }),
    override: { accounts: "remote" },
  });
  try {
    const publicClient = await connection.viem.getPublicClient();
    const chainId = await assertConnectionChainId(
      connection,
      publicClient,
      selection.expectedChainId
    );
    return await callback({
      publicClient,
      networkName: connection.networkName,
      chainId,
    });
  } finally {
    await connection.close();
  }
}

/** Create a signer-backed connection and always close it after use. */
export async function withWriteConnection<T>(
  hre: HardhatRuntimeEnvironment,
  selection: NetworkSelection,
  callback: (context: WriteNetworkContext) => Promise<T>
): Promise<T> {
  const connection =
    selection.network === undefined
      ? await hre.network.create()
      : await hre.network.create(selection.network);
  try {
    const publicClient = await connection.viem.getPublicClient();
    const chainId = await assertConnectionChainId(
      connection,
      publicClient,
      selection.expectedChainId
    );
    const [walletClient] = await connection.viem.getWalletClients();
    const signerAddress = walletClient?.account?.address;
    if (walletClient === undefined || signerAddress === undefined) {
      throw new Error(
        `Hardhat network ${connection.networkName} has no configured signer`
      );
    }
    return await callback({
      connection,
      publicClient,
      walletClient,
      signerAddress,
      networkName: connection.networkName,
      chainId,
    });
  } finally {
    await connection.close();
  }
}

export async function assertConnectionChainId(
  connection: Pick<NetworkConnection, "networkName" | "networkConfig">,
  publicClient: Pick<PublicClient, "chain" | "getChainId">,
  expectedChainId?: number
): Promise<number> {
  const configuredChainId = connection.networkConfig.chainId;
  if (configuredChainId === undefined) {
    throw new Error(
      `Hardhat network ${connection.networkName} must configure chainId`
    );
  }
  if (expectedChainId !== undefined && configuredChainId !== expectedChainId) {
    throw new Error(
      `Hardhat network ${connection.networkName} chain id ${configuredChainId} does not match expected chain id ${expectedChainId}`
    );
  }
  if (
    publicClient.chain !== undefined &&
    publicClient.chain.id !== configuredChainId
  ) {
    throw new Error(
      `Viem chain id ${publicClient.chain.id} does not match configured chain id ${configuredChainId}`
    );
  }
  const rpcChainId = await publicClient.getChainId();
  if (rpcChainId !== configuredChainId) {
    throw new Error(
      `RPC chain id ${rpcChainId} does not match configured chain id ${configuredChainId}`
    );
  }
  return configuredChainId;
}

export function assertExpectedSigner(
  actualSigner: Address,
  expectedSigner: Address,
  role = "expected signer"
): void {
  if (actualSigner.toLowerCase() !== expectedSigner.toLowerCase()) {
    throw new Error(
      `configured signer ${actualSigner} does not match ${role} ${expectedSigner}`
    );
  }
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function normalizeFieldName(value: string): string {
  return value.toLowerCase().replaceAll(/[^a-z0-9]/g, "");
}

function commandErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === "string") {
    return error;
  }
  return "unknown command failure";
}

function redactURL(candidate: string): string {
  const trailingMatch = candidate.match(/[.,;:!?]+$/u);
  const trailing = trailingMatch?.[0] ?? "";
  const value =
    trailing === "" ? candidate : candidate.slice(0, -trailing.length);
  try {
    const url = new URL(value);
    return `${url.protocol}//${url.host}/<redacted>${trailing}`;
  } catch {
    return `<redacted-url>${trailing}`;
  }
}

function defaultConfirmationIO(): ApplyConfirmationIO {
  return {
    isTTY: process.stdin.isTTY === true && process.stderr.isTTY === true,
    write(message) {
      process.stderr.write(message);
    },
    async question(prompt) {
      const readline = createInterface({
        input: process.stdin,
        output: process.stderr,
      });
      try {
        return await readline.question(prompt);
      } finally {
        readline.close();
      }
    },
  };
}
