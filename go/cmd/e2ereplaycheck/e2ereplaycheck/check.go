// Package e2ereplaycheck validates local E2E destination-chain replay after a database rebuild.
package e2ereplaycheck

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/indexer"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	localChainAEID     = 90101
	localChainAChainID = 31337
	localChainBEID     = 90102
	localChainBChainID = 31338
	packetV1HeaderLen  = 81
)

var (
	hashPattern    = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
	hexPattern     = regexp.MustCompile(`^0x(?:[0-9a-fA-F]{2})+$`)
	decimalPattern = regexp.MustCompile(`^[0-9]+$`)
)

// Evidence is the transient local E2E artifact emitted by e2e-local-run.ts.
type Evidence struct {
	SrcEID          uint32           `json:"srcEid"`
	DstEID          uint32           `json:"dstEid"`
	SourceTxHash    string           `json:"sourceTxHash"`
	ExpectedPackets []ExpectedPacket `json:"expectedPackets"`
}

// ExpectedPacket identifies one delivered packet and its chain-observed destination transactions.
type ExpectedPacket struct {
	GUID          string `json:"guid"`
	Nonce         string `json:"nonce"`
	SrcLogIndex   uint   `json:"srcLogIndex"`
	PacketHeader  string `json:"packetHeader"`
	PayloadHash   string `json:"payloadHash"`
	CommitTxHash  string `json:"commitTxHash"`
	ReceiveTxHash string `json:"receiveTxHash"`
	VerifyTxHash  string `json:"verifyTxHash"`
}

// FinalRow is the database state that must converge from destination replay.
type FinalRow struct {
	GUID          string
	PacketStatus  string
	ExecutorState string
	DVNState      string
	CommitTxHash  string
	ReceiveTxHash string
	VerifyTxHash  string
	OutboxRows    int64
}

// LoadEvidence reads and validates a destination replay evidence file.
func LoadEvidence(path string) (Evidence, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Evidence{}, err
	}
	var evidence Evidence
	if err := json.Unmarshal(body, &evidence); err != nil {
		return Evidence{}, err
	}
	return NormalizeEvidence(evidence)
}

// NormalizeEvidence validates evidence and normalizes hashes to lower-case 0x form.
func NormalizeEvidence(evidence Evidence) (Evidence, error) {
	if evidence.SrcEID == 0 || evidence.DstEID == 0 {
		return Evidence{}, errors.New("source and destination EIDs are required")
	}
	if evidence.SrcEID == evidence.DstEID {
		return Evidence{}, errors.New("source and destination EIDs must differ")
	}
	sourceTxHash, err := normalizeHash(evidence.SourceTxHash, "sourceTxHash")
	if err != nil {
		return Evidence{}, err
	}
	evidence.SourceTxHash = sourceTxHash
	if len(evidence.ExpectedPackets) == 0 {
		return Evidence{}, errors.New("expectedPackets must contain at least one packet")
	}

	guids := make(map[string]struct{}, len(evidence.ExpectedPackets))
	nonces := make(map[string]struct{}, len(evidence.ExpectedPackets))
	logIndexes := make(map[uint]struct{}, len(evidence.ExpectedPackets))
	for index, packet := range evidence.ExpectedPackets {
		guid, err := normalizeHash(packet.GUID, fmt.Sprintf("expectedPackets[%d].guid", index))
		if err != nil {
			return Evidence{}, err
		}
		if !decimalPattern.MatchString(packet.Nonce) {
			return Evidence{}, fmt.Errorf("expectedPackets[%d].nonce must be an unsigned decimal string", index)
		}
		if _, err := strconv.ParseUint(packet.Nonce, 10, 64); err != nil {
			return Evidence{}, fmt.Errorf("expectedPackets[%d].nonce must fit uint64", index)
		}
		if !hexPattern.MatchString(packet.PacketHeader) || hexByteLen(packet.PacketHeader) != packetV1HeaderLen {
			return Evidence{}, fmt.Errorf("expectedPackets[%d].packetHeader must be an %d-byte PacketV1 header", index, packetV1HeaderLen)
		}
		payloadHash, err := normalizeHash(packet.PayloadHash, fmt.Sprintf("expectedPackets[%d].payloadHash", index))
		if err != nil {
			return Evidence{}, err
		}
		commitTxHash, err := normalizeHash(packet.CommitTxHash, fmt.Sprintf("expectedPackets[%d].commitTxHash", index))
		if err != nil {
			return Evidence{}, err
		}
		receiveTxHash, err := normalizeHash(packet.ReceiveTxHash, fmt.Sprintf("expectedPackets[%d].receiveTxHash", index))
		if err != nil {
			return Evidence{}, err
		}
		verifyTxHash, err := normalizeHash(packet.VerifyTxHash, fmt.Sprintf("expectedPackets[%d].verifyTxHash", index))
		if err != nil {
			return Evidence{}, err
		}
		if _, ok := guids[guid]; ok {
			return Evidence{}, fmt.Errorf("duplicate packet guid %s", guid)
		}
		if _, ok := nonces[packet.Nonce]; ok {
			return Evidence{}, fmt.Errorf("duplicate packet nonce %s", packet.Nonce)
		}
		if _, ok := logIndexes[packet.SrcLogIndex]; ok {
			return Evidence{}, fmt.Errorf("duplicate source log index %d", packet.SrcLogIndex)
		}
		guids[guid] = struct{}{}
		nonces[packet.Nonce] = struct{}{}
		logIndexes[packet.SrcLogIndex] = struct{}{}
		packet.GUID = guid
		packet.PacketHeader = strings.ToLower(packet.PacketHeader)
		packet.PayloadHash = payloadHash
		packet.CommitTxHash = commitTxHash
		packet.ReceiveTxHash = receiveTxHash
		packet.VerifyTxHash = verifyTxHash
		evidence.ExpectedPackets[index] = packet
	}
	sort.Slice(evidence.ExpectedPackets, func(a, b int) bool {
		return evidence.ExpectedPackets[a].SrcLogIndex < evidence.ExpectedPackets[b].SrcLogIndex
	})
	return evidence, nil
}

