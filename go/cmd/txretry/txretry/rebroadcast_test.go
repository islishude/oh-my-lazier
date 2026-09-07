package txretry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
)

type replayStore struct {
	Store
	attempt             db.RebroadcastAttempt
	claims, records     int
	claimErr, recordErr error
	class, detail       string
}

func (s *replayStore) GetRebroadcastAttempt(context.Context, int64) (db.RebroadcastAttempt, error) {
	return s.attempt, nil
}
func (s *replayStore) ClaimManualRebroadcast(_ context.Context, a db.RebroadcastAttempt, _ uuid.UUID, ttl time.Duration) error {
	if ttl != txmgr.DefaultBroadcastLeaseTTL || !bytes.Equal(a.RawTx, s.attempt.RawTx) {
		return errors.New("incorrect claim")
	}
	s.claims++
	return s.claimErr
}
func (s *replayStore) MarkAttemptSendResult(ctx context.Context, _ int64, _ uuid.UUID, class, detail string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.records++
	s.class = class
	s.detail = detail
	return s.recordErr
}

type replayRPC struct {
	chain             *big.Int
	chainErr, sendErr error
	sent, closed      int
	raw               []byte
	cancel            context.CancelFunc
}

func (c *replayRPC) ChainID(context.Context) (*big.Int, error) { return c.chain, c.chainErr }
func (c *replayRPC) Close()                                    { c.closed++ }
func (c *replayRPC) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing deadline")
	}
	c.sent++
	c.raw, _ = tx.MarshalBinary()
	if c.cancel != nil {
		c.cancel()
	}
	return c.sendErr
}
func replayFixture(t *testing.T, dynamic bool) (*replayStore, *replayRPC, Dependencies) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	to := common.HexToAddress("0x1234")
	var tx *types.Transaction
	if dynamic {
		tx = types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: 7, To: &to, Gas: 21000, GasFeeCap: big.NewInt(20), GasTipCap: big.NewInt(1)})
	} else {
		tx = types.NewTx(&types.LegacyTx{Nonce: 7, To: &to, Gas: 21000, GasPrice: big.NewInt(20)})
	}
	tx, err = types.SignTx(tx, types.LatestSignerForChainID(big.NewInt(1)), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	s := &replayStore{attempt: db.RebroadcastAttempt{BroadcastClaim: db.BroadcastClaim{OutboxID: 1, AttemptID: 2, Nonce: 7, TxHash: tx.Hash(), RawTx: raw}, ChainEID: 40161, SignerID: crypto.PubkeyToAddress(key.PublicKey).Hex()}}
	c := &replayRPC{chain: big.NewInt(1)}
	deps := Dependencies{Chains: []config.ChainConfig{{EID: 40161, ChainID: 1}}, Dial: func(context.Context, string) (RPCClient, error) { return c, nil }}
	return s, c, deps
}
func TestRebroadcastOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Options
		ok   bool
	}{
		{"missing URL", Options{ID: 1, Action: "rebroadcast"}, false},
		{"HTTP", Options{ID: 1, Action: "rebroadcast", RPCURL: "https://rpc.example/key"}, true},
		{"WS", Options{ID: 1, Action: "rebroadcast", RPCURL: "wss://rpc.example"}, true},
		{"IPC", Options{ID: 1, Action: "rebroadcast", RPCURL: "/tmp/rpc.ipc"}, true},
		{"bad URL", Options{ID: 1, Action: "rebroadcast", RPCURL: "ftp://SECRET"}, false},
		{"unexpected URL", Options{ID: 1, Action: "inspect", RPCURL: "https://SECRET"}, false},
		{"bad id", Options{Action: "rebroadcast", RPCURL: "https://rpc.example"}, false},
		{"unexpected resolution", Options{ID: 1, Action: "rebroadcast", RPCURL: "https://rpc.example", Resolution: "retry"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.o.Validate()
			if (err == nil) != tc.ok {
				t.Fatalf("error=%v", err)
			}
			if err != nil && strings.Contains(err.Error(), "SECRET") {
				t.Fatal("secret leaked")
			}
		})
	}
}
func TestRebroadcastOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sendErr    error
		class      string
		recordFail bool
	}{
		{"accepted legacy", nil, db.SendErrorAccepted, false},
		{"accepted dynamic", nil, db.SendErrorAccepted, false},
		{"known", errors.New("already known https://rpc/SECRET"), db.SendErrorAccepted, false},
		{"underpriced", errors.New("transaction underpriced SECRET"), db.SendErrorUnderpriced, false},
		{"nonce low", errors.New("nonce too low SECRET"), db.SendErrorNonceTooLow, false},
		{"rejected", errors.New("invalid sender SECRET"), db.SendErrorDefinitive, false},
		{"timeout", context.DeadlineExceeded, db.SendErrorAmbiguous, false},
		{"canceled", context.Canceled, db.SendErrorAmbiguous, false},
		{"unknown", errors.New("https://rpc/SECRET"), db.SendErrorAmbiguous, false},
		{"writeback failed", nil, db.SendErrorAccepted, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, deps := replayFixture(t, tc.name != "accepted legacy")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.sendErr = tc.sendErr
			if tc.name == "canceled" {
				c.cancel = cancel
			}
			if tc.recordFail {
				s.recordErr = errors.New("SECRET database failure")
			}
			output, err := Run(ctx, s, Options{ID: 1, Action: "rebroadcast", RPCURL: "https://rpc/SECRET"}, deps)
			wantOK := tc.class == db.SendErrorAccepted && !tc.recordFail
			if (err == nil) != wantOK {
				t.Fatalf("error=%v", err)
			}
			if s.claims != 1 || s.records != 1 || c.sent != 1 || c.closed != 1 || !bytes.Equal(c.raw, s.attempt.RawTx) {
				t.Fatal("send/record/close mismatch")
			}
			var result txmgr.RebroadcastResult
			if e := json.Unmarshal(output, &result); e != nil {
				t.Fatal(e)
			}
			if result.SendClass != tc.class || result.Recorded == tc.recordFail || result.TxHash != s.attempt.TxHash {
				t.Fatalf("result=%s", output)
			}
			if strings.Contains(string(output)+s.detail, "SECRET") || (err != nil && strings.Contains(err.Error(), "SECRET")) {
				t.Fatal("secret leaked")
			}
			if tc.recordFail && !strings.Contains(err.Error(), "may have been accepted") {
				t.Fatal("missing ambiguous outcome warning")
			}
		})
	}
}
func TestRebroadcastPreflightRejectsWithoutSending(t *testing.T) {
	for _, name := range []string{"rpc chain", "rpc error", "dial error", "missing chain", "raw", "hash", "nonce", "sender", "transaction chain", "signature", "claim"} {
		t.Run(name, func(t *testing.T) {
			s, c, deps := replayFixture(t, true)
			switch name {
			case "rpc chain":
				c.chain = big.NewInt(2)
			case "rpc error":
				c.chainErr = errors.New("SECRET")
			case "dial error":
				deps.Dial = func(context.Context, string) (RPCClient, error) { return nil, errors.New("SECRET") }
			case "missing chain":
				deps.Chains = nil
			case "raw":
				s.attempt.RawTx = []byte{0}
			case "hash":
				s.attempt.TxHash = common.Hash{}
			case "nonce":
				s.attempt.Nonce++
			case "sender":
				s.attempt.SignerID = common.Address{}.Hex()
			case "transaction chain":
				deps.Chains[0].ChainID = 2
				c.chain = big.NewInt(2)
			case "signature":
				tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: 7, Gas: 21000, GasFeeCap: big.NewInt(20), GasTipCap: big.NewInt(1)})
				s.attempt.RawTx, _ = tx.MarshalBinary()
				s.attempt.TxHash = tx.Hash()
			case "claim":
				s.claimErr = errors.New("SECRET")
			}
			_, err := Run(t.Context(), s, Options{ID: 1, Action: "rebroadcast", RPCURL: "https://rpc/SECRET"}, deps)
			if err == nil || c.sent != 0 || s.records != 0 {
				t.Fatalf("unexpected send: %v", err)
			}
			if name != "claim" && s.claims != 0 {
				t.Fatal("preflight spent budget")
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("secret leaked")
			}
		})
	}
}
func TestRebroadcastRPCWire(t *testing.T) {
	s, _, deps := replayFixture(t, true)
	sends := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x1"
		case "eth_sendRawTransaction":
			sends++
			var raw string
			if len(request.Params) != 1 {
				t.Error("unexpected params")
				return
			}
			if err := json.Unmarshal(request.Params[0], &raw); err != nil {
				t.Error(err)
				return
			}
			if raw != hexutil.Encode(s.attempt.RawTx) {
				t.Error("raw transaction changed")
			}
			result = s.attempt.TxHash.Hex()
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	deps.Dial = nil
	if _, err := Run(t.Context(), s, Options{ID: 1, Action: "rebroadcast", RPCURL: server.URL}, deps); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("sends=%d", sends)
	}
}
