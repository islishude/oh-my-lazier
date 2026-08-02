import {
  jsonStringify,
  loadArtifact,
  waitForTx,
  type ChainClients,
} from "./lib.js";
import type { Abi, Address, PublicClient } from "viem";

export type OFTPathwayAction =
  | "inspect"
  | "pause-send"
  | "unpause-send"
  | "pause-receive"
  | "unpause-receive"
  | "drain"
  | "clear-rate-limit"
  | "set-rate-limit";

export type RateLimitConfig = {
  capacity: bigint;
  refillPerSecond: bigint;
};

export type OFTPathwayState = {
  chainId: number;
  testOFT: Address;
  remoteEid: number;
  sendPaused: boolean;
  receivePaused: boolean;
  outboundRateLimitConfigured: boolean;
  outboundRateLimitConfig: RateLimitConfig;
};

export type ExpectedOFTPathwayState = {
  sendPaused?: boolean;
  receivePaused?: boolean;
  outboundRateLimitConfigured?: boolean;
  outboundRateLimitConfig?: RateLimitConfig;
};

export function expectedStateForAction(
  action: OFTPathwayAction,
  rateLimit?: RateLimitConfig
): ExpectedOFTPathwayState {
  switch (action) {
    case "inspect":
      return {};
    case "pause-send":
      return { sendPaused: true };
    case "unpause-send":
      return { sendPaused: false };
    case "pause-receive":
      return { receivePaused: true };
    case "unpause-receive":
      return { receivePaused: false };
    case "drain":
      return {
        outboundRateLimitConfigured: true,
        outboundRateLimitConfig: { capacity: 0n, refillPerSecond: 0n },
      };
    case "clear-rate-limit":
      return { outboundRateLimitConfigured: false };
    case "set-rate-limit":
      if (rateLimit === undefined) {
        throw new Error(
          "RATE_LIMIT_CAPACITY and RATE_LIMIT_REFILL_PER_SECOND are required"
        );
      }
      return {
        outboundRateLimitConfigured: true,
        outboundRateLimitConfig: rateLimit,
      };
  }
}

export function validateOFTPathwayState(
  state: OFTPathwayState,
  expected: ExpectedOFTPathwayState
): string[] {
  const errors: string[] = [];
  if (
    expected.sendPaused !== undefined &&
    state.sendPaused !== expected.sendPaused
  ) {
    errors.push(
      `sendPaused is ${state.sendPaused}, expected ${expected.sendPaused}`
    );
  }
  if (
    expected.receivePaused !== undefined &&
    state.receivePaused !== expected.receivePaused
  ) {
    errors.push(
      `receivePaused is ${state.receivePaused}, expected ${expected.receivePaused}`
    );
  }
  if (expected.outboundRateLimitConfig !== undefined) {
    if (!state.outboundRateLimitConfigured) {
      errors.push("outbound rate limit is not configured");
    }
    if (
      state.outboundRateLimitConfig.capacity !==
      expected.outboundRateLimitConfig.capacity
    ) {
      errors.push(
        `rate-limit capacity is ${state.outboundRateLimitConfig.capacity}, expected ${expected.outboundRateLimitConfig.capacity}`
      );
    }
    if (
      state.outboundRateLimitConfig.refillPerSecond !==
      expected.outboundRateLimitConfig.refillPerSecond
    ) {
      errors.push(
        `rate-limit refillPerSecond is ${state.outboundRateLimitConfig.refillPerSecond}, expected ${expected.outboundRateLimitConfig.refillPerSecond}`
      );
    }
  }
  if (
    expected.outboundRateLimitConfigured !== undefined &&
    state.outboundRateLimitConfigured !== expected.outboundRateLimitConfigured
  ) {
    errors.push(
      `outboundRateLimitConfigured is ${state.outboundRateLimitConfigured}, expected ${expected.outboundRateLimitConfigured}`
    );
  }
  return errors;
}

