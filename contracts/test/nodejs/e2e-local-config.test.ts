import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  localE2EChains,
  localE2EDatabaseURL,
  localE2EKMS,
} from "../../scripts/e2e-local-config.js";

test("localE2EChains keeps Compose-compatible defaults", () => {
  const chains = localE2EChains();

  assert.deepEqual(
    chains.map((chain) => ({
      key: chain.key,
      hostRpcUrl: chain.hostRpcUrl,
      containerRpcUrl: chain.containerRpcUrl,
    })),
    [
      {
        key: "a",
        hostRpcUrl: "http://127.0.0.1:18545",
        containerRpcUrl: "http://anvil-a:8545",
      },
      {
        key: "b",
        hostRpcUrl: "http://127.0.0.1:18546",
        containerRpcUrl: "http://anvil-b:8545",
      },
    ]
  );
});

test("localE2EChains accepts CI host and container RPC overrides", () => {
  const chains = localE2EChains(
    resolver({
      E2E_CHAIN_A_HOST_RPC_URL: "http://127.0.0.1:28545",
      E2E_CHAIN_A_CONTAINER_RPC_URL: "http://127.0.0.1:28545",
      E2E_CHAIN_B_HOST_RPC_URL: "http://127.0.0.1:28546",
      E2E_CHAIN_B_CONTAINER_RPC_URL: "http://127.0.0.1:28546",
    })
  );

  assert.equal(chains[0]?.hostRpcUrl, "http://127.0.0.1:28545");
  assert.equal(chains[0]?.containerRpcUrl, "http://127.0.0.1:28545");
  assert.equal(chains[1]?.hostRpcUrl, "http://127.0.0.1:28546");
  assert.equal(chains[1]?.containerRpcUrl, "http://127.0.0.1:28546");
});

test("localE2EDatabaseURL keeps defaults and accepts overrides", () => {
  assert.equal(
    localE2EDatabaseURL("host"),
    "postgres://laz_worker:laz_worker@127.0.0.1:55433/laz_worker?sslmode=disable"
  );
  assert.equal(
    localE2EDatabaseURL("container"),
    "postgres://laz_worker:laz_worker@postgres:5432/laz_worker?sslmode=disable"
  );

  const resolve = resolver({
    E2E_HOST_DATABASE_URL: "postgres://host/worker",
    E2E_CONTAINER_DATABASE_URL: "postgres://container/worker",
  });
  assert.equal(localE2EDatabaseURL("host", resolve), "postgres://host/worker");
  assert.equal(
    localE2EDatabaseURL("container", resolve),
    "postgres://container/worker"
  );
});

test("localE2EKMS keeps Compose-compatible defaults and accepts overrides", () => {
  assert.deepEqual(localE2EKMS(), {
    hostEndpoint: "http://127.0.0.1:4566",
    containerEndpoint: "http://localstack:4566",
    region: "us-east-1",
  });

  assert.deepEqual(
    localE2EKMS(
      resolver({
        E2E_KMS_HOST_ENDPOINT: "http://127.0.0.1:14566",
        E2E_KMS_CONTAINER_ENDPOINT: "http://127.0.0.1:24566",
        E2E_KMS_REGION: "local-test-1",
      })
    ),
    {
      hostEndpoint: "http://127.0.0.1:14566",
      containerEndpoint: "http://127.0.0.1:24566",
      region: "local-test-1",
    }
  );
});

test("local E2E worker keeps keystore password overrideable", () => {
  const compose = readFileSync("docker-compose.e2e.yml", "utf8");
  assert.match(
    compose,
    /E2E_KEYSTORE_PASSWORD:\s*\$\{E2E_KEYSTORE_PASSWORD:-local-e2e-password\}/
  );

  const makefile = readFileSync("Makefile", "utf8");
  assert.match(
    makefile,
    /E2E_KEYSTORE_PASSWORD="\$\(E2E_KEYSTORE_PASSWORD\)" \$\(E2E_COMPOSE\) --profile worker up/
  );
});

test("local E2E worker config enables fast RBF replacement", () => {
  const deployScript = readFileSync(
    "contracts/scripts/e2e-local-deploy.ts",
    "utf8"
  );
  assert.match(
    deployScript,
    /tx_manager:\n  stale_broadcast_replacement_after_seconds: 2/
  );
  assert.match(deployScript, /max_priority_fee_per_gas_wei: "2000000000"/);
});

function resolver(overrides: Record<string, string>) {
  return (name: string, fallback: string) => overrides[name] ?? fallback;
}
