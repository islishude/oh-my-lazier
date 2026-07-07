import {
  decodeErrorResult,
  keccak256,
  toBytes,
  type Abi,
  type Hex,
} from "viem";

type AbiErrorItem = {
  type: "error";
  name: string;
  inputs: readonly { type: string }[];
};

export type KnownContractError = {
  name: string;
  signature: string;
  selector: Hex;
  args?: readonly unknown[];
};

export function decodeKnownContractError(
  error: unknown,
  abi: Abi,
): KnownContractError | undefined {
  const selectors = knownErrorSelectors(abi);
  for (const data of extractHexCandidates(error)) {
    const selector = data.slice(0, 10) as Hex;
    const item = selectors.get(selector);
    if (item === undefined) {
      continue;
    }
    try {
      const decoded = decodeErrorResult({ abi, data });
      return {
        name: decoded.errorName,
        signature: errorSignature(item),
        selector,
        args: decoded.args,
      };
    } catch {
      return {
        name: item.name,
        signature: errorSignature(item),
        selector,
      };
    }
  }
  return undefined;
}

export function formatKnownContractError(
  decoded: KnownContractError,
): string {
  const args =
    decoded.args === undefined ? "" : ` args=${jsonStringify(decoded.args)}`;
  return `${decoded.signature} selector=${decoded.selector}${args}`;
}

export function knownContractErrorHint(
  decoded: KnownContractError,
): string | undefined {
  switch (decoded.name) {
    case "PriceSnapshotStale":
      return "Refresh the source OpenPriceFeed price snapshot for this dstEid before retrying, or run check:price-config to inspect the current updatedAt/staleAfter values.";
    case "DuplicateLzReceiveOption":
      return "Retry with empty extraOptions; configured pathways already enforce the lzReceive option.";
    case "PathwayDisabled":
      return "Configure or enable the source OpenExecutor/OpenDVN pathway for this source OApp and destination EID.";
    case "UnauthorizedSendLib":
      return "Allow the source SendUln302 address on the configured OpenExecutor/OpenDVN worker.";
    case "MissingLzReceiveOption":
      return "Configure enforced lzReceive options on the OApp pathway, or pass an explicit lzReceive gas option for an unenforced pathway.";
    default:
      return undefined;
  }
}

export function enrichKnownContractError(input: {
  error: unknown;
  abi: Abi;
  context: string;
}): Error | undefined {
  const decoded = decodeKnownContractError(input.error, input.abi);
  if (decoded === undefined) {
    return undefined;
  }
  const hint = knownContractErrorHint(decoded);
  return new Error(
    `${input.context} reverted with ${formatKnownContractError(decoded)}${hint === undefined ? "" : `. ${hint}`}`,
    { cause: input.error },
  );
}

function knownErrorSelectors(abi: Abi): Map<Hex, AbiErrorItem> {
  const selectors = new Map<Hex, AbiErrorItem>();
  for (const item of abi) {
    if (item.type !== "error") {
      continue;
    }
    selectors.set(errorSelector(item), item);
  }
  return selectors;
}

function extractHexCandidates(error: unknown): Hex[] {
  const candidates: Hex[] = [];
  const seen = new Set<unknown>();
  collectHexCandidates(error, candidates, seen);
  candidates.sort((a, b) => b.length - a.length);
  return candidates;
}

function collectHexCandidates(
  value: unknown,
  candidates: Hex[],
  seen: Set<unknown>,
): void {
  if (typeof value === "string") {
    for (const [candidate] of value.matchAll(/0x[0-9a-fA-F]{8,}/g)) {
      if (candidate.length % 2 === 0) {
        candidates.push(candidate as Hex);
      }
    }
    return;
  }
  if (value === null || typeof value !== "object" || seen.has(value)) {
    return;
  }
  seen.add(value);
  if (value instanceof Error) {
    collectHexCandidates(value.message, candidates, seen);
    collectHexCandidates(value.cause, candidates, seen);
  }
  for (const item of Object.values(value as Record<string, unknown>)) {
    collectHexCandidates(item, candidates, seen);
  }
}

function errorSelector(item: AbiErrorItem): Hex {
  return keccak256(toBytes(errorSignature(item))).slice(0, 10) as Hex;
}

function errorSignature(item: AbiErrorItem): string {
  return `${item.name}(${item.inputs.map((input) => input.type).join(",")})`;
}

function jsonStringify(value: unknown): string {
  return JSON.stringify(value, (_key, item) =>
    typeof item === "bigint" ? item.toString() : item,
  );
}