// Check rebuilds the local E2E database and proves destination replay convergence.
func Check(ctx context.Context, cfg config.Config, evidence Evidence, timeout time.Duration) error {
	evidence, err := NormalizeEvidence(evidence)
	if err != nil {
		return err
	}
	if timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if err := ValidateLocalE2EConfig(cfg, evidence); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	registry, err := chain.NewRegistry(cfg.Chains, cfg.Pathways)
	if err != nil {
		return err
	}
	defer registry.Close()
	if err := validateLocalE2ERPCChainIDs(ctx, registry); err != nil {
		return err
	}
	pathways := registry.Pathways()
	sourceChain, err := registry.Get(evidence.SrcEID)
	if err != nil {
		return err
	}
	destinationChain, err := registry.Get(evidence.DstEID)
	if err != nil {
		return err
	}

	if err := resetDatabase(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	if err := store.SyncConfig(ctx, registry); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	destinationOnly := indexer.StreamSet{ExecutorDestination: true, DVNDestination: true}
	sourceOnly := indexer.StreamSet{ExecutorSource: true, DVNSource: true}

	destinationIndexer := indexer.New(destinationChain, pathways, store, logger).WithStreams(destinationOnly)
	if err := waitForDestinationDeferred(ctx, store, destinationIndexer, evidence.DstEID); err != nil {
		return err
	}
	if rows, err := indexedPacketCount(ctx, pool, evidence); err != nil {
		return err
	} else if rows != 0 {
		return fmt.Errorf("destination-only replay created %d packet rows before source replay, want 0", rows)
	}

	sourceIndexer := indexer.New(sourceChain, pathways, store, logger).WithStreams(sourceOnly)
	if err := waitForSourceRows(ctx, pool, sourceIndexer, evidence); err != nil {
		return err
	}

	finalDestinationIndexer := indexer.New(destinationChain, pathways, store, logger).WithStreams(destinationOnly)
	return waitForFinalRows(ctx, pool, finalDestinationIndexer, evidence)
}

// ValidateLocalE2EConfig prevents the destructive replay probe from running outside local E2E.
func ValidateLocalE2EConfig(cfg config.Config, evidence Evidence) error {
	if _, err := NormalizeEvidence(evidence); err != nil {
		return err
	}
	if err := validateLocalDatabaseURL(cfg.DatabaseURL); err != nil {
		return err
	}
	if len(cfg.Chains) != 2 {
		return fmt.Errorf("local replay check requires exactly 2 chains, got %d", len(cfg.Chains))
	}
	expectedChains := map[uint32]uint64{
		localChainAEID: localChainAChainID,
		localChainBEID: localChainBChainID,
	}
	for _, configuredChain := range cfg.Chains {
		chainID, ok := expectedChains[configuredChain.EID]
		if !ok {
			return fmt.Errorf("chain eid %d is not a local e2e chain", configuredChain.EID)
		}
		if configuredChain.ChainID != chainID {
			return fmt.Errorf("chain %d chain_id = %d, want local e2e chain_id %d", configuredChain.EID, configuredChain.ChainID, chainID)
		}
		if configuredChain.StartBlockNumber != 0 {
			return fmt.Errorf("chain %d start_block_number = %d, want 0 for rebuild replay", configuredChain.EID, configuredChain.StartBlockNumber)
		}
		for _, rpcURL := range configuredChain.RPCURLs {
			if err := validateLocalHTTPURL(rpcURL, fmt.Sprintf("chain %d rpc_url", configuredChain.EID)); err != nil {
				return err
			}
		}
	}
	if _, ok := expectedChains[evidence.SrcEID]; !ok {
		return fmt.Errorf("evidence source eid %d is not a local e2e chain", evidence.SrcEID)
	}
	if _, ok := expectedChains[evidence.DstEID]; !ok {
		return fmt.Errorf("evidence destination eid %d is not a local e2e chain", evidence.DstEID)
	}
	for _, pathway := range cfg.Pathways {
		if pathway.SrcEID == evidence.SrcEID && pathway.DstEID == evidence.DstEID {
			if !pathway.Enabled {
				return fmt.Errorf("evidence pathway %d->%d is disabled", evidence.SrcEID, evidence.DstEID)
			}
			return nil
		}
	}
	return fmt.Errorf("evidence pathway %d->%d is not configured", evidence.SrcEID, evidence.DstEID)
}

func validateLocalE2ERPCChainIDs(ctx context.Context, registry *chain.Registry) error {
	for _, configuredChain := range registry.All() {
		if err := configuredChain.RPC.ValidateChainID(ctx, configuredChain.ChainID); err != nil {
			return fmt.Errorf("validate local e2e chain %d rpc chain_id: %w", configuredChain.EID, err)
		}
	}
	return nil
}

// CompareFinalRows validates final replay state loaded from Postgres.
func CompareFinalRows(evidence Evidence, rows []FinalRow) error {
	evidence, err := NormalizeEvidence(evidence)
	if err != nil {
		return err
	}
	if len(rows) != len(evidence.ExpectedPackets) {
		return fmt.Errorf("final rows = %d, want %d", len(rows), len(evidence.ExpectedPackets))
	}
	expected := make(map[string]ExpectedPacket, len(evidence.ExpectedPackets))
	for _, packet := range evidence.ExpectedPackets {
		expected[packet.GUID] = packet
	}
	for _, row := range rows {
		guid := normalizeHashForCompare(row.GUID)
		packet, ok := expected[guid]
		if !ok {
			return fmt.Errorf("unexpected final row guid %s", guid)
		}
		if row.PacketStatus != string(packets.ExecutorDelivered) {
			return fmt.Errorf("packet %s status = %s, want %s", guid, row.PacketStatus, packets.ExecutorDelivered)
		}
		if row.ExecutorState != string(packets.ExecutorDelivered) {
			return fmt.Errorf("executor job %s status = %s, want %s", guid, row.ExecutorState, packets.ExecutorDelivered)
		}
		if row.DVNState != string(packets.DVNVerified) {
			return fmt.Errorf("dvn job %s status = %s, want %s", guid, row.DVNState, packets.DVNVerified)
		}
		if normalizeHashForCompare(row.CommitTxHash) != packet.CommitTxHash {
			return fmt.Errorf("executor job %s commit_tx_hash = %s, want %s", guid, row.CommitTxHash, packet.CommitTxHash)
		}
		if normalizeHashForCompare(row.ReceiveTxHash) != packet.ReceiveTxHash {
			return fmt.Errorf("executor job %s receive_tx_hash = %s, want %s", guid, row.ReceiveTxHash, packet.ReceiveTxHash)
		}
		if normalizeHashForCompare(row.VerifyTxHash) != packet.VerifyTxHash {
			return fmt.Errorf("dvn job %s verify_tx_hash = %s, want %s", guid, row.VerifyTxHash, packet.VerifyTxHash)
		}
		if row.OutboxRows != 0 {
			return fmt.Errorf("packet %s has %d tx_outbox rows, want 0", guid, row.OutboxRows)
		}
		delete(expected, guid)
	}
	if len(expected) > 0 {
		missing := make([]string, 0, len(expected))
		for guid := range expected {
			missing = append(missing, guid)
		}
		sort.Strings(missing)
		return fmt.Errorf("missing final rows for %s", strings.Join(missing, ","))
	}
	return nil
}

func waitForDestinationDeferred(ctx context.Context, store *db.Store, destinationIndexer *indexer.Indexer, dstEID uint32) error {
	var lastErr error
	executorDeferredSeen := false
	dvnDeferredSeen := false
	for {
		result, err := destinationIndexer.ProcessOnce(ctx)
		if err != nil {
			lastErr = err
		} else if result.DestinationToBlock > 0 {
			executorDeferred, err := cursorDeferred(ctx, store, dstEID, indexer.ExecutorDestinationStream, result.DestinationToBlock)
			if err != nil {
				return err
			}
			dvnDeferred, err := cursorDeferred(ctx, store, dstEID, indexer.DVNDestinationStream, result.DestinationToBlock)
			if err != nil {
				return err
			}
			executorDeferredSeen = executorDeferredSeen || executorDeferred
			dvnDeferredSeen = dvnDeferredSeen || dvnDeferred
			if executorDeferredSeen && dvnDeferredSeen {
				return nil
			}
			lastErr = fmt.Errorf("destination cursors have not both deferred yet through block %d", result.DestinationToBlock)
		} else {
			lastErr = errors.New("destination indexer did not process a destination window")
		}
		if err := sleepOrDone(ctx); err != nil {
			return fmt.Errorf("timed out waiting for destination cursor deferral: %w", lastErr)
		}
	}
}

func waitForSourceRows(ctx context.Context, pool *pgxpool.Pool, sourceIndexer *indexer.Indexer, evidence Evidence) error {
	var lastErr error
	for {
		if _, err := sourceIndexer.ProcessOnce(ctx); err != nil {
			lastErr = err
		} else if ok, err := sourceRowsPresent(ctx, pool, evidence); err != nil {
			lastErr = err
		} else if ok {
			return nil
		} else {
			lastErr = errors.New("source packet/job rows are not present yet")
		}
		if err := sleepOrDone(ctx); err != nil {
			return fmt.Errorf("timed out waiting for source replay rows: %w", lastErr)
		}
	}
}

func waitForFinalRows(ctx context.Context, pool *pgxpool.Pool, destinationIndexer *indexer.Indexer, evidence Evidence) error {
	var lastErr error
	for {
		if _, err := destinationIndexer.ProcessOnce(ctx); err != nil {
			lastErr = err
		} else if rows, err := loadFinalRows(ctx, pool, evidence); err != nil {
			lastErr = err
		} else if err := CompareFinalRows(evidence, rows); err != nil {
			lastErr = err
		} else {
			return nil
		}
		if err := sleepOrDone(ctx); err != nil {
			return fmt.Errorf("timed out waiting for destination replay convergence: %w", lastErr)
		}
	}
}

func cursorDeferred(ctx context.Context, store *db.Store, chainEID uint32, stream string, target uint64) (bool, error) {
	cursor, err := store.GetIndexerCursor(ctx, chainEID, stream)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return cursor < target, nil
}

func resetDatabase(ctx context.Context, databaseURL string) error {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE`); err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `CREATE SCHEMA public`)
	return err
}

func indexedPacketCount(ctx context.Context, pool *pgxpool.Pool, evidence Evidence) (int64, error) {
	sourceTxHash, err := hexBytes(evidence.SourceTxHash)
	if err != nil {
		return 0, err
	}
	var count int64
	err = pool.QueryRow(ctx, `
		SELECT count(*)::bigint
		FROM packets
		WHERE src_eid = $1 AND dst_eid = $2 AND src_tx_hash = $3
	`, evidence.SrcEID, evidence.DstEID, sourceTxHash).Scan(&count)
	return count, err
}

func sourceRowsPresent(ctx context.Context, pool *pgxpool.Pool, evidence Evidence) (bool, error) {
	for _, packet := range evidence.ExpectedPackets {
		guid, err := hexBytes(packet.GUID)
		if err != nil {
			return false, err
		}
		var present bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM packets p
				JOIN executor_jobs ej ON ej.guid = p.guid
				JOIN dvn_jobs dj ON dj.guid = p.guid
				WHERE p.guid = $1
					AND p.src_eid = $2
					AND p.dst_eid = $3
					AND p.src_tx_hash = $4
			)
		`, guid, evidence.SrcEID, evidence.DstEID, mustHexBytes(evidence.SourceTxHash)).Scan(&present); err != nil {
			return false, err
		}
		if !present {
			return false, nil
		}
	}
	return true, nil
}

