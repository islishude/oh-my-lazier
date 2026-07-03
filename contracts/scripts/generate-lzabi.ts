import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

type AbiInput = {
  name: string;
  type: string;
  internalType?: string;
  indexed?: boolean;
  components?: AbiInput[];
};

type AbiEntry = {
  type: string;
  name?: string;
  inputs?: AbiInput[];
  outputs?: AbiInput[];
  stateMutability?: string;
  anonymous?: boolean;
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
  selections?: AbiSelection[];
  entries?: AbiEntry[];
};

const outputs: AbiOutput[] = [
  {
    path: "go/internal/lzabi/abis/endpoint_v2.json",
    selections: [
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "isValidReceiveLibrary",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "verifiable",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "inboundPayloadHash",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "inboundNonce",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "lazyInboundNonce",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "lzReceive",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "event",
        name: "PacketSent",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "event",
        name: "PacketVerified",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "event",
        name: "PacketDelivered",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "event",
        name: "LzReceiveAlert",
      },
    ],
  },
  {
    path: "go/internal/lzabi/abis/open_dvn.json",
    selections: [
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
        type: "event",
        name: "DVNJobAssigned",
      },
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
        type: "function",
        name: "submitVerification",
      },
    ],
  },
  {
    path: "go/internal/lzabi/abis/open_executor.json",
    selections: [
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
        type: "event",
        name: "ExecutorJobAssigned",
      },
    ],
  },
  {
    path: "go/internal/lzabi/abis/receive_uln302.json",
    selections: [
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
        type: "function",
        name: "commitVerification",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
        type: "function",
        name: "verify",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
        type: "function",
        name: "getUlnConfig",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
        type: "function",
        name: "verifiable",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
        type: "function",
        name: "hashLookup",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/ReceiveUln302.sol/ReceiveUln302.json",
        type: "event",
        name: "PayloadVerified",
      },
    ],
  },
  {
    path: "go/internal/lzabi/abis/send_uln302.json",
    selections: [
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json",
        type: "event",
        name: "ExecutorFeePaid",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-messagelib-v2/artifacts/contracts/uln/uln302/SendUln302.sol/SendUln302.json",
        type: "event",
        name: "DVNFeePaid",
      },
    ],
  },
  {
    path: "go/internal/configcheck/abis/endpoint.json",
    selections: [
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "eid",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "getSendLibrary",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "getReceiveLibrary",
      },
      {
        artifact:
          "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
        type: "function",
        name: "getConfig",
      },
    ],
  },
  {
    path: "go/internal/configcheck/abis/oapp.json",
    selections: [
      {
        artifact:
          "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
        type: "function",
        name: "endpoint",
      },
      {
        artifact:
          "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
        type: "function",
        name: "peers",
      },
    ],
  },
  {
    path: "go/internal/configcheck/abis/worker.json",
    selections: [
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
        type: "function",
        name: "allowedSendLib",
      },
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
        type: "function",
        name: "pathwayConfig",
      },
      {
        artifact:
          "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
        type: "function",
        name: "verifiers",
      },
    ],
  },
  {
    path: "go/internal/configcheck/abis/config_decoder.json",
    entries: [
      {
        type: "function",
        name: "executorConfig",
        stateMutability: "pure",
        inputs: [],
        outputs: [
          {
            name: "",
            type: "tuple",
            components: [
              { name: "maxMessageSize", type: "uint32" },
              { name: "executor", type: "address" },
            ],
          },
        ],
      },
      {
        type: "function",
        name: "ulnConfig",
        stateMutability: "pure",
        inputs: [],
        outputs: [
          {
            name: "",
            type: "tuple",
            components: [
              { name: "confirmations", type: "uint64" },
              { name: "requiredDVNCount", type: "uint8" },
              { name: "optionalDVNCount", type: "uint8" },
              { name: "optionalDVNThreshold", type: "uint8" },
              { name: "requiredDVNs", type: "address[]" },
              { name: "optionalDVNs", type: "address[]" },
            ],
          },
        ],
      },
    ],
  },
];

const repoRoot = process.cwd();
const checkOnly = process.argv.includes("--check");

