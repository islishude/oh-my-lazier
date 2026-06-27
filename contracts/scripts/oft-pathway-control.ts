import {
  createClients,
  createPublicClientFromEnv,
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  loadArtifact,
  requiredEnv,
  waitForTx,
} from "./lib.js";
import type { Abi, Address, PublicClient } from "viem";

export type OFTPathwayAction =
  | "inspect"
  | "pause-send"
  | "unpause-send"
  | "pause-receive"
  | "unpause-receive"
  | "drain"
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
  outboundRateLimitConfig?: RateLimitConfig;
};

export function expectedStateForAction(
  action: OFTPathwayAction,
  rateLimit?: RateLimitConfig,
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
        outboundRateLimitConfig: { capacity: 0n, refillPerSecond: 0n },
      };
    case "set-rate-limit":
      if (rateLimit === undefined) {
        throw new Error("RATE_LIMIT_CAPACITY and RATE_LIMIT_REFILL_PER_SECOND are required");
      }
      return { outboundRateLimitConfig: rateLimit };
  }
}

export function validateOFTPathwayState(
  state: OFTPathwayState,
  expected: ExpectedOFTPathwayState,
): string[] {
  const errors: string[] = [];
  if (
    expected.sendPaused !== undefined &&
    state.sendPaused !== expected.sendPaused
  ) {
    errors.push(
      `sendPaused is ${state.sendPaused}, expected ${expected.sendPaused}`,
    );
  }
  if (
    expected.receivePaused !== undefined &&
    state.receivePaused !== expected.receivePaused
  ) {
    errors.push(
      `receivePaused is ${state.receivePaused}, expected ${expected.receivePaused}`,
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
        `rate-limit capacity is ${state.outboundRateLimitConfig.capacity}, expected ${expected.outboundRateLimitConfig.capacity}`,
      );
    }
    if (
      state.outboundRateLimitConfig.refillPerSecond !==
      expected.outboundRateLimitConfig.refillPerSecond
    ) {
      errors.push(
        `rate-limit refillPerSecond is ${state.outboundRateLimitConfig.refillPerSecond}, expected ${expected.outboundRateLimitConfig.refillPerSecond}`,
      );
    }
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

function parseAction(): OFTPathwayAction {
  const action = requiredEnv("OFT_PATHWAY_ACTION");
  switch (action) {
    case "inspect":
    case "pause-send":
    case "unpause-send":
    case "pause-receive":
    case "unpause-receive":
    case "drain":
    case "set-rate-limit":
      return action;
    default:
      throw new Error(`unsupported OFT_PATHWAY_ACTION ${action}`);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const testOFTArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
  );
  const action = parseAction();
  const testOFT = envAddress("TEST_OFT");
  const remoteEid = envUint32("REMOTE_EID");
  const rateLimit =
    action === "set-rate-limit"
      ? {
          capacity: envBigInt("RATE_LIMIT_CAPACITY"),
          refillPerSecond: envBigInt("RATE_LIMIT_REFILL_PER_SECOND"),
        }
      : undefined;
  const expected = expectedStateForAction(action, rateLimit);

  if (action === "inspect") {
    const state = await readOFTPathwayState({
      publicClient: createPublicClientFromEnv(),
      testOFT,
      remoteEid,
      testOFTAbi: testOFTArtifact.abi,
    });
    console.log(jsonStringify({ ok: true, action, state }));
  } else {
    const { account, publicClient, walletClient } = createClients();
    let hash;
    if (action === "pause-send" || action === "unpause-send") {
      hash = await walletClient.writeContract({
        address: testOFT,
        abi: testOFTArtifact.abi,
        functionName: "pauseSend",
        args: [remoteEid, action === "pause-send"],
        account,
        chain: null,
      });
    } else if (action === "pause-receive" || action === "unpause-receive") {
      hash = await walletClient.writeContract({
        address: testOFT,
        abi: testOFTArtifact.abi,
        functionName: "pauseReceive",
        args: [remoteEid, action === "pause-receive"],
        account,
        chain: null,
      });
    } else {
      const config =
        action === "drain"
          ? { capacity: 0n, refillPerSecond: 0n }
          : rateLimit;
      if (config === undefined) {
        throw new Error("rate-limit config is required");
      }
      hash = await walletClient.writeContract({
        address: testOFT,
        abi: testOFTArtifact.abi,
        functionName: "setOutboundRateLimit",
        args: [remoteEid, config],
        account,
        chain: null,
      });
    }
    await waitForTx(publicClient, `TestOFT.${action}`, hash);
    const state = await readOFTPathwayState({
      publicClient,
      testOFT,
      remoteEid,
      testOFTAbi: testOFTArtifact.abi,
    });
    const errors = validateOFTPathwayState(state, expected);
    console.log(jsonStringify({ ok: errors.length === 0, action, state, errors }));
    if (errors.length > 0) {
      process.exitCode = 1;
    }
  }
}