func loadFinalRows(ctx context.Context, pool *pgxpool.Pool, evidence Evidence) ([]FinalRow, error) {
	rows := make([]FinalRow, 0, len(evidence.ExpectedPackets))
	for _, packet := range evidence.ExpectedPackets {
		guid, err := hexBytes(packet.GUID)
		if err != nil {
			return nil, err
		}
		var row FinalRow
		row.GUID = packet.GUID
		if err := pool.QueryRow(ctx, `
			SELECT
				p.status,
				ej.status,
				COALESCE(encode(ej.commit_tx_hash, 'hex'), ''),
				COALESCE(encode(ej.receive_tx_hash, 'hex'), ''),
				dj.status,
				COALESCE(encode(dj.verify_tx_hash, 'hex'), ''),
				count(tx.id)::bigint
			FROM packets p
			JOIN executor_jobs ej ON ej.guid = p.guid
			JOIN dvn_jobs dj ON dj.guid = p.guid
			LEFT JOIN tx_outbox tx ON tx.guid = p.guid
			WHERE p.guid = $1
			GROUP BY p.status, ej.status, ej.commit_tx_hash, ej.receive_tx_hash, dj.status, dj.verify_tx_hash
		`, guid).Scan(
			&row.PacketStatus,
			&row.ExecutorState,
			&row.CommitTxHash,
			&row.ReceiveTxHash,
			&row.DVNState,
			&row.VerifyTxHash,
			&row.OutboxRows,
		); err != nil {
			return nil, err
		}
		row.CommitTxHash = with0x(row.CommitTxHash)
		row.ReceiveTxHash = with0x(row.ReceiveTxHash)
		row.VerifyTxHash = with0x(row.VerifyTxHash)
		rows = append(rows, row)
	}
	return rows, nil
}

func validateLocalDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse database_url: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("database_url scheme %q is not postgres", parsed.Scheme)
	}
	if parsed.User == nil || parsed.User.Username() != "laz_worker" {
		return errors.New("database_url user must be laz_worker for local e2e replay check")
	}
	if strings.TrimPrefix(parsed.Path, "/") != "laz_worker" {
		return errors.New("database_url database must be laz_worker for local e2e replay check")
	}
	if parsed.Query().Get("sslmode") != "disable" {
		return errors.New("database_url sslmode must be disable for local e2e replay check")
	}
	return validateLocalHost(parsed.Hostname(), "database_url host")
}

func validateLocalHTTPURL(raw, label string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s scheme %q is not http or https", label, parsed.Scheme)
	}
	return validateLocalHost(parsed.Hostname(), label+" host")
}

func validateLocalHost(host, label string) error {
	switch strings.ToLower(host) {
	case "localhost":
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%s %q is not a loopback host", label, host)
	}
	return nil
}

func normalizeHash(value, label string) (string, error) {
	value = normalizeHashForCompare(value)
	if !hashPattern.MatchString(value) {
		return "", fmt.Errorf("%s must be a 32-byte hex value", label)
	}
	return value, nil
}

func normalizeHashForCompare(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if strings.HasPrefix(value, "0X") {
		value = "0x" + value[2:]
	}
	if !strings.HasPrefix(value, "0x") {
		value = "0x" + value
	}
	return strings.ToLower(value)
}

func hexByteLen(value string) int {
	return (len(value) - 2) / 2
}

func hexBytes(value string) ([]byte, error) {
	normalized := strings.TrimPrefix(normalizeHashForCompare(value), "0x")
	return hex.DecodeString(normalized)
}

func mustHexBytes(value string) []byte {
	out, err := hexBytes(value)
	if err != nil {
		panic(err)
	}
	return out
}

func with0x(value string) string {
	if value == "" {
		return value
	}
	return common.BytesToHash(mustDecodeHex(value)).Hex()
}

func mustDecodeHex(value string) []byte {
	out, err := hex.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return out
}

func sleepOrDone(ctx context.Context) error {
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
