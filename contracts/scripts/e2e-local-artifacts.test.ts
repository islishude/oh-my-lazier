import assert from "node:assert/strict";
import { test } from "node:test";
import {
  validateLocalE2EDeployment,
  validateLocalE2EGeneratedKMSKey,
} from "./e2e-local-artifacts.js";

test("validateLocalE2EDeployment accepts generated deployment shape", () => {
  const deployment = validDeployment();
  assert.equal(validateLocalE2EDeployment(deployment).chains.a.eid, 90101);
});

test("validateLocalE2EDeployment rejects invalid addresses", () => {
  const deployment = validDeployment();
  deployment.chains.a.primaryOpenDVN = "not-an-address";
  assert.throws(
    () => validateLocalE2EDeployment(deployment),
    /deployment\.chains\.a\.primaryOpenDVN must be an EVM address/,
  );
});

test("validateLocalE2EGeneratedKMSKey accepts helper output", () => {
  assert.equal(validateLocalE2EGeneratedKMSKey(validKMSKey()).keyId, "key-1");
});

test("validateLocalE2EGeneratedKMSKey rejects invalid address", () => {
  const key = validKMSKey();
  key.address = "not-an-address";
  assert.throws(
    () => validateLocalE2EGeneratedKMSKey(key),
    /kms\.address must be an EVM address/,
  );
});

function validDeployment() {
  return {
    generatedAt: "2026-07-03T00:00:00.000Z",
    deployer: "0x1111111111111111111111111111111111111111",
    worker: "0x2222222222222222222222222222222222222222",
    signers: {
      kms: {
        ...validKMSKey(),
        hostEndpoint: "http://127.0.0.1:4566",
        containerEndpoint: "http://localstack:4566",
      },
      keystore: {
        address: "0x2222222222222222222222222222222222222222",
      },
    },
    parameters: {
      confirmations: "1",
      maxMessageSize: 10000,
      minLzReceiveGas: "100000",
      lzReceiveGas: "250000",
      maxLzReceiveGas: "1000000",
    },
    chains: {
      a: validChain("a", 90101, 31337),
      b: validChain("b", 90102, 31338),
    },
  };
}

function validChain(key: "a" | "b", eid: number, chainId: number) {
  return {
    key,
    name: `local-anvil-${key}`,
    eid,
    chainId,
    hostRpcUrl: "http://127.0.0.1:18545",
    containerRpcUrl: "http://anvil-a:8545",
    endpoint: "0x3333333333333333333333333333333333333333",
    sendUln: "0x4444444444444444444444444444444444444444",
    receiveUln: "0x5555555555555555555555555555555555555555",
    oft: "0x6666666666666666666666666666666666666666",
    openExecutor: "0x7777777777777777777777777777777777777777",
    primaryOpenDVN: "0x8888888888888888888888888888888888888888",
    secondaryOpenDVN: "0x9999999999999999999999999999999999999999",
    executorSigner:
      key === "a"
        ? "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        : "0x2222222222222222222222222222222222222222",
    dvnSigner:
      key === "a"
        ? "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        : "0x2222222222222222222222222222222222222222",
  };
}

function validKMSKey() {
  return {
    keyId: "key-1",
    region: "us-east-1",
    address: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  };
}
