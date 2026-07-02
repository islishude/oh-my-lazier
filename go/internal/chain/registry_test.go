package chain

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestRegistryIndexesChainsAndPathways(t *testing.T) {
	registry, err := NewRegistry(testChains(), testPathways())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	ethereum, err := registry.Get(40161)
	if err != nil {
		t.Fatalf("Get(40161) error = %v", err)
	}
	if ethereum.EndpointAddress != common.HexToAddress("0x1111111111111111111111111111111111111111") {
		t.Fatalf("endpoint address = %s", ethereum.EndpointAddress)
	}
	if ethereum.TxRoles.Executor.SignerID != "0x9999999999999999999999999999999999999999" {
		t.Fatalf("executor signer = %s", ethereum.TxRoles.Executor.SignerID)
	}
	if ethereum.StartBlockNumber != 12345 {
		t.Fatalf("StartBlockNumber = %d, want 12345", ethereum.StartBlockNumber)
	}
	if ethereum.IndexerQueryBlockRange != 250 {
		t.Fatalf("IndexerQueryBlockRange = %d, want 250", ethereum.IndexerQueryBlockRange)
	}
	if ethereum.TxType != config.TxTypeDynamicFee {
		t.Fatalf("TxType = %q, want %q", ethereum.TxType, config.TxTypeDynamicFee)
	}
	base, err := registry.Get(40245)
	if err != nil {
		t.Fatalf("Get(40245) error = %v", err)
	}
	if base.TxType != config.TxTypeLegacy {
		t.Fatalf("base TxType = %q, want %q", base.TxType, config.TxTypeLegacy)
	}

	pathway, err := registry.Pathway(
		40161,
		40245,
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
		common.HexToAddress("0x8888888888888888888888888888888888888888"),
	)
	if err != nil {
		t.Fatalf("Pathway() error = %v", err)
	}
	if !pathway.Enabled {
		t.Fatal("pathway.Enabled = false")
	}
	if pathway.MaxMessageSize != 10000 {
		t.Fatalf("MaxMessageSize = %d", pathway.MaxMessageSize)
	}
	if pathway.SourceWorkers.OpenExecutor != common.HexToAddress("0x2222222222222222222222222222222222222222") {
		t.Fatalf("pathway open executor address = %s", pathway.SourceWorkers.OpenExecutor)
	}
	if pathway.DVNMode != config.DVNModeShadow {
		t.Fatalf("pathway dvn mode = %q", pathway.DVNMode)
	}
}

func TestRegistryRejectsUnknownPathway(t *testing.T) {
	registry, err := NewRegistry(testChains(), testPathways())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	_, err = registry.Pathway(
		40161,
		40245,
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		common.HexToAddress("0x8888888888888888888888888888888888888888"),
	)
	if err == nil {
		t.Fatal("Pathway() error = nil, want unknown pathway error")
	}
}

func testChains() []config.ChainConfig {
	return []config.ChainConfig{
		{
			EID:                    40161,
			Name:                   "ethereum-sepolia",
			Family:                 config.ChainFamilyEVM,
			ChainID:                11155111,
			EndpointAddress:        config.MustEVMAddress("0x1111111111111111111111111111111111111111"),
			Confirmations:          12,
			StartBlockNumber:       12345,
			IndexerQueryBlockRange: 250,
			RPCURLs:                []string{"http://localhost:8545"},
			TxRoles: config.ChainTxRolesConfig{
				Executor: config.ExecutorTxRoleConfig{Signer: config.MustEVMAddress("0x9999999999999999999999999999999999999999")},
				DVN: config.DVNTxRoleConfig{
					Signer:                  config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
					TxGasLimit:              120000,
					MaxFeePerGasWei:         "2000000000",
					MaxPriorityFeePerGasWei: "1000000000",
				},
			},
		},
		{
			EID:             40245,
			Name:            "base-sepolia",
			Family:          config.ChainFamilyEVM,
			ChainID:         84532,
			TxType:          config.TxTypeLegacy,
			EndpointAddress: config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8546"},
			TxRoles: config.ChainTxRolesConfig{
				Executor: config.ExecutorTxRoleConfig{Signer: config.MustEVMAddress("0x9999999999999999999999999999999999999999")},
				DVN: config.DVNTxRoleConfig{
					Signer:                  config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
					TxGasLimit:              120000,
					MaxFeePerGasWei:         "2000000000",
					MaxPriorityFeePerGasWei: "1000000000",
				},
			},
		},
	}
}

func testPathways() []config.PathwayConfig {
	return []config.PathwayConfig{
		{
			SrcEID:     40161,
			DstEID:     40245,
			SrcOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
			DstOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
			SendLib:    config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
			ReceiveLib: config.MustEVMAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			SourceWorkers: config.WorkerContractsConfig{
				OpenExecutor: config.MustEVMAddress("0x2222222222222222222222222222222222222222"),
				OpenDVN:      config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
			},
			DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
			Enabled:        true,
			MaxMessageSize: 10000,
		},
	}
}
