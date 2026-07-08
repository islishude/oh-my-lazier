import {
  DEPLOYMENTS_V2_URL,
  expectedLayerZeroChains,
  verifyLayerZeroAddresses,
  type DeploymentRecord,
} from "./lz-addresses.js";
import { jsonStringify } from "./lib.js";

const deployments = await fetchJSON<DeploymentRecord[]>(DEPLOYMENTS_V2_URL);

const errors = verifyLayerZeroAddresses({ deployments });
if (errors.length > 0) {
  console.error(jsonStringify({ ok: false, errors }));
  process.exit(1);
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
  }),
);

async function fetchJSON<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}
