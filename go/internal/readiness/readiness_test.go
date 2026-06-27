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
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: executorSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: executorDestStream, LastBlock: 100},
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
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: executorSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: executorDestStream, LastBlock: 100},
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

func TestEvaluateRejectsMissingOrUnstartedRequiredIndexerCursors(t *testing.T) {
	report := Evaluate(db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true},
			{SrcEID: 40245, DstEID: 40161, Enabled: true},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: executorSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: executorDestStream, LastBlock: 0},
		},
	})

	if report.Ready {
		t.Fatal("ready = true, want false")
	}
	gotCodes := make(map[string]int)
	for _, issue := range report.Issues {
		gotCodes[issue.Code]++
	}
	if gotCodes["indexer_cursor_missing"] != 2 {
		t.Fatalf("missing cursor issues = %d, want 2; issues = %+v", gotCodes["indexer_cursor_missing"], report.Issues)
	}
	if gotCodes["indexer_cursor_unstarted"] != 1 {
		t.Fatalf("unstarted cursor issues = %d, want 1; issues = %+v", gotCodes["indexer_cursor_unstarted"], report.Issues)
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
