// Package e2eindexcheck validates local E2E indexer evidence against Postgres.
package e2eindexcheck

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const expectedPacketCount = 2

var (
	hashPattern    = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
	decimalPattern = regexp.MustCompile(`^[0-9]+$`)
)

// Evidence is the transient local E2E artifact emitted by e2e-local-run.ts.
type Evidence struct {
	SrcEID          uint32           `json:"srcEid"`
	DstEID          uint32           `json:"dstEid"`
	SourceTxHash    string           `json:"sourceTxHash"`
	ExpectedPackets []ExpectedPacket `json:"expectedPackets"`
}

// ExpectedPacket identifies one PacketSent log expected for the batch source transaction.
type ExpectedPacket struct {
	GUID        string `json:"guid"`
	Nonce       string `json:"nonce"`
	SrcLogIndex uint   `json:"srcLogIndex"`
}

// PacketRow is the DB state relevant to one indexed source packet.
type PacketRow struct {
	GUID           string
	SrcEID         uint32
	DstEID         uint32
	Nonce          string
	SrcLogIndex    uint
	HasExecutorJob bool
	HasDVNJob      bool
}

// LoadEvidence reads and validates an E2E indexer evidence file.
func LoadEvidence(path string) (Evidence, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Evidence{}, err
	}
	var evidence Evidence
	if err := json.Unmarshal(body, &evidence); err != nil {
		return Evidence{}, err
	}
	if err := ValidateEvidence(evidence); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// ValidateEvidence checks the evidence file before any database polling.
func ValidateEvidence(evidence Evidence) error {
	if evidence.SrcEID == 0 || evidence.DstEID == 0 {
		return errors.New("source and destination EIDs are required")
	}
	if evidence.SrcEID == evidence.DstEID {
		return errors.New("source and destination EIDs must differ")
	}
	if !hashPattern.MatchString(evidence.SourceTxHash) {
		return errors.New("source transaction hash must be a 32-byte hex value")
	}
	if len(evidence.ExpectedPackets) != expectedPacketCount {
		return fmt.Errorf("expected %d packets, got %d", expectedPacketCount, len(evidence.ExpectedPackets))
	}

	guids := make(map[string]struct{}, expectedPacketCount)
	nonces := make(map[string]struct{}, expectedPacketCount)
	logIndexes := make(map[uint]struct{}, expectedPacketCount)
	for index, packet := range evidence.ExpectedPackets {
		guid := normalizeHash(packet.GUID)
		if !hashPattern.MatchString(guid) {
			return fmt.Errorf("expectedPackets[%d].guid must be a 32-byte hex value", index)
		}
		if !decimalPattern.MatchString(packet.Nonce) {
			return fmt.Errorf("expectedPackets[%d].nonce must be an unsigned decimal string", index)
		}
		if _, err := strconv.ParseUint(packet.Nonce, 10, 64); err != nil {
			return fmt.Errorf("expectedPackets[%d].nonce must fit uint64", index)
		}
		if _, ok := guids[guid]; ok {
			return fmt.Errorf("duplicate packet guid %s", guid)
		}
		if _, ok := nonces[packet.Nonce]; ok {
			return fmt.Errorf("duplicate packet nonce %s", packet.Nonce)
		}
		if _, ok := logIndexes[packet.SrcLogIndex]; ok {
			return fmt.Errorf("duplicate source log index %d", packet.SrcLogIndex)
		}
		guids[guid] = struct{}{}
		nonces[packet.Nonce] = struct{}{}
		logIndexes[packet.SrcLogIndex] = struct{}{}
		evidence.ExpectedPackets[index].GUID = guid
	}
	return nil
}

