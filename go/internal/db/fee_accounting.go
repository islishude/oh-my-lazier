package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/bigutil"
)

const (
	workerRoleExecutor = "executor"
	workerRoleDVN      = "dvn"

	workerPurposeExecutorCommitVerification = "executor_commit_verification"
	workerPurposeExecutorLzReceive          = "executor_lz_receive"
	workerPurposeDVNVerify                  = "dvn_verify"
)

// UnpricedWorkerReceiptCost identifies a mined worker transaction whose destination gas cost still needs source-token pricing.
type UnpricedWorkerReceiptCost struct {
	ID            int64
	Role          string
	SrcEID        uint32
	DstEID        uint32
	ChainEID      uint32
	Purpose       string
	GUID          common.Hash
	GasCostDstWei *big.Int
}

// ListUnpricedWorkerReceiptCosts returns mined worker receipts missing source-token gas-cost pricing.
func (s *Store) ListUnpricedWorkerReceiptCosts(ctx context.Context, limit int) ([]UnpricedWorkerReceiptCost, error) {
	if limit <= 0 {
		return nil, errors.New("unpriced receipt cost limit must be positive")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			tx.id,
			CASE
				WHEN tx.purpose IN ($1, $2) THEN $4
				ELSE $5
			END AS role,
			p.src_eid,
			p.dst_eid,
			tx.chain_eid,
			tx.purpose,
			p.guid,
			tx.receipt_gas_cost_dst_wei::text
		FROM tx_outbox tx
		JOIN packets p ON p.guid = tx.guid
		WHERE tx.purpose IN ($1, $2, $3)
			AND tx.receipt_gas_cost_dst_wei IS NOT NULL
			AND tx.receipt_gas_cost_src_wei IS NULL
		ORDER BY tx.receipt_observed_at, tx.id
		LIMIT $6
	`, workerPurposeExecutorCommitVerification, workerPurposeExecutorLzReceive, workerPurposeDVNVerify, workerRoleExecutor, workerRoleDVN, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	costs := make([]UnpricedWorkerReceiptCost, 0)
	for rows.Next() {
		var row struct {
			id            int64
			role          string
			srcEID        uint32
			dstEID        uint32
			chainEID      uint32
			purpose       string
			guid          []byte
			gasCostDstWei string
		}
		if err := rows.Scan(&row.id, &row.role, &row.srcEID, &row.dstEID, &row.chainEID, &row.purpose, &row.guid, &row.gasCostDstWei); err != nil {
			return nil, err
		}
		if len(row.guid) != common.HashLength {
			return nil, fmt.Errorf("unpriced receipt guid has length %d", len(row.guid))
		}
		gasCostDstWei, err := bigutil.ParseRequiredDecimal("receipt_gas_cost_dst_wei", &row.gasCostDstWei)
		if err != nil {
			return nil, err
		}
		costs = append(costs, UnpricedWorkerReceiptCost{
			ID:            row.id,
			Role:          row.role,
			SrcEID:        row.srcEID,
			DstEID:        row.dstEID,
			ChainEID:      row.chainEID,
			Purpose:       row.purpose,
			GUID:          common.BytesToHash(row.guid),
			GasCostDstWei: gasCostDstWei,
		})
	}
	return costs, rows.Err()
}

func (s *Store) txReceiptGasCostStats(ctx context.Context) ([]TxReceiptGasCostStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chain_eid, purpose, sum(receipt_gas_cost_dst_wei)::text
		FROM tx_outbox
		WHERE receipt_gas_cost_dst_wei IS NOT NULL
		GROUP BY chain_eid, purpose
		ORDER BY chain_eid, purpose
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TxReceiptGasCostStat
	for rows.Next() {
		var stat TxReceiptGasCostStat
		if err := rows.Scan(&stat.ChainEID, &stat.Purpose, &stat.GasCostDstWei); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *Store) workerFeeStats(ctx context.Context) ([]WorkerFeeStat, error) {
	rows, err := s.pool.Query(ctx, `
		WITH job_fees AS (
			SELECT
				$4::text AS role,
				p.src_eid,
				p.dst_eid,
				p.guid,
				COALESCE(ej.assigned_fee, 0) AS revenue
			FROM executor_jobs ej
			JOIN packets p ON p.guid = ej.guid
			UNION ALL
			SELECT
				$5::text AS role,
				p.src_eid,
				p.dst_eid,
				p.guid,
				COALESCE(dj.assigned_fee, 0) AS revenue
			FROM dvn_jobs dj
			JOIN packets p ON p.guid = dj.guid
		),
		job_costs AS (
			SELECT
				CASE
					WHEN tx.purpose IN ($1, $2) THEN $4::text
					ELSE $5::text
				END AS role,
				p.src_eid,
				p.dst_eid,
				p.guid,
				COALESCE(sum(tx.receipt_gas_cost_src_wei) FILTER (WHERE tx.receipt_gas_cost_src_wei IS NOT NULL), 0) AS actual_cost,
				count(*) FILTER (
					WHERE tx.receipt_gas_cost_dst_wei IS NOT NULL
						AND tx.receipt_gas_cost_src_wei IS NULL
				)::bigint AS unpriced_receipts
			FROM tx_outbox tx
			JOIN packets p ON p.guid = tx.guid
			WHERE tx.purpose IN ($1, $2, $3)
				AND tx.receipt_gas_cost_dst_wei IS NOT NULL
			GROUP BY role, p.src_eid, p.dst_eid, p.guid
		),
		job_margins AS (
			SELECT
				jf.role,
				jf.src_eid,
				jf.dst_eid,
				jf.guid,
				jf.revenue,
				COALESCE(jc.actual_cost, 0) AS actual_cost,
				COALESCE(jc.unpriced_receipts, 0) AS unpriced_receipts
			FROM job_fees jf
			LEFT JOIN job_costs jc
				ON jc.role = jf.role
				AND jc.src_eid = jf.src_eid
				AND jc.dst_eid = jf.dst_eid
				AND jc.guid = jf.guid
		)
		SELECT
			role,
			src_eid,
			dst_eid,
			sum(revenue)::text,
			sum(actual_cost)::text,
			sum(revenue - actual_cost)::text,
			count(*) FILTER (WHERE actual_cost > revenue)::bigint,
			COALESCE(sum(unpriced_receipts), 0)::bigint
		FROM job_margins
		GROUP BY role, src_eid, dst_eid
		ORDER BY role, src_eid, dst_eid
	`, workerPurposeExecutorCommitVerification, workerPurposeExecutorLzReceive, workerPurposeDVNVerify, workerRoleExecutor, workerRoleDVN)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []WorkerFeeStat
	for rows.Next() {
		var stat WorkerFeeStat
		if err := rows.Scan(
			&stat.Role,
			&stat.SrcEID,
			&stat.DstEID,
			&stat.RevenueSrcWei,
			&stat.ActualGasCostSrcWei,
			&stat.GrossMarginSrcWei,
			&stat.NegativeMarginJobs,
			&stat.UnpricedReceipts,
		); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}
