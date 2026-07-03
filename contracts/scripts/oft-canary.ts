import { type Address, type Hex } from "viem";
import { addressToBytes32 } from "./lib.js";

const type3 = "0003";
const executorWorkerID = "01";
const lzReceiveOptionType = "01";
const lzReceivePayloadBytes = 16;

export type OFTSendParam = {
  dstEid: number;
  to: Hex;
  amountLD: bigint;
  minAmountLD: bigint;
  extraOptions: Hex;
  composeMsg: Hex;
  oftCmd: Hex;
};

export function buildCanarySendParam(input: {
  dstEid: number;
  recipient: Address;
  amountLD: bigint;
  minAmountLD: bigint;
  lzReceiveGas?: bigint;
  extraOptions?: Hex;
}): OFTSendParam {
  if (
    !Number.isInteger(input.dstEid) ||
    input.dstEid < 0 ||
    input.dstEid > 0xffffffff
  ) {
    throw new Error("dstEid must be a uint32");
  }
  if (input.amountLD <= 0n) {
    throw new Error("amountLD must be positive");
  }
  if (input.minAmountLD <= 0n || input.minAmountLD > input.amountLD) {
    throw new Error(
      "minAmountLD must be positive and no greater than amountLD",
    );
  }
  return {
    dstEid: input.dstEid,
    to: addressToBytes32(input.recipient),
    amountLD: input.amountLD,
    minAmountLD: input.minAmountLD,
    extraOptions:
      input.extraOptions ??
      (input.lzReceiveGas === undefined
        ? "0x"
        : buildLzReceiveOption(input.lzReceiveGas)),
    composeMsg: "0x",
    oftCmd: "0x",
  };
}

export function buildLzReceiveOption(gas: bigint): Hex {
  if (gas <= 0n || gas > (1n << 128n) - 1n) {
    throw new Error("lzReceiveGas must be a non-zero uint128");
  }
  const payload = uint128Hex(gas);
  const size = (1 + lzReceivePayloadBytes).toString(16).padStart(4, "0");
  return `0x${type3}${executorWorkerID}${size}${lzReceiveOptionType}${payload}` as Hex;
}

function uint128Hex(value: bigint): string {
  if (value < 0n || value > (1n << 128n) - 1n) {
    throw new Error("value must be a uint128");
  }
  return value.toString(16).padStart(32, "0");
}
