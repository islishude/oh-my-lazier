import { buildCanarySendParam } from "./oft-canary.js";
import {
  createClients,
  envAddress,
  envBigInt,
  envUint32,
  jsonStringify,
  loadArtifact,
  optionalBigInt,
  waitForTx,
} from "./lib.js";

const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);

const { account, publicClient, walletClient } = createClients();
const testOFT = envAddress("TEST_OFT");
const recipient = envAddress("RECIPIENT");
const dstEid = envUint32("DST_EID");
const amountLD = envBigInt("AMOUNT_LD");
const minAmountLD = optionalBigInt("MIN_AMOUNT_LD") ?? amountLD;
const lzReceiveGas = envBigInt("LZ_RECEIVE_GAS");
const refundAddress =
  process.env.REFUND_ADDRESS === undefined || process.env.REFUND_ADDRESS === ""
    ? account.address
    : envAddress("REFUND_ADDRESS");

const sendParam = buildCanarySendParam({
  dstEid,
  recipient,
  amountLD,
  minAmountLD,
  lzReceiveGas,
});

const fee = (await publicClient.readContract({
  address: testOFT,
  abi: testOFTArtifact.abi,
  functionName: "quoteSend",
  args: [sendParam, false],
})) as { nativeFee: bigint; lzTokenFee: bigint };

if (fee.lzTokenFee !== 0n) {
  throw new Error("canary only supports native-fee payment");
}

await waitForTx(
  publicClient,
  "TestOFT.send canary",
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
    lzReceiveGas,
    refundAddress,
    fee,
    sendParam,
  }),
);
