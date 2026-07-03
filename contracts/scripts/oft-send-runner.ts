import { buildCanarySendParam } from "./oft-canary.js";
import {
  createClients,
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  loadArtifact,
  optionalAddress,
  optionalBigInt,
  optionalParam,
  waitForTx,
} from "./lib.js";
import type { Hex } from "viem";

const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);

export async function sendOFTFromEnv(label: string): Promise<void> {
  const { account, publicClient, walletClient } = createClients();
  const testOFT = envAddress("TEST_OFT");
  const recipient = envAddress("RECIPIENT");
  const dstEid = envUint32("DST_EID");
  const amountLD = envBigInt("AMOUNT_LD");
  const minAmountLD = optionalBigInt("MIN_AMOUNT_LD") ?? amountLD;
  const lzReceiveGas = optionalBigInt("LZ_RECEIVE_GAS");
  const extraOptions = optionalHex("EXTRA_OPTIONS");
  if (lzReceiveGas !== undefined && extraOptions !== undefined) {
    throw new Error("LZ_RECEIVE_GAS and EXTRA_OPTIONS must not both be set");
  }
  const refundAddress = optionalAddress("REFUND_ADDRESS") ?? account.address;

  const sendParam = buildCanarySendParam({
    dstEid,
    recipient,
    amountLD,
    minAmountLD,
    lzReceiveGas,
    extraOptions,
  });

  const fee = (await publicClient.readContract({
    address: testOFT,
    abi: testOFTArtifact.abi,
    functionName: "quoteSend",
    args: [sendParam, false],
  })) as { nativeFee: bigint; lzTokenFee: bigint };

  if (fee.lzTokenFee !== 0n) {
    throw new Error("OFT send only supports native-fee payment");
  }

  await waitForTx(
    publicClient,
    label,
    await walletClient.writeContract({
      address: testOFT,
      abi: testOFTArtifact.abi,
      functionName: "send",
      args: [sendParam, fee, refundAddress],
      value: fee.nativeFee,
      account,
      chain: null,
    }),
  );

  console.log(
    jsonStringify({
      chainId: Number(await publicClient.getChainId()),
      sender: account.address,
      testOFT,
      dstEid,
      recipient,
      amountLD,
      minAmountLD,
      refundAddress,
      fee,
      sendParam,
    }),
  );
}

function optionalHex(name: string): Hex | undefined {
  const value = optionalParam(name);
  if (value === undefined || value === "") {
    return undefined;
  }
  if (!/^0x(?:[0-9a-fA-F]{2})*$/.test(value)) {
    throw new Error(`${name} must be 0x-prefixed hex bytes`);
  }
  return value as Hex;
}
