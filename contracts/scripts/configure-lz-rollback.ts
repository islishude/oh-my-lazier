import { readFileSync } from "node:fs";
import { rollbackConfigPlan, type LzConfigSnapshot } from "./lz-config.js";
import {
  jsonStringify,
  loadABIArtifact,
  waitForTx,
  type ChainClients,
} from "./lib.js";

const endpointArtifact = loadABIArtifact(
  "node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json"
);

export function readLzRollbackPlan(snapshotPath: string) {
  const snapshot = JSON.parse(
    readFileSync(snapshotPath, "utf8")
  ) as LzConfigSnapshot;
  return { snapshot, plan: rollbackConfigPlan(snapshot) };
}

export function printLzRollbackPlan(snapshotPath: string): void {
  const { plan } = readLzRollbackPlan(snapshotPath);
  console.log(jsonStringify({ dryRun: true, ...plan }));
}

export async function applyLzRollback(
  snapshotPath: string,
  clients: ChainClients
): Promise<void> {
  const { snapshot, plan } = readLzRollbackPlan(snapshotPath);
  for (const batch of plan.batches) {
    await waitForTx(
      clients.publicClient,
      batch.label,
      await clients.walletClient.writeContract({
        address: snapshot.endpoint,
        abi: endpointArtifact.abi,
        functionName: "setConfig",
        args: [snapshot.oapp, batch.library, batch.configs],
        account: clients.account,
        chain: clients.walletClient.chain,
      })
    );
  }

  console.log(jsonStringify({ dryRun: false, ...plan }));
}
