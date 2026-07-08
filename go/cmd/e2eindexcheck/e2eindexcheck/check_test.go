package e2eindexcheck

import (
	"strings"
	"testing"
)

func TestValidateEvidenceAcceptsTwoDistinctPackets(t *testing.T) {
	evidence := validEvidence()
	if err := ValidateEvidence(evidence); err != nil {
		t.Fatalf("ValidateEvidence() error = %v", err)
	}
}

func TestValidateEvidenceRejectsDuplicateGUID(t *testing.T) {
	evidence := validEvidence()
	evidence.ExpectedPackets[1].GUID = evidence.ExpectedPackets[0].GUID

	err := ValidateEvidence(evidence)
	if err == nil || !strings.Contains(err.Error(), "duplicate packet guid") {
		t.Fatalf("ValidateEvidence() error = %v, want duplicate packet guid", err)
	}
}

func TestCompareRequiresExecutorAndDVNJobs(t *testing.T) {
	evidence := validEvidence()
	rows := validRows()
	rows[1].HasDVNJob = false

	err := Compare(evidence, rows)
	if err == nil || !strings.Contains(err.Error(), "missing dvn job") {
		t.Fatalf("Compare() error = %v, want missing dvn job", err)
	}
}

func TestCompareRequiresExpectedPacketIdentity(t *testing.T) {
	evidence := validEvidence()
	rows := validRows()
	rows[0].Nonce = "99"

	err := Compare(evidence, rows)
	if err == nil || !strings.Contains(err.Error(), "does not match expected") {
		t.Fatalf("Compare() error = %v, want nonce mismatch", err)
	}
}

func validEvidence() Evidence {
	return Evidence{
		SrcEID:       90101,
		DstEID:       90102,
		SourceTxHash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedPackets: []ExpectedPacket{
			{
				GUID:        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Nonce:       "7",
				SrcLogIndex: 12,
			},
			{
				GUID:        "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				Nonce:       "8",
				SrcLogIndex: 16,
			},
		},
	}
}

func validRows() []PacketRow {
	return []PacketRow{
		{
			GUID:           "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SrcEID:         90101,
			DstEID:         90102,
			Nonce:          "7",
			SrcLogIndex:    12,
			HasExecutorJob: true,
			HasDVNJob:      true,
		},
		{
			GUID:           "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			SrcEID:         90101,
			DstEID:         90102,
			Nonce:          "8",
			SrcLogIndex:    16,
			HasExecutorJob: true,
			HasDVNJob:      true,
		},
	}
}
