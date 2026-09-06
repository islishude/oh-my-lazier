package rpcquorum

import (
	"context"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// TransactionVisibility is one provider's non-authoritative mempool observation.
type TransactionVisibility struct {
	ProviderID string `json:"provider_id"`
	State      string `json:"state"`
}

// TransactionVisibility queries every configured provider. Errors never count
// as absence, and no observation establishes canonical inclusion or finality.
func (c *Client) TransactionVisibility(ctx context.Context, hash common.Hash) []TransactionVisibility {
	c.mu.Lock()
	n := len(c.providers)
	c.mu.Unlock()
	out := make([]TransactionVisibility, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			out[i] = TransactionVisibility{ProviderID: providerID(i), State: "unavailable"}
			probe, cancel := context.WithTimeout(ctx, c.probeTimeout)
			defer cancel()
			client, err := c.providerClient(probe, i)
			if err != nil {
				return
			}
			tx, pending, err := client.TransactionByHash(probe, hash)
			if errors.Is(err, ethereum.NotFound) {
				out[i].State = "absent"
				return
			}
			if err != nil || tx == nil || tx.Hash() != hash {
				return
			}
			out[i].State = "mined"
			if pending {
				out[i].State = "pending"
			}
		})
	}
	wg.Wait()
	return out
}
