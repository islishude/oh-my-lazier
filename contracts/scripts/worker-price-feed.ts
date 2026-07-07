import type { Address } from "viem";

export function shouldSetPriceFeed(currentPriceFeed: string, configuredPriceFeed: Address): boolean {
  return currentPriceFeed.toLowerCase() !== configuredPriceFeed.toLowerCase();
}
