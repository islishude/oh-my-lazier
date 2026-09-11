package pricing

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
)

func TestOnlySourceFailures(t *testing.T) {
	source := runtimeSourceFailure(1, "stale", errors.New("old price"))
	database := errors.New("database unavailable")
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"source", source, true},
		{"wrapped aggregate", fmt.Errorf("cycle: %w", errors.Join(source, source)), true},
		{"mixed", errors.Join(source, database), false},
		{"nested mixed", errors.Join(source, fmt.Errorf("feed: %w", errors.Join(source, database))), false},
		{"cancel", errors.Join(source, context.Canceled), false},
		{"fatal", workerloop.Fatal(source), false},
		{"configuration", runtimeSourceFailure(1, "unavailable", newPriceSourceConfigurationError(database)), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := onlySourceFailures(tc.err); got != tc.want {
				t.Fatalf("onlySourceFailures = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBotSourceCooldownAcrossTriggers(t *testing.T) {
	for _, trigger := range []string{"periodic", "gas spike", "pending drain"} {
		t.Run(trigger, func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			stale := &countingPrice{source: "primary", price: big.NewRat(2000, 1), observedAt: now.Add(-2 * time.Minute)}
			sources := testSources()
			source := sources[40161]
			source.Primary = ConfiguredPriceReader{Name: "primary", Reader: stale, MaxAge: time.Minute}
			sources[40161] = source
			store := &fakeStore{}
			bot, err := NewWithDependencies(store, testRegistry(t), testSettings(), sources, emptySnapshotReader{}, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			bot.now = func() time.Time { return now }
			recorder := &fakeMetricsRecorder{}
			bot.WithMetrics(recorder)
			if err := bot.EnqueueOnce(t.Context()); !onlySourceFailures(err) {
				t.Fatalf("initial error = %v", err)
			}
			updates, err := bot.uniquePriceUpdates()
			if err != nil {
				t.Fatal(err)
			}
			bot.lastGasPrices = make(map[string]*big.Int)
			for _, update := range updates {
				bot.lastGasPrices[priceUpdateKey(update)] = big.NewInt(1)
				if trigger == "pending drain" {
					bot.rememberPendingFeed(update.SrcEID, update.PriceFeed)
				}
			}
			evaluate := bot.EnqueueOnce
			if trigger != "periodic" {
				evaluate = bot.EnqueueOnGasSpike
			}
			for range 3 {
				now = now.Add(10 * time.Second)
				if err := evaluate(t.Context()); !onlySourceFailures(err) {
					t.Fatalf("cooldown error = %v", err)
				}
			}
			if stale.count.Load() != 1 || recorder.failures != 1 || len(store.requests) != 0 {
				t.Fatalf("reads=%d failures=%d writes=%d", stale.count.Load(), recorder.failures, len(store.requests))
			}
			if trigger == "pending drain" && len(bot.pendingFeeds) != 2 {
				t.Fatalf("lost drain markers: %v", bot.pendingFeeds)
			}
			now = now.Add(30 * time.Second)
			stale.observedAt = now
			if err := evaluate(t.Context()); err != nil {
				t.Fatal(err)
			}
			if stale.count.Load() != 2 || len(store.requests) != 2 || len(bot.sourceCooldowns) != 0 {
				t.Fatalf("recovery reads=%d writes=%d cooldowns=%d", stale.count.Load(), len(store.requests), len(bot.sourceCooldowns))
			}
		})
	}
}

func TestBotRunWaitsForSourceRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stale := newScheduledPrice(time.Now().Add(-2 * time.Minute))
		sources := testSources()
		other := sources[40449]
		other.Primary.Reader = fixedPrice{source: "primary", price: big.NewRat(1000, 1), observedAt: time.Now()}
		sources[40449] = other
		source := sources[40161]
		source.Primary = ConfiguredPriceReader{Name: "primary", Reader: stale, MaxAge: time.Minute}
		sources[40161] = source
		store := &fakeStore{}
		bot, err := NewWithDependencies(store, testRegistry(t), testSettings(), sources, emptySnapshotReader{}, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- bot.Run(ctx) }()
		synctest.Wait()
		time.Sleep(59 * time.Second)
		synctest.Wait()
		if stale.count.Load() != 1 {
			t.Fatalf("premature reads: %d", stale.count.Load())
		}
		stale.observedAt.Store(time.Now().Unix())
		time.Sleep(time.Second)
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v", err)
		}
		if stale.count.Load() != 2 || len(store.requests) != 2 {
			t.Fatalf("recovery reads/writes: %d/%d", stale.count.Load(), len(store.requests))
		}

	})
}

