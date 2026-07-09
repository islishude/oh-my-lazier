package e2ereplaycheck

import (
	"strings"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestNormalizeEvidenceSortsAndValidatesReplayHashes(t *testing.T) {
	evidence := validEvidence()
	evidence.ExpectedPackets = []ExpectedPacket{
		{
			GUID:          "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
			Nonce:         "2",
			SrcLogIndex:   8,
			PacketHeader:  packetHeaderHex(),
			PayloadHash:   "0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC",
			CommitTxHash:  "0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD",
			ReceiveTxHash: "0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE",
			VerifyTxHash:  "0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF",
		},
		validEvidence().ExpectedPackets[0],
	}

	normalized, err := NormalizeEvidence(evidence)
	if err != nil {
		t.Fatalf("NormalizeEvidence() error = %v", err)
	}
	if normalized.SourceTxHash != strings.ToLower(evidence.SourceTxHash) {
		t.Fatalf("SourceTxHash = %s, want lowercase", normalized.SourceTxHash)
	}
	if normalized.ExpectedPackets[0].SrcLogIndex != 4 || normalized.ExpectedPackets[1].SrcLogIndex != 8 {
		t.Fatalf("packets not sorted by source log index: %+v", normalized.ExpectedPackets)
	}

	evidence.ExpectedPackets[0].CommitTxHash = "0x1234"
	if _, err := NormalizeEvidence(evidence); err == nil || !strings.Contains(err.Error(), "commitTxHash") {
		t.Fatalf("NormalizeEvidence() error = %v, want commitTxHash validation", err)
	}
}

func TestValidateLocalE2EConfigGuardsDestructiveReplay(t *testing.T) {
	cfg := validLocalConfig()
	if err := ValidateLocalE2EConfig(cfg, validEvidence()); err != nil {
		t.Fatalf("ValidateLocalE2EConfig() error = %v", err)
	}

	remoteDB := cfg
	remoteDB.DatabaseURL = "postgres://laz_worker:laz_worker@example.com/laz_worker?sslmode=disable"
	if err := ValidateLocalE2EConfig(remoteDB, validEvidence()); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("ValidateLocalE2EConfig() error = %v, want loopback guard", err)
	}

	disabledPathway := cfg
	disabledPathway.Pathways[0].Enabled = false
	if err := ValidateLocalE2EConfig(disabledPathway, validEvidence()); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("ValidateLocalE2EConfig() error = %v, want disabled pathway guard", err)
	}
}

func TestCompareFinalRowsRequiresObservedHashesAndNoOutbox(t *testing.T) {
	evidence := validEvidence()
	packet := evidence.ExpectedPackets[0]
	row := FinalRow{
		GUID:          packet.GUID,
		PacketStatus:  string(packets.ExecutorDelivered),
		ExecutorState: string(packets.ExecutorDelivered),
		DVNState:      string(packets.DVNVerified),
		CommitTxHash:  packet.CommitTxHash,
		ReceiveTxHash: strings.TrimPrefix(packet.ReceiveTxHash, "0x"),
		VerifyTxHash:  packet.VerifyTxHash,
	}
	if err := CompareFinalRows(evidence, []FinalRow{row}); err != nil {
		t.Fatalf("CompareFinalRows() error = %v", err)
	}

	row.OutboxRows = 1
	if err := CompareFinalRows(evidence, []FinalRow{row}); err == nil || !strings.Contains(err.Error(), "tx_outbox") {
		t.Fatalf("CompareFinalRows() error = %v, want tx_outbox mismatch", err)
	}
	row.OutboxRows = 0
	row.CommitTxHash = "0x9999999999999999999999999999999999999999999999999999999999999999"
	if err := CompareFinalRows(evidence, []FinalRow{row}); err == nil || !strings.Contains(err.Error(), "commit_tx_hash") {
		t.Fatalf("CompareFinalRows() error = %v, want commit_tx_hash mismatch", err)
	}
}

func validEvidence() Evidence {
	return Evidence{
		SrcEID:       localChainAEID,
		DstEID:       localChainBEID,
		SourceTxHash: "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		ExpectedPackets: []ExpectedPacket{
			{
				GUID:          "0x1111111111111111111111111111111111111111111111111111111111111111",
				Nonce:         "1",
				SrcLogIndex:   4,
				PacketHeader:  packetHeaderHex(),
				PayloadHash:   "0x2222222222222222222222222222222222222222222222222222222222222222",
				CommitTxHash:  "0x3333333333333333333333333333333333333333333333333333333333333333",
				ReceiveTxHash: "0x4444444444444444444444444444444444444444444444444444444444444444",
				VerifyTxHash:  "0x5555555555555555555555555555555555555555555555555555555555555555",
			},
		},
	}
}

func validLocalConfig() config.Config {
	return config.Config{
		DatabaseURL: "postgres://laz_worker:laz_worker@127.0.0.1:55433/laz_worker?sslmode=disable",
		Chains: []config.ChainConfig{
			{
				EID:              localChainAEID,
				Name:             "local-anvil-a",
				Family:           config.ChainFamilyEVM,
				ChainID:          localChainAChainID,
				StartBlockNumber: 0,
				RPCURLs:          []string{"http://127.0.0.1:18545"},
			},
			{
				EID:              localChainBEID,
				Name:             "local-anvil-b",
				Family:           config.ChainFamilyEVM,
				ChainID:          localChainBChainID,
				StartBlockNumber: 0,
				RPCURLs:          []string{"http://127.0.0.1:18546"},
			},
		},
		Pathways: []config.PathwayConfig{
			{
				SrcEID:  localChainAEID,
				DstEID:  localChainBEID,
				Enabled: true,
			},
		},
	}
}

func packetHeaderHex() string {
	return "0x01" + strings.Repeat("00", packetV1HeaderLen-1)
}