// Check polls Postgres until the evidence rows are indexed or the timeout expires.
func Check(ctx context.Context, databaseURL string, evidence Evidence, timeout time.Duration) error {
	if err := ValidateEvidence(evidence); err != nil {
		return err
	}
	if databaseURL == "" {
		return errors.New("database URL is required")
	}
	if timeout <= 0 {
		return errors.New("timeout must be positive")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		rows, err := loadRows(ctx, pool, evidence.SourceTxHash)
		if err != nil {
			lastErr = err
		} else if err := Compare(evidence, rows); err != nil {
			lastErr = err
		} else {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for multi-send indexer rows: %w", lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Compare validates loaded packet rows against the expected evidence.
func Compare(evidence Evidence, rows []PacketRow) error {
	if err := ValidateEvidence(evidence); err != nil {
		return err
	}
	if len(rows) != expectedPacketCount {
		return fmt.Errorf("indexed packet rows = %d, want %d", len(rows), expectedPacketCount)
	}
	expectedByGUID := make(map[string]ExpectedPacket, expectedPacketCount)
	for _, packet := range evidence.ExpectedPackets {
		expectedByGUID[normalizeHash(packet.GUID)] = packet
	}

	sort.Slice(rows, func(a, b int) bool {
		return rows[a].SrcLogIndex < rows[b].SrcLogIndex
	})
	for _, row := range rows {
		rowGUID := normalizeHash(row.GUID)
		expected, ok := expectedByGUID[rowGUID]
		if !ok {
			return fmt.Errorf("unexpected packet guid %s", rowGUID)
		}
		if row.SrcEID != evidence.SrcEID || row.DstEID != evidence.DstEID {
			return fmt.Errorf("packet %s route %d->%d does not match evidence %d->%d", rowGUID, row.SrcEID, row.DstEID, evidence.SrcEID, evidence.DstEID)
		}
		if row.Nonce != expected.Nonce {
			return fmt.Errorf("packet %s nonce %s does not match expected %s", rowGUID, row.Nonce, expected.Nonce)
		}
		if row.SrcLogIndex != expected.SrcLogIndex {
			return fmt.Errorf("packet %s source log index %d does not match expected %d", rowGUID, row.SrcLogIndex, expected.SrcLogIndex)
		}
		if !row.HasExecutorJob {
			return fmt.Errorf("packet %s missing executor job", rowGUID)
		}
		if !row.HasDVNJob {
			return fmt.Errorf("packet %s missing dvn job", rowGUID)
		}
	}
	return nil
}

func loadRows(ctx context.Context, pool *pgxpool.Pool, sourceTxHash string) ([]PacketRow, error) {
	txHash, err := hexBytes(sourceTxHash)
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT
			encode(p.guid, 'hex'),
			p.src_eid,
			p.dst_eid,
			p.nonce::text,
			p.src_log_index,
			ej.guid IS NOT NULL,
			dj.guid IS NOT NULL
		FROM packets p
		LEFT JOIN executor_jobs ej ON ej.guid = p.guid
		LEFT JOIN dvn_jobs dj ON dj.guid = p.guid
		WHERE p.src_tx_hash = $1
		ORDER BY p.src_log_index
	`, txHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PacketRow, 0, expectedPacketCount)
	for rows.Next() {
		var row PacketRow
		var srcEID int64
		var dstEID int64
		var srcLogIndex int64
		if err := rows.Scan(
			&row.GUID,
			&srcEID,
			&dstEID,
			&row.Nonce,
			&srcLogIndex,
			&row.HasExecutorJob,
			&row.HasDVNJob,
		); err != nil {
			return nil, err
		}
		if srcEID < 0 || dstEID < 0 || srcLogIndex < 0 {
			return nil, fmt.Errorf("indexed row has negative route or log index")
		}
		row.SrcEID = uint32(srcEID)
		row.DstEID = uint32(dstEID)
		row.SrcLogIndex = uint(srcLogIndex)
		row.GUID = "0x" + row.GUID
		out = append(out, row)
	}
	return out, rows.Err()
}

func hexBytes(value string) ([]byte, error) {
	normalized := strings.TrimPrefix(normalizeHash(value), "0x")
	return hex.DecodeString(normalized)
}

func normalizeHash(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "0X") {
		value = "0x" + value[2:]
	}
	if !strings.HasPrefix(value, "0x") {
		value = "0x" + value
	}
	return strings.ToLower(value)
}