func TestCooldownDoesNotBlockIndependentOrSameAssetFeeds(t *testing.T) {
	for _, sameAsset := range []bool{false, true} {
		t.Run(fmt.Sprint(sameAsset), func(t *testing.T) {
			sources := testSources()
			failed := sources[40161]
			reader := &countingPrice{source: "primary", price: big.NewRat(2000, 1), observedAt: time.Unix(1_699_990_000, 0)}
			failed.Primary = ConfiguredPriceReader{Name: "primary", Reader: reader, MaxAge: time.Minute}
			sources[40161] = failed
			sources[40500] = sources[40449]
			pathways := testPathways()
			independent := pathways[1]
			independent.DstEID = 40500
			pathways = append(pathways[:1], independent)
			if sameAsset {
				s := sources[40449]
				s.NativeAssetID = "shared"
				sources[40449] = s
				s = sources[40500]
				s.NativeAssetID = "shared"
				sources[40500] = s
			}
			store := &fakeStore{}
			bot, err := NewWithDependencies(store, testRegistryWithPathways(t, pathways), testSettings(), sources, emptySnapshotReader{}, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			bot.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
			if sameAsset {
				bot.sourceCooldowns = map[uint32]sourceCooldown{40500: {err: runtimeSourceFailure(40500, "stale", errors.New("old")), nextRetryAt: bot.now().Add(time.Minute)}}
			}
			if err := bot.EnqueueOnce(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(store.requests) != 1 {
				t.Fatalf("healthy writes=%d", len(store.requests))
			}
			if _, ok := bot.sourceCooldowns[40161]; !ok {
				t.Fatal("partial success did not register cooldown")
			}
			if err := bot.EnqueueOnce(t.Context()); err != nil {
				t.Fatal(err)
			}
			if reader.count.Load() != 1 {
				t.Fatalf("partial success bypassed cooldown: %d", reader.count.Load())
			}
		})
	}
}

func TestBotRunRetriesBetweenPeriodicTicks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newScheduledPrice(time.Now())
		sources := testSources()
		entry := sources[40161]
		entry.Primary = ConfiguredPriceReader{Name: "primary", Reader: source, MaxAge: 10 * time.Second}
		sources[40161] = entry
		entry = sources[40449]
		entry.Primary.Reader = fixedPrice{source: "primary", price: big.NewRat(1000, 1), observedAt: time.Now()}
		sources[40449] = entry
		store := &fakeStore{}
		bot, err := NewWithDependencies(store, testRegistry(t), testSettings(), sources, emptySnapshotReader{}, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- bot.Run(ctx) }()
		time.Sleep(15 * time.Second)
		synctest.Wait() // Fast drain evaluates the now-stale observation.
		if source.count.Load() != 2 {
			t.Fatalf("reads=%d", source.count.Load())
		}
		time.Sleep(59 * time.Second)
		synctest.Wait()
		if source.count.Load() != 2 {
			t.Fatalf("cooldown bypassed: reads=%d", source.count.Load())
		}
		source.observedAt.Store(time.Now().Unix())
		time.Sleep(time.Second)
		synctest.Wait() // t=75s, ahead of the t=120s periodic tick.
		cancel()
		synctest.Wait()
		<-done
		if source.count.Load() != 3 || len(store.requests) != 4 {
			t.Fatalf("off-grid recovery reads=%d writes=%d", source.count.Load(), len(store.requests))
		}

	})
}

type scheduledPrice struct {
	count      atomic.Int32
	observedAt atomic.Int64
}

func newScheduledPrice(observedAt time.Time) *scheduledPrice {
	source := &scheduledPrice{}
	source.observedAt.Store(observedAt.Unix())
	return source
}
func (s *scheduledPrice) PriceUSD(context.Context) (SourcePrice, error) {
	s.count.Add(1)
	return SourcePrice{Source: "primary", USD: big.NewRat(2000, 1), ObservedAt: time.Unix(s.observedAt.Load(), 0)}, nil
}

