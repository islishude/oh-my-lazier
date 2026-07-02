package readiness

import (
	"fmt"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

const (
	executorSourceStream = "executor_source"
	executorDestStream   = "executor_destination"
)

// Issue is one failed pre-migration readiness check.
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Report is the pre-migration readiness verdict derived from DB-backed state.
type Report struct {
	Ready  bool             `json:"ready"`
	Issues []Issue          `json:"issues"`
	Stats  db.StatsSnapshot `json:"stats"`
}

// Evaluate checks worker durable state against the mainnet readiness runbook gates.
func Evaluate(snapshot db.StatsSnapshot) Report {
	var issues []Issue
	activeChains := make(map[uint32]struct{})
	requiredCursors := make(map[uint32]map[string]struct{})
	for _, chain := range snapshot.Chains {
		if !chain.Enabled {
			continue
		}
		activeChains[chain.EID] = struct{}{}
		if chain.Paused {
			issues = append(issues, Issue{
				Code:    "chain_paused",
				Message: fmt.Sprintf("chain %d (%s) is paused", chain.EID, chain.Name),
			})
		}
	}
	for _, pathway := range snapshot.Pathways {
		if !pathway.Enabled {
			continue
		}
		if _, ok := activeChains[pathway.SrcEID]; !ok {
			continue
		}
		if _, ok := activeChains[pathway.DstEID]; !ok {
			continue
		}
		requireCursor(requiredCursors, pathway.SrcEID, executorSourceStream)
		requireCursor(requiredCursors, pathway.DstEID, executorDestStream)
		if pathway.Paused {
			issues = append(issues, Issue{
				Code:    "pathway_paused",
				Message: fmt.Sprintf("pathway %d -> %d is paused", pathway.SrcEID, pathway.DstEID),
			})
		}
	}
	for _, outbox := range snapshot.TxOutbox {
		if outbox.Status != db.TxStatusFailed || outbox.Count == 0 {
			continue
		}
		if outbox.RetryState != db.TxOutboxRetryStateExhausted {
			continue
		}
		if _, ok := activeChains[outbox.ChainEID]; !ok {
			continue
		}
		issues = append(issues, Issue{
			Code:    "failed_outbox",
			Message: fmt.Sprintf("chain %d has %d exhausted failed tx_outbox rows", outbox.ChainEID, outbox.Count),
		})
	}
	for _, packet := range snapshot.Packets {
		if packet.Status != string(packets.ExecutorManualReview) || packet.Count == 0 {
			continue
		}
		if _, ok := activeChains[packet.SrcEID]; !ok {
			continue
		}
		if _, ok := activeChains[packet.DstEID]; !ok {
			continue
		}
		issues = append(issues, Issue{
			Code:    "packet_manual_review",
			Message: fmt.Sprintf("pathway %d -> %d has %d packets requiring manual review", packet.SrcEID, packet.DstEID, packet.Count),
		})
	}
	for _, job := range snapshot.ExecutorJobs {
		if job.Count == 0 {
			continue
		}
		switch job.Status {
		case string(packets.ExecutorLzReceiveFailed):
			issues = append(issues, Issue{
				Code:    "executor_lz_receive_failed",
				Message: fmt.Sprintf("executor has %d failed lzReceive jobs", job.Count),
			})
		case string(packets.ExecutorManualReview):
			issues = append(issues, Issue{
				Code:    "executor_manual_review",
				Message: fmt.Sprintf("executor has %d jobs requiring manual review", job.Count),
			})
		}
	}
	for _, job := range snapshot.DVNJobs {
		if job.Count == 0 {
			continue
		}
		switch job.Status {
		case string(packets.DVNQuorumConflict):
			issues = append(issues, Issue{
				Code:    "dvn_quorum_conflict",
				Message: fmt.Sprintf("dvn has %d quorum conflict jobs", job.Count),
			})
		case string(packets.DVNReorgDetected):
			issues = append(issues, Issue{
				Code:    "dvn_reorg_detected",
				Message: fmt.Sprintf("dvn has %d jobs waiting for reorg rollback", job.Count),
			})
		case string(packets.DVNManualReview):
			issues = append(issues, Issue{
				Code:    "dvn_manual_review",
				Message: fmt.Sprintf("dvn has %d jobs requiring manual review", job.Count),
			})
		}
	}
	cursorProgress := make(map[uint32]map[string]uint64)
	for _, cursor := range snapshot.IndexerCursors {
		if _, ok := activeChains[cursor.ChainEID]; !ok {
			continue
		}
		if cursorProgress[cursor.ChainEID] == nil {
			cursorProgress[cursor.ChainEID] = make(map[string]uint64)
		}
		cursorProgress[cursor.ChainEID][cursor.Stream] = cursor.LastBlock
	}
	for chainEID, streams := range requiredCursors {
		for stream := range streams {
			lastBlock, ok := cursorProgress[chainEID][stream]
			if !ok {
				issues = append(issues, Issue{
					Code:    "indexer_cursor_missing",
					Message: fmt.Sprintf("chain %d is missing indexer cursor %q", chainEID, stream),
				})
				continue
			}
			if lastBlock == 0 {
				issues = append(issues, Issue{
					Code:    "indexer_cursor_unstarted",
					Message: fmt.Sprintf("chain %d indexer cursor %q has not advanced", chainEID, stream),
				})
			}
		}
	}
	return Report{Ready: len(issues) == 0, Issues: issues, Stats: snapshot}
}

func requireCursor(required map[uint32]map[string]struct{}, chainEID uint32, stream string) {
	if required[chainEID] == nil {
		required[chainEID] = make(map[string]struct{})
	}
	required[chainEID][stream] = struct{}{}
}
