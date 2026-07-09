import { readFileSync } from "node:fs";
import { rollbackConfigPlan, type LzConfigSnapshot } from "./lz-config.js";
import {
  assertConfiguredChain,
  createClients,
  jsonStringify,
  loadABIArtifact,
  optionalBool,
  requiredEnv,
  waitForTx,
} from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
);

const snapshotPath = requiredEnv("LZ_CONFIG_SNAPSHOT");
const snapshot = JSON.parse(
  readFileSync(snapshotPath, "utf8"),
) as LzConfigSnapshot;
const plan = rollbackConfigPlan(snapshot);
if (optionalBool("DRY_RUN") === true) {
  console.log(jsonStringify({ dryRun: true, ...plan }));
  process.exit(0);
}

const { account, publicClient, walletClient } = createClients();
await assertConfiguredChain(publicClient);

for (const batch of plan.batches) {
  await waitForTx(
    publicClient,
    batch.label,
    await walletClient.writeContract({
      address: snapshot.endpoint,
      abi: endpointArtifact.abi,
      functionName: "setConfig",
      args: [snapshot.oapp, batch.library, batch.configs],
      account,
      chain: walletClient.chain,
    }),
  );
}

console.log(
  jsonStringify({
    dryRun: false,
    ...plan,
  }),
);
