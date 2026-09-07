package txretry

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
)

// RPCClient is the read-and-send boundary of operator rebroadcasts.
type RPCClient interface {
	txmgr.TransactionSender
	ChainID(context.Context) (*big.Int, error)
	Close()
}

// Dependencies supplies chain policy and an injectable RPC connection factory.
type Dependencies struct {
	Chains []config.ChainConfig
	Dial   func(context.Context, string) (RPCClient, error)
}

func runRebroadcast(ctx context.Context, store Store, o Options, deps Dependencies) (json.RawMessage, error) {
	a, err := store.GetRebroadcastAttempt(ctx, o.ID)
	if err != nil {
		return nil, errors.New("cannot read active transaction attempt")
	}
	var chainID uint64
	for _, chain := range deps.Chains {
		if chain.EID == a.ChainEID {
			chainID = chain.ChainID
			break
		}
	}
	if chainID == 0 {
		return nil, errors.New("outbox chain is missing from worker config")
	}
	dial := deps.Dial
	if dial == nil {
		dial = func(ctx context.Context, url string) (RPCClient, error) { return ethclient.DialContext(ctx, url) }
	}
	rpcCtx, cancel := context.WithTimeout(ctx, txmgr.DefaultSendTimeout)
	defer cancel()
	client, err := dial(rpcCtx, o.RPCURL)
	if err != nil {
		return nil, errors.New("RPC connection failed")
	}
	defer client.Close()
	actual, err := client.ChainID(rpcCtx)
	if err != nil {
		return nil, errors.New("RPC chain ID check failed")
	}
	if actual == nil || actual.Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return nil, errors.New("RPC chain ID does not match configured chain")
	}
	result, sendErr := txmgr.Rebroadcast(ctx, store, client, a, chainID)
	if result.SendClass == "" {
		return nil, sendErr
	}
	output, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return output, sendErr
}
