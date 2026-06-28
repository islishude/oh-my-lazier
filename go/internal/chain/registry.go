package chain

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
)

// Chain is a configured LayerZero endpoint chain plus its RPC quorum client.
type Chain struct {
	EID              uint32
	Name             string
	ChainID          *big.Int
	EndpointAddress  common.Address
	Confirmations    uint64
	StartBlockNumber uint64
	Workers          WorkerContracts
	RPC              *rpcquorum.Client
}

// WorkerContracts identifies the self-hosted worker contracts deployed on one chain.
type WorkerContracts struct {
	OpenExecutor common.Address
	OpenDVN      common.Address
}

// Pathway is one configured source-to-destination OApp pathway.
type Pathway struct {
	SrcEID         uint32
	DstEID         uint32
	SrcOApp        common.Address
	DstOApp        common.Address
	SendLib        common.Address
	ReceiveLib     common.Address
	Enabled        bool
	MaxMessageSize uint64
}

// Registry indexes configured chains by endpoint ID.
type Registry struct {
	byEID    map[uint32]Chain
	pathways map[string]Pathway
}

// NewRegistry builds chain and pathway indexes from validated startup configuration.
func NewRegistry(chains []config.ChainConfig, pathways []config.PathwayConfig) (*Registry, error) {
	registry := &Registry{
		byEID:    make(map[uint32]Chain, len(chains)),
		pathways: make(map[string]Pathway, len(pathways)),
	}
	for _, cfg := range chains {
		registry.byEID[cfg.EID] = Chain{
			EID:              cfg.EID,
			Name:             cfg.Name,
			ChainID:          big.NewInt(cfg.ChainID),
			EndpointAddress:  common.HexToAddress(cfg.EndpointAddress),
			Confirmations:    cfg.Confirmations,
			StartBlockNumber: cfg.StartBlockNumber,
			Workers: WorkerContracts{
				OpenExecutor: common.HexToAddress(cfg.Workers.OpenExecutor),
				OpenDVN:      common.HexToAddress(cfg.Workers.OpenDVN),
			},
			RPC: rpcquorum.New(cfg.Name, cfg.RPCURLs),
		}
	}
	for _, cfg := range pathways {
		pathway := Pathway{
			SrcEID:         cfg.SrcEID,
			DstEID:         cfg.DstEID,
			SrcOApp:        common.HexToAddress(cfg.SrcOApp),
			DstOApp:        common.HexToAddress(cfg.DstOApp),
			SendLib:        common.HexToAddress(cfg.SendLib),
			ReceiveLib:     common.HexToAddress(cfg.ReceiveLib),
			Enabled:        cfg.Enabled,
			MaxMessageSize: cfg.MaxMessageSize,
		}
		registry.pathways[pathwayKey(pathway.SrcEID, pathway.DstEID, pathway.SrcOApp, pathway.DstOApp)] = pathway
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

// Pathway returns the configured pathway for a source/destination OApp pair.
func (r *Registry) Pathway(srcEID, dstEID uint32, srcOApp, dstOApp common.Address) (Pathway, error) {
	pathway, ok := r.pathways[pathwayKey(srcEID, dstEID, srcOApp, dstOApp)]
	if !ok {
		return Pathway{}, fmt.Errorf("unknown pathway %d -> %d %s -> %s", srcEID, dstEID, srcOApp, dstOApp)
	}
	return pathway, nil
}

// Pathways returns every configured pathway.
func (r *Registry) Pathways() []Pathway {
	pathways := make([]Pathway, 0, len(r.pathways))
	for _, pathway := range r.pathways {
		pathways = append(pathways, pathway)
	}
	return pathways
}

func pathwayKey(srcEID, dstEID uint32, srcOApp, dstOApp common.Address) string {
	return fmt.Sprintf("%d:%d:%s:%s", srcEID, dstEID, srcOApp, dstOApp)
}
