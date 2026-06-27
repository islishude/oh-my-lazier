package executor

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestProcessCommitterOnceEnqueuesCommitTx(t *testing.T) {
	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorVerifiable)
	store := &fakeStore{
		work: []db.ExecutorWorkItem{{
			Packet: packet,
			Job:    db.ExecutorJobRecord{GUID: packet.GUID, Status: string(packets.ExecutorVerifiable)},
		}},
	}
	worker := NewWithCallers(
		store,
		testRegistry(t),
		map[uint32]ContractCaller{packet.DstEID: fakeCommitReadyCaller{}},
		"0x9999999999999999999999999999999999999999",
		slog.Default(),
	)

	processed, err := worker.ProcessCommitterOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessCommitterOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.expectedStatus != string(packets.ExecutorVerifiable) {
		t.Fatalf("expected status = %q, want %q", store.expectedStatus, packets.ExecutorVerifiable)
	}
	if store.nextStatus != string(packets.ExecutorCommitTxEnqueued) {
		t.Fatalf("next status = %q, want %q", store.nextStatus, packets.ExecutorCommitTxEnqueued)
	}
	if store.request.Purpose != TxPurposeCommitVerification {
		t.Fatalf("purpose = %q, want %q", store.request.Purpose, TxPurposeCommitVerification)
	}
	if store.request.To != common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatalf("to = %s, want receive lib", store.request.To)
	}
}

func TestIsCommitVerifiableRejectsEmptyPayloadHash(t *testing.T) {
	packet := testPacketRecord()
	packet.PayloadHash = common.Hash{}

	ready, err := IsCommitVerifiable(
		context.Background(),
		failingCaller{},
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		packet,
	)
	if err == nil {
		t.Fatal("IsCommitVerifiable() error = nil, want payload hash validation error")
	}
	if ready {
		t.Fatal("ready = true, want false")
	}
}

func TestProcessDelivererOnceEnqueuesLzReceiveTx(t *testing.T) {
	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorExecutable)
	store := &fakeStore{
		workByStatus: map[string][]db.ExecutorWorkItem{string(packets.ExecutorExecutable): {{
			Packet: packet,
			Job:    db.ExecutorJobRecord{GUID: packet.GUID, Status: string(packets.ExecutorExecutable)},
		}}},
	}
	worker := NewWithCallers(
		store,
		testRegistry(t),
		map[uint32]ContractCaller{packet.DstEID: fakeExecutableCaller{payloadHash: packet.PayloadHash, inboundNonce: 7}},
		"0x9999999999999999999999999999999999999999",
		slog.Default(),
	)

	processed, err := worker.ProcessDelivererOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessDelivererOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.expectedStatus != string(packets.ExecutorExecutable) {
		t.Fatalf("expected status = %q, want %q", store.expectedStatus, packets.ExecutorExecutable)
	}
	if store.nextStatus != string(packets.ExecutorLzReceiveTxEnqueued) {
		t.Fatalf("next status = %q, want %q", store.nextStatus, packets.ExecutorLzReceiveTxEnqueued)
	}
	if store.request.Purpose != TxPurposeLzReceive {
		t.Fatalf("purpose = %q, want %q", store.request.Purpose, TxPurposeLzReceive)
	}
	if store.request.To != common.HexToAddress("0x4444444444444444444444444444444444444444") {
		t.Fatalf("to = %s, want destination endpoint", store.request.To)
	}
}

func TestProcessDelivererOnceRetriesFailedLzReceive(t *testing.T) {
	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorLzReceiveFailed)
	store := &fakeStore{
		workByStatus: map[string][]db.ExecutorWorkItem{string(packets.ExecutorLzReceiveFailed): {{
			Packet: packet,
			Job:    db.ExecutorJobRecord{GUID: packet.GUID, Status: string(packets.ExecutorLzReceiveFailed), LastError: "previous alert"},
		}}},
	}
	worker := NewWithCallers(
		store,
		testRegistry(t),
		map[uint32]ContractCaller{packet.DstEID: fakeExecutableCaller{payloadHash: packet.PayloadHash, inboundNonce: 7}},
		"0x9999999999999999999999999999999999999999",
		slog.Default(),
	)

	processed, err := worker.ProcessDelivererOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessDelivererOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.expectedStatus != string(packets.ExecutorLzReceiveFailed) {
		t.Fatalf("expected status = %q, want %q", store.expectedStatus, packets.ExecutorLzReceiveFailed)
	}
	if store.nextStatus != string(packets.ExecutorLzReceiveTxEnqueued) {
		t.Fatalf("next status = %q, want %q", store.nextStatus, packets.ExecutorLzReceiveTxEnqueued)
	}
	if store.request.Purpose != TxPurposeLzReceive {
		t.Fatalf("purpose = %q, want %q", store.request.Purpose, TxPurposeLzReceive)
	}
}

