package chain

import (
	"fmt"
	"math/big"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
)

// Chain is a configured LayerZero endpoint chain plus its RPC quorum client.
type Chain struct {
	EID           uint32
	Name          string
	ChainID       *big.Int
	Confirmations uint64
	RPC           *rpcquorum.Client
}

// Registry indexes configured chains by endpoint ID.
type Registry struct {
	byEID map[uint32]Chain
}

// NewRegistry builds an endpoint-ID registry from validated chain configuration.
func NewRegistry(chains []config.ChainConfig) (*Registry, error) {
	registry := &Registry{byEID: make(map[uint32]Chain, len(chains))}
	for _, cfg := range chains {
		registry.byEID[cfg.EID] = Chain{
			EID:           cfg.EID,
			Name:          cfg.Name,
			ChainID:       big.NewInt(cfg.ChainID),
			Confirmations: cfg.Confirmations,
			RPC:           rpcquorum.New(cfg.Name, cfg.RPCURLs),
		}
	}
	return registry, nil
}

// Get returns the configured chain for an endpoint ID.
func (r *Registry) Get(eid uint32) (Chain, error) {
	chain, ok := r.byEID[eid]
	if !ok {
		return Chain{}, fmt.Errorf("unknown chain eid %d", eid)
	}
	return chain, nil
}

// All returns every configured chain.
func (r *Registry) All() []Chain {
	chains := make([]Chain, 0, len(r.byEID))
	for _, chain := range r.byEID {
		chains = append(chains, chain)
	}
	return chains
}
