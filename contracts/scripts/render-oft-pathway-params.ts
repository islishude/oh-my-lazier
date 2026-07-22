import type { Address } from "viem";
import { requireLayerZeroLabsDVNForLibraries } from "./lz-addresses.js";
import type {
  PriceSnapshotInput,
  WorkerFeeModelInput,
} from "./oft-pathway-ignition.js";
import {
  buildOAppEndpointConfigParameters,
  buildOpenWorkersPathwayConfigParameters,
} from "./oft-pathway-ignition.js";

export type RenderOftPathwayPriceSnapshotInput = Omit<
  PriceSnapshotInput,
  "updatedAt"
> & {
  updatedAt?: bigint;
};

export type RenderOftPathwayParamsInput = {
  oapp: Address;
  endpoint: Address;
  delegate?: Address;
  remoteEid: number;
  remoteOApp: Address;
  sendUln: Address;
  receiveUln: Address;
  openExecutor: Address;
  openDVN: Address;
  priceFeed: Address;
  bootstrapPriceSubmitter: Address;
  requiredDVNs?: Address[];
  includeLayerZeroLabsDVN?: boolean;
  confirmations: bigint;
  maxMessageSize: number;
  minLzReceiveGas?: bigint;
  maxLzReceiveGas: bigint;
  priceSnapshot: RenderOftPathwayPriceSnapshotInput;
  executorFeeModel: WorkerFeeModelInput;
  dvnFeeModel: WorkerFeeModelInput;
  dvnVerifier?: Address;
  enforcedLzReceiveGas: bigint;
};

export type RenderOftPathwayParamsOptions = {
  now?: bigint;
};

/** Build both Ignition parameter objects for one directional OFT pathway. */
export function renderOftPathwayParams(
  input: RenderOftPathwayParamsInput,
  options: RenderOftPathwayParamsOptions = {}
) {
  const requiredDVNs = resolveRequiredDVNs(input);
  const updatedAt =
    input.priceSnapshot.updatedAt ??
    options.now ??
    BigInt(Math.floor(Date.now() / 1000));
  const normalizedInput = {
    ...input,
    requiredDVNs,
    minLzReceiveGas: input.minLzReceiveGas ?? input.enforcedLzReceiveGas,
    priceSnapshot: { ...input.priceSnapshot, updatedAt },
  };

  return {
    ...buildOAppEndpointConfigParameters(normalizedInput),
    ...buildOpenWorkersPathwayConfigParameters(normalizedInput),
  };
}

export function resolveRequiredDVNs(input: {
  openDVN: Address;
  endpoint: Address;
  sendUln: Address;
  receiveUln: Address;
  requiredDVNs?: Address[];
  includeLayerZeroLabsDVN?: boolean;
}): Address[] {
  const dvns = input.requiredDVNs ?? [input.openDVN];
  if (input.includeLayerZeroLabsDVN !== true) {
    return dvns;
  }
  return [
    ...dvns,
    requireLayerZeroLabsDVNForLibraries(
      {
        endpointV2: input.endpoint,
        sendUln302: input.sendUln,
        receiveUln302: input.receiveUln,
      },
      "input.includeLayerZeroLabsDVN"
    ),
  ];
}
