import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

type AbiInput = {
  name: string;
  type: string;
  internalType?: string;
  components?: AbiInput[];
};

type AbiEntry = {
  type: string;
  name?: string;
  inputs?: AbiInput[];
  outputs?: AbiInput[];
  stateMutability?: string;
};

type Artifact = {
  abi: AbiEntry[];
};

type AbiSelection = {
  artifact: string;
  type: string;
  name: string;
};

type AbiOutput = {
  path: string;
  selections: AbiSelection[];
};

const outputs: AbiOutput[] = [
  {
    path: "go/internal/pricing/abis/price_snapshot.json",
    selections: [
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenPriceFeed.sol/OpenPriceFeed.json",
        type: "function",
        name: "setPriceSnapshot",
      },
    ],
  },
  {
    path: "go/internal/pricing/abis/uniswap_v3_quoter.json",
    selections: [
      {
        artifact:
          "node_modules/@uniswap/v3-periphery/artifacts/contracts/interfaces/IQuoterV2.sol/IQuoterV2.json",
        type: "function",
        name: "quoteExactInputSingle",
      },
    ],
  },
];

const repoRoot = process.cwd();
const checkOnly = process.argv.includes("--check");

async function main() {
  const artifactCache = new Map<string, Artifact>();

  for (const output of outputs) {
    const abi = await Promise.all(
      output.selections.map((selection) =>
        readSelectedEntry(selection, artifactCache),
      ),
    );
    const target = path.join(repoRoot, output.path);
    const formatted = formatABI(abi);
    if (checkOnly) {
      const current = await readFile(target, "utf8");
      if (current !== formatted) {
        throw new Error(`${output.path} is not generated from pinned artifacts`);
      }
    } else {
      await writeFile(target, formatted);
    }
  }

  if (checkOnly) {
    await assertNoUnexpectedABIJSON();
  }
}

async function readSelectedEntry(
  selection: AbiSelection,
  artifactCache: Map<string, Artifact>,
): Promise<AbiEntry> {
  const artifact = await readArtifact(selection.artifact, artifactCache);
  const matches = artifact.abi.filter(
    (entry) => entry.type === selection.type && entry.name === selection.name,
  );
  if (matches.length !== 1) {
    throw new Error(
      `${selection.artifact}: expected one ${selection.type} ${selection.name}, found ${matches.length}`,
    );
  }
  return matches[0];
}

async function readArtifact(
  artifactPath: string,
  artifactCache: Map<string, Artifact>,
): Promise<Artifact> {
  const absolutePath = path.join(repoRoot, artifactPath);
  const cached = artifactCache.get(absolutePath);
  if (cached !== undefined) {
    return cached;
  }

  const raw = await readFile(absolutePath, "utf8");
  const parsed = JSON.parse(raw) as Partial<Artifact>;
  if (!Array.isArray(parsed.abi)) {
    throw new Error(`${artifactPath}: missing ABI array`);
  }
  const artifact = { abi: parsed.abi };
  artifactCache.set(absolutePath, artifact);
  return artifact;
}

async function assertNoUnexpectedABIJSON() {
  const abiDir = path.join(repoRoot, "go/internal/pricing/abis");
  const expected = new Set(outputs.map((output) => path.basename(output.path)));
  const actual = await readdir(abiDir);
  const unexpected = actual
    .filter((entry) => entry.endsWith(".json") && !expected.has(entry))
    .sort();
  if (unexpected.length > 0) {
    throw new Error(`unexpected ABI JSON files: ${unexpected.join(", ")}`);
  }
}

function formatABI(entries: AbiEntry[]): string {
  return `[\n${entries.map(formatEntry).join(",\n")}\n]\n`;
}

function formatEntry(entry: AbiEntry): string {
  if (
    entry.type !== "function" ||
    entry.name === undefined ||
    entry.stateMutability === undefined
  ) {
    throw new Error("pricing ABI generator only supports named functions");
  }

  return [
    "  {",
    `    "inputs": ${formatParameters(entry.inputs ?? [], 4)},`,
    `    "name": ${JSON.stringify(entry.name)},`,
    `    "outputs": ${formatParameters(entry.outputs ?? [], 4)},`,
    `    "stateMutability": ${JSON.stringify(entry.stateMutability)},`,
    `    "type": "function"`,
    "  }",
  ].join("\n");
}

function formatParameters(inputs: AbiInput[], indent: number): string {
  if (inputs.length === 0) {
    return "[]";
  }

  const spaces = " ".repeat(indent);
  const lines = ["["];
  inputs.forEach((input, index) => {
    const suffix = index === inputs.length - 1 ? "" : ",";
    lines.push(`${formatParameter(input, indent + 2)}${suffix}`);
  });
  lines.push(`${spaces}]`);
  return lines.join("\n");
}

function formatParameter(input: AbiInput, indent: number): string {
  const spaces = " ".repeat(indent);
  if (input.components === undefined) {
    return `${spaces}${formatCompactParameter(input)}`;
  }

  const componentLines = input.components.map((component, index) => {
    const suffix = index === input.components!.length - 1 ? "" : ",";
    return `${formatParameter(component, indent + 4)}${suffix}`;
  });
  return [
    `${spaces}{`,
    `${spaces}  "components": [`,
    ...componentLines,
    `${spaces}  ],`,
    `${spaces}  "internalType": ${JSON.stringify(input.internalType)},`,
    `${spaces}  "name": ${JSON.stringify(input.name)},`,
    `${spaces}  "type": ${JSON.stringify(input.type)}`,
    `${spaces}}`,
  ].join("\n");
}

function formatCompactParameter(input: AbiInput): string {
  return `{ "internalType": ${JSON.stringify(
    input.internalType,
  )}, "name": ${JSON.stringify(input.name)}, "type": ${JSON.stringify(
    input.type,
  )} }`;
}

await main();
