import {
  DEPLOYMENTS_V2_URL,
  expectedLayerZeroChains,
  verifyLayerZeroAddresses,
  type DeploymentRecord,
} from "./lz-addresses.js";
import { jsonStringify } from "./lib.js";
import { CommandOutputError } from "./command-harness.js";

export async function checkLayerZeroAddresses(): Promise<void> {
  const deployments = await fetchJSON<DeploymentRecord[]>(DEPLOYMENTS_V2_URL);
  const errors = verifyLayerZeroAddresses({ deployments });
  if (errors.length > 0) {
    throw new CommandOutputError(jsonStringify({ ok: false, errors }));
  }

  console.log(
    jsonStringify({
      ok: true,
      sources: [DEPLOYMENTS_V2_URL],
      chains: expectedLayerZeroChains.map((chain) => ({
        chainKey: chain.chainKey,
        eid: chain.eid,
        nativeChainId: chain.nativeChainId,
      })),
    })
  );
}

async function fetchJSON<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}