func TestProcessDelivererOnceSkipsWhenEndpointNotExecutable(t *testing.T) {
	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorExecutable)
	store := &fakeStore{
		workByStatus: map[string][]db.ExecutorWorkItem{string(packets.ExecutorExecutable): {{
			Packet: packet,
			Job:    db.ExecutorJobRecord{GUID: packet.GUID, Status: string(packets.ExecutorExecutable)},
		}}},
	}
	worker := NewWithCallers(
		store,
		testRegistry(t),
		map[uint32]ContractCaller{packet.DstEID: fakeExecutableCaller{payloadHash: packet.PayloadHash, inboundNonce: 6}},
		"0x9999999999999999999999999999999999999999",
		slog.Default(),
	)

	processed, err := worker.ProcessDelivererOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessDelivererOnce() error = %v", err)
	}
	if processed {
		t.Fatal("processed = true, want false")
	}
	if store.request.Purpose != "" {
		t.Fatalf("unexpected enqueue purpose %q", store.request.Purpose)
	}
}

type fakeStore struct {
	work           []db.ExecutorWorkItem
	workByStatus   map[string][]db.ExecutorWorkItem
	guid           common.Hash
	expectedStatus string
	nextStatus     string
	request        db.TxRequest
}

func (s *fakeStore) ListExecutorWork(_ context.Context, status string, _ int) ([]db.ExecutorWorkItem, error) {
	if s.workByStatus != nil {
		return s.workByStatus[status], nil
	}
	return s.work, nil
}

func (s *fakeStore) EnqueueExecutorTx(_ context.Context, guid common.Hash, expectedStatus, nextStatus string, request db.TxRequest) (int64, error) {
	s.guid = guid
	s.expectedStatus = expectedStatus
	s.nextStatus = nextStatus
	s.request = request
	return 123, nil
}

type failingCaller struct{}

func (failingCaller) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return nil, fmt.Errorf("unexpected eth_call")
}

type fakeCommitReadyCaller struct{}

func (fakeCommitReadyCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	method, err := methodBySelector(call.Data)
	if err != nil {
		return nil, err
	}
	switch method.Name {
	case "isValidReceiveLibrary", "verifiable":
		return method.Outputs.Pack(true)
	case "getUlnConfig":
		return method.Outputs.Pack(ulnConfig{
			Confirmations:        12,
			RequiredDVNCount:     1,
			OptionalDVNCount:     0,
			OptionalDVNThreshold: 0,
			RequiredDVNs:         []common.Address{common.HexToAddress("0x3333333333333333333333333333333333333333")},
		})
	default:
		return nil, fmt.Errorf("unexpected method %s", method.Name)
	}
}

type fakeExecutableCaller struct {
	payloadHash  common.Hash
	inboundNonce uint64
}

func (c fakeExecutableCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	method, err := methodBySelector(call.Data)
	if err != nil {
		return nil, err
	}
	switch method.Name {
	case "inboundPayloadHash":
		return method.Outputs.Pack(c.payloadHash)
	case "inboundNonce":
		return method.Outputs.Pack(c.inboundNonce)
	case "lazyInboundNonce":
		return method.Outputs.Pack(uint64(0))
	default:
		return nil, fmt.Errorf("unexpected method %s", method.Name)
	}
}

func methodBySelector(data []byte) (methodView, error) {
	if len(data) < 4 {
		return methodView{}, fmt.Errorf("call data length %d is shorter than selector", len(data))
	}
	for _, method := range endpointViewABI.Methods {
		if string(method.ID) == string(data[:4]) {
			return methodView{Name: method.Name, Outputs: method.Outputs}, nil
		}
	}
	for _, method := range receiveUlnViewABI.Methods {
		if string(method.ID) == string(data[:4]) {
			return methodView{Name: method.Name, Outputs: method.Outputs}, nil
		}
	}
	return methodView{}, fmt.Errorf("unknown selector %x", data[:4])
}

type methodView struct {
	Name    string
	Outputs interface {
		Pack(args ...any) ([]byte, error)
	}
}

func testRegistry(t *testing.T) *chain.Registry {
	t.Helper()
	registry, err := chain.NewRegistry([]config.ChainConfig{
		{
			EID:             40161,
			Name:            "ethereum-sepolia",
			ChainID:         11155111,
			EndpointAddress: "0x1111111111111111111111111111111111111111",
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8545"},
			Workers: config.WorkerContractsConfig{
				OpenExecutor: "0x2222222222222222222222222222222222222222",
				OpenDVN:      "0x3333333333333333333333333333333333333333",
			},
		},
		{
			EID:             40245,
			Name:            "base-sepolia",
			ChainID:         84532,
			EndpointAddress: "0x4444444444444444444444444444444444444444",
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8546"},
			Workers: config.WorkerContractsConfig{
				OpenExecutor: "0x5555555555555555555555555555555555555555",
				OpenDVN:      "0x6666666666666666666666666666666666666666",
			},
		},
	}, []config.PathwayConfig{
		{
			SrcEID:         40161,
			DstEID:         40245,
			SrcOApp:        "0x1111111111111111111111111111111111111111",
			DstOApp:        "0x2222222222222222222222222222222222222222",
			SendLib:        "0x9999999999999999999999999999999999999999",
			ReceiveLib:     "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Enabled:        true,
			MaxMessageSize: 10000,
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}