func TestBotRuntimeSourceFailureCategories(t *testing.T) {
	for _, tc := range []struct {
		name       string
		primaryErr error
		sanity     bool
		deviation  bool
		wantRetry  bool
	}{
		{name: "timeout", primaryErr: context.DeadlineExceeded, wantRetry: true},
		{name: "unavailable", primaryErr: errors.New("provider down"), wantRetry: true},
		{name: "sanity unavailable", sanity: true, primaryErr: errors.New("provider down"), wantRetry: true},
		{name: "deviation", sanity: true, deviation: true, wantRetry: true},
		{name: "configuration", primaryErr: newPriceSourceConfigurationError(errors.New("description mismatch"))},
		{name: "fatal", primaryErr: workerloop.Fatal(errors.New("broken invariant"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sources := testSources()
			entry := sources[40161]
			if tc.sanity {
				sanity := testConfiguredPrice("sanity", big.NewRat(9000, 1))
				if !tc.deviation {
					sanity.Reader = errorPrice{err: tc.primaryErr}
				}
				entry.Sanity = []ConfiguredPriceReader{sanity}
			} else {
				entry.Primary.Reader = errorPrice{err: tc.primaryErr}
			}
			sources[40161] = entry
			bot, err := NewWithDependencies(&fakeStore{}, testRegistry(t), testSettings(), sources, emptySnapshotReader{}, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			bot.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
			err = bot.EnqueueOnce(t.Context())
			if err == nil || onlySourceFailures(err) != tc.wantRetry {
				t.Fatalf("error=%v retry=%v", err, onlySourceFailures(err))
			}
			_, cooling := bot.sourceCooldowns[40161]
			if cooling != tc.wantRetry {
				t.Fatalf("cooling=%v", cooling)
			}
		})
	}
}

type errorPrice struct{ err error }

func (s errorPrice) PriceUSD(context.Context) (SourcePrice, error) { return SourcePrice{}, s.err }

func TestBotExpiredCooldownWaitsForPendingDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newScheduledPrice(time.Now().Add(-time.Hour))
		sources := testSources()
		entry := sources[40161]
		entry.Primary = ConfiguredPriceReader{Name: "primary", Reader: source, MaxAge: time.Minute}
		sources[40161] = entry
		entry = sources[40449]
		entry.Primary.Reader = fixedPrice{source: "primary", price: big.NewRat(1000, 1), observedAt: time.Now()}
		sources[40449] = entry
		pathways := testPathways()[:1]
		calldata, err := BuildSetPriceSnapshotCalldata([]PriceSnapshotUpdate{{DstEid: 40449, Snapshot: PriceSnapshot{DstGasPriceInSrcToken: big.NewInt(1), DstDataFeePerByteInSrcToken: big.NewInt(0), UpdatedAt: uint64(time.Now().Unix()), StaleAfter: 1800}}})
		if err != nil {
			t.Fatal(err)
		}
		store := &gatedPricingStore{rows: []db.PendingPricingTx{{ID: 1, To: common.HexToAddress(pathways[0].SourceWorkers.PriceFeed.Hex()), Calldata: calldata, Status: db.TxStatusQueued}}}
		bot, err := NewWithDependencies(store, testRegistryWithPathways(t, pathways), testSettings(), sources, emptySnapshotReader{}, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- bot.Run(ctx) }()
		time.Sleep(30 * time.Second)
		synctest.Wait()
		store.gated.Store(true)
		time.Sleep(95 * time.Second)
		synctest.Wait()
		if source.count.Load() != 1 || store.polls.Load() > 15 {
			t.Fatalf("pending cooldown spun: source=%d pending=%d", source.count.Load(), store.polls.Load())
		}
		source.observedAt.Store(time.Now().Unix())
		store.gated.Store(false)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		cancel()
		synctest.Wait()
		<-done
		if source.count.Load() != 2 || len(store.requests) != 1 {
			t.Fatalf("drain reads=%d writes=%d", source.count.Load(), len(store.requests))
		}
	})
}

type gatedPricingStore struct {
	fakeStore
	gated atomic.Bool
	polls atomic.Int32
	rows  []db.PendingPricingTx
}

func (s *gatedPricingStore) ListPendingPricingTxs(context.Context, uint32) ([]db.PendingPricingTx, error) {
	s.polls.Add(1)
	if s.gated.Load() {
		return s.rows, nil
	}
	return nil, nil
}

func TestBotRunPropagatesMixedSourceAndDatabaseFailures(t *testing.T) {
	sources := testSources()
	sources[40500] = sources[40449]
	entry := sources[40161]
	entry.Primary.Reader = errorPrice{err: errors.New("market offline")}
	sources[40161] = entry
	pathways := testPathways()
	pathways[1].DstEID = 40500
	databaseErr := errors.New("database unavailable")
	bot, err := NewWithDependencies(&fakeStore{enqueueErr: databaseErr}, testRegistryWithPathways(t, pathways), testSettings(), sources, emptySnapshotReader{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	bot.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	if err := bot.Run(t.Context()); !errors.Is(err, databaseErr) {
		t.Fatalf("Run error=%v, want database error preserved", err)
	}
}
