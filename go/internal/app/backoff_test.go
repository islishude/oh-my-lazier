package app

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/pricing"
)

func TestSupervisorBackoff(t *testing.T) {
	for _, tc := range []struct {
		name      string
		capDelay  time.Duration
		nilResult bool
		stableAt  int
		want      []time.Duration
	}{
		{"ordinary", time.Minute, false, -1, []time.Duration{5, 10, 20, 40, 60, 60}},
		{"unexpected return", time.Minute, true, -1, []time.Duration{5, 10, 20, 40, 60, 60}},
		{"pricing", 15 * time.Minute, false, -1, []time.Duration{5, 10, 20, 40, 80, 160, 320, 640, 900, 900}},
		{"short interval", 2 * time.Second, false, -1, []time.Duration{2, 2, 2}},
		{"stable reset", time.Minute, false, 3, []time.Duration{5, 10, 20, 5, 10, 20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			policy := newLoopBackoff(tc.capDelay)
			policy.now = func() time.Time { return now }
			var delays []time.Duration
			policy.wait = func(_ context.Context, delay time.Duration) bool {
				delays = append(delays, delay/time.Second)
				now = now.Add(delay)
				return len(delays) < len(tc.want)
			}
			calls := 0
			run := func(context.Context) error {
				if calls == tc.stableAt {
					now = now.Add(policy.resetAfter)
				}
				calls++
				if tc.nilResult {
					return nil
				}
				return errors.New("temporary")
			}
			if err := superviseLoop(t.Context(), "test", policy, discardLogger(), nil, run); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(delays, tc.want) {
				t.Fatalf("delays=%v, want %v", delays, tc.want)
			}
		})
	}
}

func TestSupervisorCancellationDuringBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		calls := 0
		go func() {
			done <- superviseLoop(ctx, "test", newLoopBackoff(time.Minute), discardLogger(), nil, func(context.Context) error { calls++; return errors.New("temporary") })
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-done; err != nil || calls != 1 {
			t.Fatalf("err=%v calls=%d", err, calls)
		}
	})
}

type pricingOutageStore struct{}

func (pricingOutageStore) ListPendingPricingTxs(context.Context, uint32) ([]db.PendingPricingTx, error) {
	return nil, nil
}
func (pricingOutageStore) EnqueuePricingSnapshotTx(context.Context, db.TxRequest) (int64, error) {
	return 0, errors.New("unexpected enqueue")
}

type pricingOutageSnapshot struct{}

func (pricingOutageSnapshot) PriceSnapshot(context.Context, uint32, common.Address, uint32) (pricing.PriceSnapshot, error) {
	return pricing.PriceSnapshot{DstGasPriceInSrcToken: big.NewInt(0), DstDataFeePerByteInSrcToken: big.NewInt(0)}, nil
}

type pricingOutageSource struct{ reads int }

func (s *pricingOutageSource) PriceUSD(context.Context) (pricing.SourcePrice, error) {
	s.reads++
	return pricing.SourcePrice{Source: "coingecko", USD: big.NewRat(1, 1), ObservedAt: time.Now().Add(-time.Hour)}, nil
}
func (*pricingOutageSource) SuggestGasPrice(context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func TestPricingSourceOutageDoesNotRestartSupervisor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig("0x9999999999999999999999999999999999999999", "unused")
		registry, err := chain.NewRegistry(cfg.Chains, cfg.Pathways)
		if err != nil {
			t.Fatal(err)
		}
		source := &pricingOutageSource{}
		sources := make(map[uint32]pricing.ChainSources)
		for _, c := range cfg.Chains {
			sources[c.EID] = pricing.ChainSources{Primary: pricing.ConfiguredPriceReader{Name: "coingecko", Reader: source, MaxAge: time.Minute}, Gas: source, DataFeePerByteWei: big.NewInt(0)}
		}
		settings := pricing.Settings{Enabled: true, SignerID: "0x9999999999999999999999999999999999999999", Interval: 15 * time.Minute, StaleAfter: 2 * time.Hour, Heartbeat: time.Hour, MaxDeviation: 500, MinUpdateDeviation: 50, SourceRequestTimeout: time.Second, GasSpikeBps: 1000}
		bot, err := pricing.NewWithDependencies(pricingOutageStore{}, registry, settings, sources, pricingOutageSnapshot{}, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		retries := &recordingLoopRetries{}
		done := make(chan error, 1)
		runs := 0
		go func() {
			done <- superviseLoop(ctx, "pricing", newLoopBackoff(settings.Interval), discardLogger(), retries, func(ctx context.Context) error { runs++; return bot.Run(ctx) })
		}()
		time.Sleep(31 * time.Minute)
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if runs != 1 || len(retries.names) != 0 || source.reads != 3 {
			t.Fatalf("runs=%d retries=%v source reads=%d", runs, retries.names, source.reads)
		}

	})
}
