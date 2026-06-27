import assert from "node:assert/strict";
import test from "node:test";
import {
  expectedStateForAction,
  validateOFTPathwayState,
  type OFTPathwayState,
} from "./oft-pathway-control.js";

test("expectedStateForAction maps drain to zero-capacity rate limit", () => {
  assert.deepEqual(expectedStateForAction("drain"), {
    outboundRateLimitConfig: { capacity: 0n, refillPerSecond: 0n },
  });
});

test("validateOFTPathwayState accepts matching pause and rate-limit state", () => {
  const state = baseState();
  state.sendPaused = true;
  state.outboundRateLimitConfigured = true;
  state.outboundRateLimitConfig = { capacity: 100n, refillPerSecond: 2n };

  assert.deepEqual(
    validateOFTPathwayState(state, {
      sendPaused: true,
      outboundRateLimitConfig: { capacity: 100n, refillPerSecond: 2n },
    }),
    [],
  );
});

test("validateOFTPathwayState reports pause and rate-limit mismatches", () => {
  const errors = validateOFTPathwayState(baseState(), {
    receivePaused: true,
    outboundRateLimitConfig: { capacity: 1n, refillPerSecond: 1n },
  });

  assert.equal(errors.length, 4);
  assert.match(errors[0], /receivePaused/);
  assert.match(errors[1], /not configured/);
  assert.match(errors[2], /capacity/);
  assert.match(errors[3], /refillPerSecond/);
});

test("expectedStateForAction requires rate-limit values for set-rate-limit", () => {
  assert.throws(
    () => expectedStateForAction("set-rate-limit"),
    /RATE_LIMIT_CAPACITY/,
  );
});

function baseState(): OFTPathwayState {
  return {
    chainId: 11155111,
    testOFT: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    remoteEid: 40245,
    sendPaused: false,
    receivePaused: false,
    outboundRateLimitConfigured: false,
    outboundRateLimitConfig: { capacity: 0n, refillPerSecond: 0n },
  };
}
