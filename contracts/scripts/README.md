# Contract Scripts

These scripts use the compiled Hardhat artifacts and `viem`. They require `npm run compile` before execution and read all network-specific values from environment variables.

Deploy the local pathway contracts:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
ENDPOINT=... \
OWNER=... \
TOKEN_NAME=TestOFT \
TOKEN_SYMBOL=TOFT \
INITIAL_RECIPIENT=... \
INITIAL_SUPPLY=0 \
npm run deploy:workers
```

Configure one local direction after both sides are deployed:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
TEST_OFT=... \
OPEN_EXECUTOR=... \
OPEN_DVN=... \
REMOTE_EID=40245 \
REMOTE_OFT=... \
SEND_LIB=... \
MAX_MESSAGE_SIZE=10000 \
MIN_LZ_RECEIVE_GAS=200000 \
MAX_LZ_RECEIVE_GAS=1000000 \
PRICE_BASE_FEE=0 \
PRICE_DST_GAS_PRICE_IN_SRC_TOKEN=1 \
PRICE_BUFFER_BPS=1000 \
PRICE_STALE_AFTER=3600 \
npm run configure:workers
```

`SRC_OAPP` defaults to `TEST_OFT`. Set `RATE_LIMIT_CAPACITY` and `RATE_LIMIT_REFILL_PER_SECOND` together to configure outbound rate limiting.

Inspect the current LayerZero libraries and message-lib configs for one local direction:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
RECEIVE_ULN=... \
npm run inspect:lz-config
```

Switch the source send library executor config to OpenExecutor:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
OPEN_EXECUTOR=... \
EXECUTOR_MAX_MESSAGE_SIZE=10000 \
npm run configure:lz-executor
```

Configure ULN302 on one local chain so OpenDVN and the LayerZero Labs DVN are both required:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
RECEIVE_ULN=... \
OPEN_DVN=... \
LAYERZERO_LABS_DVN=... \
CONFIRMATIONS=12 \
npm run configure:lz-dvn
```

Run the LayerZero config scripts on both chains with the local endpoint, local OApp, local message libraries, and local DVN addresses for each direction. `configure:lz-dvn` explicitly sets `optionalDVNCount` to LayerZero's NIL value so default optional DVNs are not inherited during the first-phase required-DVN migration.