export async function readOFTPathwayState(input: {
  publicClient: PublicClient;
  testOFT: Address;
  remoteEid: number;
  testOFTAbi: Abi;
}): Promise<OFTPathwayState> {
  const [chainId, sendPaused, receivePaused, configured, config] =
    await Promise.all([
      input.publicClient.getChainId(),
      input.publicClient.readContract({
        address: input.testOFT,
        abi: input.testOFTAbi,
        functionName: "sendPaused",
        args: [input.remoteEid],
      }) as Promise<boolean>,
      input.publicClient.readContract({
        address: input.testOFT,
        abi: input.testOFTAbi,
        functionName: "receivePaused",
        args: [input.remoteEid],
      }) as Promise<boolean>,
      input.publicClient.readContract({
        address: input.testOFT,
        abi: input.testOFTAbi,
        functionName: "outboundRateLimitConfigured",
        args: [input.remoteEid],
      }) as Promise<boolean>,
      input.publicClient.readContract({
        address: input.testOFT,
        abi: input.testOFTAbi,
        functionName: "outboundRateLimitConfig",
        args: [input.remoteEid],
      }),
    ]);

  return {
    chainId,
    testOFT: input.testOFT,
    remoteEid: input.remoteEid,
    sendPaused,
    receivePaused,
    outboundRateLimitConfigured: configured,
    outboundRateLimitConfig: normalizeRateLimitConfig(config),
  };
}

function normalizeRateLimitConfig(value: unknown): RateLimitConfig {
  if (Array.isArray(value)) {
    return {
      capacity: value[0] as bigint,
      refillPerSecond: value[1] as bigint,
    };
  }
  const item = value as { capacity: bigint; refillPerSecond: bigint };
  return {
    capacity: item.capacity,
    refillPerSecond: item.refillPerSecond,
  };
}

export type RunOFTPathwayInput = {
  action: OFTPathwayAction;
  testOFT: Address;
  remoteEid: number;
  rateLimit?: RateLimitConfig;
};

export async function inspectOFTPathway(
  input: Omit<RunOFTPathwayInput, "action" | "rateLimit">,
  publicClient: PublicClient
): Promise<void> {
  const testOFTArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json"
  );
  const state = await readOFTPathwayState({
    publicClient,
    testOFT: input.testOFT,
    remoteEid: input.remoteEid,
    testOFTAbi: testOFTArtifact.abi,
  });
  console.log(jsonStringify({ ok: true, action: "inspect", state }));
}

export async function applyOFTPathway(
  input: RunOFTPathwayInput,
  clients: ChainClients
): Promise<void> {
  if (input.action === "inspect") {
    throw new Error("inspect is a read-only OFT pathway action");
  }
  const testOFTArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json"
  );
  const expected = expectedStateForAction(input.action, input.rateLimit);
  let hash;
  if (input.action === "pause-send" || input.action === "unpause-send") {
    hash = await clients.walletClient.writeContract({
      address: input.testOFT,
      abi: testOFTArtifact.abi,
      functionName: "pauseSend",
      args: [input.remoteEid, input.action === "pause-send"],
      account: clients.account,
      chain: clients.walletClient.chain,
    });
  } else if (
    input.action === "pause-receive" ||
    input.action === "unpause-receive"
  ) {
    hash = await clients.walletClient.writeContract({
      address: input.testOFT,
      abi: testOFTArtifact.abi,
      functionName: "pauseReceive",
      args: [input.remoteEid, input.action === "pause-receive"],
      account: clients.account,
      chain: clients.walletClient.chain,
    });
  } else if (input.action === "clear-rate-limit") {
    hash = await clients.walletClient.writeContract({
      address: input.testOFT,
      abi: testOFTArtifact.abi,
      functionName: "clearOutboundRateLimit",
      args: [input.remoteEid],
      account: clients.account,
      chain: clients.walletClient.chain,
    });
  } else {
    const config =
      input.action === "drain"
        ? { capacity: 0n, refillPerSecond: 0n }
        : input.rateLimit;
    if (config === undefined) {
      throw new Error("rate-limit config is required");
    }
    hash = await clients.walletClient.writeContract({
      address: input.testOFT,
      abi: testOFTArtifact.abi,
      functionName: "setOutboundRateLimit",
      args: [input.remoteEid, config],
      account: clients.account,
      chain: clients.walletClient.chain,
    });
  }
  await waitForTx(clients.publicClient, `TestOFT.${input.action}`, hash);
  const state = await readOFTPathwayState({
    publicClient: clients.publicClient,
    testOFT: input.testOFT,
    remoteEid: input.remoteEid,
    testOFTAbi: testOFTArtifact.abi,
  });
  const errors = validateOFTPathwayState(state, expected);
  console.log(
    jsonStringify({
      ok: errors.length === 0,
      action: input.action,
      state,
      errors,
    })
  );
  if (errors.length > 0) {
    throw new Error(
      `OFT pathway state check failed with ${errors.length} error(s)`
    );
  }
}
