package readiness

import (
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
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
			{ChainEID: 40161, Stream: dvnSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: dvnDestStream, LastBlock: 100},
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
			{ChainEID: 40245, Status: db.TxStatusFailed, RetryState: db.TxOutboxRetryStateExhausted, Count: 3},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: executorSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: executorDestStream, LastBlock: 100},
			{ChainEID: 40161, Stream: dvnSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: dvnDestStream, LastBlock: 100},
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

func TestEvaluateIgnoresRetryingFailedOutbox(t *testing.T) {
	report := Evaluate(db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true},
		},
		TxOutbox: []db.TxOutboxStat{
			{ChainEID: 40245, Status: db.TxStatusFailed, RetryState: db.TxOutboxRetryStateRetrying, Count: 2},
			{ChainEID: 40245, Status: db.TxStatusFailed, RetryState: db.TxOutboxRetryStateSuperseded, Count: 1},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: executorSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: executorDestStream, LastBlock: 100},
			{ChainEID: 40161, Stream: dvnSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: dvnDestStream, LastBlock: 100},
		},
	})

	if !report.Ready {
		t.Fatalf("ready = false, issues = %+v", report.Issues)
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
	if gotCodes["indexer_cursor_missing"] != 6 {
		t.Fatalf("missing cursor issues = %d, want 6; issues = %+v", gotCodes["indexer_cursor_missing"], report.Issues)
	}
	if gotCodes["indexer_cursor_unstarted"] != 1 {
		t.Fatalf("unstarted cursor issues = %d, want 1; issues = %+v", gotCodes["indexer_cursor_unstarted"], report.Issues)
	}
}

func TestEvaluateRejectsUnsafeDurableJobStates(t *testing.T) {
	report := Evaluate(db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true},
		},
		Packets: []db.PacketStat{
			{SrcEID: 40161, DstEID: 40245, Status: string(packets.ExecutorManualReview), Count: 2},
		},
		ExecutorJobs: []db.StatusStat{
			{Status: string(packets.ExecutorLzReceiveFailed), Count: 1},
			{Status: string(packets.ExecutorManualReview), Count: 1},
		},
		DVNJobs: []db.StatusStat{
			{Status: string(packets.DVNQuorumConflict), Count: 1},
			{Status: string(packets.DVNReorgDetected), Count: 1},
			{Status: string(packets.DVNManualReview), Count: 1},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: executorSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: executorDestStream, LastBlock: 100},
			{ChainEID: 40161, Stream: dvnSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: dvnDestStream, LastBlock: 100},
		},
	})

	if report.Ready {
		t.Fatal("ready = true, want false")
	}
	gotCodes := make(map[string]int)
	for _, issue := range report.Issues {
		gotCodes[issue.Code]++
	}
	for _, want := range []string{
		"packet_manual_review",
		"executor_lz_receive_failed",
		"executor_manual_review",
		"dvn_quorum_conflict",
		"dvn_reorg_detected",
		"dvn_manual_review",
	} {
		if gotCodes[want] != 1 {
			t.Fatalf("issue %q count = %d, want 1; issues = %+v", want, gotCodes[want], report.Issues)
		}
	}
}

func TestEvaluateWithServicesRequiresOnlyEnabledRoleState(t *testing.T) {
	snapshot := db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true},
		},
		ExecutorJobs: []db.StatusStat{
			{Status: string(packets.ExecutorLzReceiveFailed), Count: 1},
		},
		DVNJobs: []db.StatusStat{
			{Status: string(packets.DVNQuorumConflict), Count: 1},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: dvnSourceStream, LastBlock: 100},
			{ChainEID: 40245, Stream: dvnDestStream, LastBlock: 100},
		},
	}

	report := EvaluateWithServices(snapshot, Services{DVNEnabled: true})
	if report.Ready {
		t.Fatal("ready = true, want dvn conflict issue")
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "dvn_quorum_conflict" {
		t.Fatalf("issues = %+v, want only dvn conflict", report.Issues)
	}

	snapshot.DVNJobs = nil
	report = EvaluateWithServices(snapshot, Services{DVNEnabled: true})
	if !report.Ready {
		t.Fatalf("ready = false, issues = %+v", report.Issues)
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
			{ChainEID: 40161, Status: db.TxStatusFailed, RetryState: db.TxOutboxRetryStateExhausted, Count: 3},
		},
	})

	if !report.Ready {
		t.Fatalf("ready = false, issues = %+v", report.Issues)
	}
}
