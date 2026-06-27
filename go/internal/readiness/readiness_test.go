package readiness

import (
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/db"
)

func TestEvaluateAcceptsCleanActiveState(t *testing.T) {
	report := Evaluate(db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true},
		},
		TxOutbox: []db.TxOutboxStat{
			{ChainEID: 40245, Status: db.TxStatusConfirmed, Count: 2},
		},
	})

	if !report.Ready {
		t.Fatalf("ready = false, issues = %+v", report.Issues)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("issues = %+v, want none", report.Issues)
	}
}

func TestEvaluateRejectsPausedActiveStateAndFailedOutbox(t *testing.T) {
	report := Evaluate(db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true, Paused: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true, Paused: true},
		},
		TxOutbox: []db.TxOutboxStat{
			{ChainEID: 40245, Status: db.TxStatusFailed, Count: 3},
		},
	})

	if report.Ready {
		t.Fatal("ready = true, want false")
	}
	wantCodes := []string{"chain_paused", "pathway_paused", "failed_outbox"}
	if len(report.Issues) != len(wantCodes) {
		t.Fatalf("issues = %+v, want %d issues", report.Issues, len(wantCodes))
	}
	for i, want := range wantCodes {
		if report.Issues[i].Code != want {
			t.Fatalf("issue[%d].code = %q, want %q", i, report.Issues[i].Code, want)
		}
	}
}

func TestEvaluateIgnoresDisabledState(t *testing.T) {
	report := Evaluate(db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "disabled-source", Enabled: false, Paused: true},
			{EID: 40245, Name: "active-destination", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true, Paused: true},
		},
		TxOutbox: []db.TxOutboxStat{
			{ChainEID: 40161, Status: db.TxStatusFailed, Count: 3},
		},
	})

	if !report.Ready {
		t.Fatalf("ready = false, issues = %+v", report.Issues)
	}
}
