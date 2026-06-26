import {
  createClients,
  envAddress,
  envBigInt,
  loadArtifact,
  requiredEnv,
  waitForContract,
} from "./lib.js";

const testOFTArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/oft/TestOFT.sol/TestOFT.json",
);
const openExecutorArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenExecutor.sol/OpenExecutor.json",
);
const openDVNArtifact = loadArtifact(
  "contracts/artifacts/contracts/contracts/workers/OpenDVN.sol/OpenDVN.json",
);

const { account, publicClient, walletClient } = createClients();

const endpoint = envAddress("ENDPOINT");
const owner = envAddress("OWNER");
const tokenName = requiredEnv("TOKEN_NAME");
const tokenSymbol = requiredEnv("TOKEN_SYMBOL");
const initialRecipient = envAddress("INITIAL_RECIPIENT");
const initialSupply = envBigInt("INITIAL_SUPPLY");

const testOFTHash = await walletClient.deployContract({
  abi: testOFTArtifact.abi,
  bytecode: testOFTArtifact.bytecode,
  args: [
    tokenName,
    tokenSymbol,
    endpoint,
    owner,
    initialRecipient,
    initialSupply,
  ],
  account,
  chain: null,
});
const testOFT = await waitForContract(publicClient, testOFTHash);

const openExecutorHash = await walletClient.deployContract({
  abi: openExecutorArtifact.abi,
  bytecode: openExecutorArtifact.bytecode,
  args: [owner],
  account,
  chain: null,
});
const openExecutor = await waitForContract(publicClient, openExecutorHash);

const openDVNHash = await walletClient.deployContract({
  abi: openDVNArtifact.abi,
  bytecode: openDVNArtifact.bytecode,
  args: [owner],
  account,
  chain: null,
});
const openDVN = await waitForContract(publicClient, openDVNHash);

console.log(
  JSON.stringify(
    {
      chainId: Number(await publicClient.getChainId()),
      deployer: account.address,
      testOFT,
      openExecutor,
      openDVN,
    },
    null,
    2,
  ),
);
