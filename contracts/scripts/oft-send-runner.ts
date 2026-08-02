import { buildCanarySendParam } from "./oft-canary.js";
import { enrichKnownContractError } from "./contract-error.js";
import {
  jsonStringify,
  loadABIArtifact,
  loadArtifact,
  waitForTx,
  type ChainClients,
} from "./lib.js";
import type { Abi, Address, Hex } from "viem";

export type SendOFTInput = {
  testOFT: Address;
  recipient: Address;
  dstEid: number;
  amountLD: bigint;
  minAmountLD?: bigint;
  lzReceiveGas?: bigint;
  extraOptions?: Hex;
  refundAddress?: Address;
};

export function buildSendOFTPlan(input: SendOFTInput) {
  const minAmountLD = input.minAmountLD ?? input.amountLD;
  if (input.lzReceiveGas !== undefined && input.extraOptions !== undefined) {
    throw new Error(
      "input.lzReceiveGas and input.extraOptions must not both be set"
    );
  }
  return {
    testOFT: input.testOFT,
    dstEid: input.dstEid,
    recipient: input.recipient,
    amountLD: input.amountLD,
    minAmountLD,
    ...(input.refundAddress === undefined
      ? {}
      : { refundAddress: input.refundAddress }),
    sendParam: buildCanarySendParam({
      dstEid: input.dstEid,
      recipient: input.recipient,
      amountLD: input.amountLD,
      minAmountLD,
      lzReceiveGas: input.lzReceiveGas,
      extraOptions: input.extraOptions,
    }),
  };
}

export async function sendOFT(
  label: string,
  input: SendOFTInput,
  clients: ChainClients
): Promise<void> {
  const plan = buildSendOFTPlan(input);
  const refundAddress = input.refundAddress ?? clients.account.address;
  const testOFTArtifact = loadArtifact(
    "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json"
  );
  const workerErrorsArtifact = loadABIArtifact(
    "contracts/artifacts/contracts/contracts/common/WorkerErrors.sol/WorkerErrors.json"
  );
  const knownSendErrorsABI = [
    ...testOFTArtifact.abi,
    ...workerErrorsArtifact.abi,
  ] as Abi;

  const fee = (await withKnownContractErrors(
    "TestOFT.quoteSend",
    clients.publicClient.readContract({
      address: input.testOFT,
      abi: testOFTArtifact.abi,
      functionName: "quoteSend",
      args: [plan.sendParam, false],
    }),
    knownSendErrorsABI
  )) as { nativeFee: bigint; lzTokenFee: bigint };

  if (fee.lzTokenFee !== 0n) {
    throw new Error("OFT send only supports native-fee payment");
  }

  await waitForTx(
    clients.publicClient,
    label,
    await withKnownContractErrors(
      "TestOFT.send",
      clients.walletClient.writeContract({
        address: input.testOFT,
        abi: testOFTArtifact.abi,
        functionName: "send",
        args: [plan.sendParam, fee, refundAddress],
        value: fee.nativeFee,
        account: clients.account,
        chain: clients.walletClient.chain,
      }),
      knownSendErrorsABI
    )
  );

  console.log(
    jsonStringify({
      chainId: Number(await clients.publicClient.getChainId()),
      sender: clients.account.address,
      ...plan,
      refundAddress,
      fee,
    })
  );
}

async function withKnownContractErrors<T>(
  context: string,
  promise: Promise<T>,
  knownSendErrorsABI: Abi
): Promise<T> {
  try {
    return await promise;
  } catch (error) {
    throw (
      enrichKnownContractError({
        error,
        abi: knownSendErrorsABI,
        context,
      }) ?? error
    );
  }
}