async function main() {
  const artifactCache = new Map<string, Artifact>();

  for (const output of outputs) {
    const abi =
      output.entries ??
      (await Promise.all(
        (output.selections ?? []).map((selection) =>
          readSelectedEntry(selection, artifactCache),
        ),
      ));
    const target = path.join(repoRoot, output.path);
    const formatted = formatABI(abi);
    if (checkOnly) {
      const current = await readFile(target, "utf8");
      if (current !== formatted) {
        throw new Error(`${output.path} is not generated from pinned artifacts`);
      }
    } else {
      await mkdir(path.dirname(target), { recursive: true });
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
  const dirs = new Set(outputs.map((output) => path.dirname(output.path)));
  for (const dir of dirs) {
    const expected = new Set(
      outputs
        .filter((output) => path.dirname(output.path) === dir)
        .map((output) => path.basename(output.path)),
    );
    const actual = await readdir(path.join(repoRoot, dir));
    const unexpected = actual
      .filter((entry) => entry.endsWith(".json") && !expected.has(entry))
      .sort();
    if (unexpected.length > 0) {
      throw new Error(
        `${dir}: unexpected ABI JSON files: ${unexpected.join(", ")}`,
      );
    }
  }
}

function formatABI(entries: AbiEntry[]): string {
  return `[\n${entries.map(formatEntry).join(",\n")}\n]\n`;
}

function formatEntry(entry: AbiEntry): string {
  if (entry.type === "function") {
    return formatFunction(entry);
  }
  if (entry.type === "event") {
    return formatEvent(entry);
  }
  throw new Error(`unsupported ABI entry type: ${entry.type}`);
}

function formatFunction(entry: AbiEntry): string {
  if (entry.name === undefined || entry.stateMutability === undefined) {
    throw new Error("function ABI entry is missing required fields");
  }
  return [
    "  {",
    `    "name": ${JSON.stringify(entry.name)},`,
    `    "type": "function",`,
    `    "stateMutability": ${JSON.stringify(entry.stateMutability)},`,
    `    "inputs": ${formatFunctionParameters(entry.inputs ?? [], 4)},`,
    `    "outputs": ${formatFunctionParameters(entry.outputs ?? [], 4)}`,
    "  }",
  ].join("\n");
}

function formatFunctionParameters(inputs: AbiInput[], indent: number): string {
  if (inputs.length === 0) {
    return "[]";
  }
  if (inputs.length === 1 && inputs[0].components === undefined) {
    return `[${formatCompactParameter(inputs[0])}]`;
  }

  const spaces = " ".repeat(indent);
  const lines = ["["];
  inputs.forEach((input, index) => {
    const suffix = index === inputs.length - 1 ? "" : ",";
    lines.push(`${formatFunctionParameter(input, indent + 2)}${suffix}`);
  });
  lines.push(`${spaces}]`);
  return lines.join("\n");
}

function formatFunctionParameter(input: AbiInput, indent: number): string {
  const spaces = " ".repeat(indent);
  if (input.components === undefined) {
    return `${spaces}${formatCompactParameter(input)}`;
  }

  const componentLines = input.components.map(
    (component, index) =>
      `${" ".repeat(indent + 4)}${formatCompactParameter(component)}${
        index === input.components!.length - 1 ? "" : ","
      }`,
  );
  return [
    `${spaces}{`,
    `${spaces}  "name": ${JSON.stringify(input.name)},`,
    `${spaces}  "type": "tuple",`,
    `${spaces}  "components": [`,
    ...componentLines,
    `${spaces}  ]`,
    `${spaces}}`,
  ].join("\n");
}

function formatEvent(entry: AbiEntry): string {
  if (entry.name === undefined || entry.anonymous === undefined) {
    throw new Error("event ABI entry is missing required fields");
  }
  return [
    "  {",
    `    "anonymous": ${JSON.stringify(entry.anonymous)},`,
    `    "inputs": ${formatEventParameters(entry.inputs ?? [], 4)},`,
    `    "name": ${JSON.stringify(entry.name)},`,
    `    "type": "event"`,
    "  }",
  ].join("\n");
}

function formatEventParameters(inputs: AbiInput[], indent: number): string {
  const spaces = " ".repeat(indent);
  if (inputs.length === 0) {
    return "[]";
  }

  const lines = ["["];
  inputs.forEach((input, index) => {
    const suffix = index === inputs.length - 1 ? "" : ",";
    lines.push(`${formatEventParameter(input, indent + 2)}${suffix}`);
  });
  lines.push(`${spaces}]`);
  return lines.join("\n");
}

function formatEventParameter(input: AbiInput, indent: number): string {
  const spaces = " ".repeat(indent);
  if (input.components === undefined) {
    return [
      `${spaces}{`,
      `${spaces}  "indexed": ${JSON.stringify(input.indexed)},`,
      `${spaces}  "internalType": ${JSON.stringify(input.internalType)},`,
      `${spaces}  "name": ${JSON.stringify(input.name)},`,
      `${spaces}  "type": ${JSON.stringify(input.type)}`,
      `${spaces}}`,
    ].join("\n");
  }

  const componentLines = input.components.map(
    (component, index) =>
      `${" ".repeat(indent + 4)}${formatCompactEventComponent(component)}${
        index === input.components!.length - 1 ? "" : ","
      }`,
  );
  return [
    `${spaces}{`,
    `${spaces}  "components": [`,
    ...componentLines,
    `${spaces}  ],`,
    `${spaces}  "indexed": ${JSON.stringify(input.indexed)},`,
    `${spaces}  "internalType": ${JSON.stringify(input.internalType)},`,
    `${spaces}  "name": ${JSON.stringify(input.name)},`,
    `${spaces}  "type": ${JSON.stringify(input.type)}`,
    `${spaces}}`,
  ].join("\n");
}

function formatCompactParameter(input: AbiInput): string {
  return `{ "name": ${JSON.stringify(input.name)}, "type": ${JSON.stringify(
    input.type,
  )} }`;
}

function formatCompactEventComponent(input: AbiInput): string {
  return `{ "internalType": ${JSON.stringify(
    input.internalType,
  )}, "name": ${JSON.stringify(input.name)}, "type": ${JSON.stringify(
    input.type,
  )} }`;
}

await main();
