package rpcquorum

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type visibilityService struct {
	Tx   *types.Transaction
	Err  error
	Wait bool
}

func (s visibilityService) GetTransactionByHash(ctx context.Context, _ common.Hash) (any, error) {
	if s.Wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.Tx == nil {
		return nil, s.Err
	}
	return s.Tx, s.Err
}

func TestTransactionVisibilityValidatesHashAndSeparatesErrors(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := types.SignTx(types.NewTx(&types.LegacyTx{Nonce: 1, Gas: 21000, GasPrice: big.NewInt(1)}), types.LatestSignerForChainID(big.NewInt(1)), key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		svc  visibilityService
		hash common.Hash
		want string
	}{
		{"absent", visibilityService{}, tx.Hash(), "absent"},
		{"error", visibilityService{Err: errors.New("private backend error")}, tx.Hash(), "unavailable"},
		{"timeout", visibilityService{Wait: true}, tx.Hash(), "unavailable"},
		{"wrong hash", visibilityService{Tx: tx}, common.HexToHash("0x99"), "unavailable"},
		{"pending", visibilityService{Tx: tx}, tx.Hash(), "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := rpc.NewServer()
			if e := srv.RegisterName("eth", tc.svc); e != nil {
				t.Fatal(e)
			}
			defer srv.Stop()
			client := ethclient.NewClient(rpc.DialInProc(srv))
			defer client.Close()
			c := &Client{providers: []configuredProvider{{client: client}}, probeTimeout: 50 * time.Millisecond}
			r := c.TransactionVisibility(t.Context(), tc.hash)
			if len(r) != 1 || r[0].State != tc.want || r[0].ProviderID == "" {
				t.Fatalf("unexpected observation: %+v", r)
			}
		})
	}
}
