package pricing

import (
	"context"
	"errors"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
)

// sourceFailure marks a rejected runtime observation, not a broken local setup.
type sourceFailure struct {
	eid      uint32
	category string
	cause    error
}

func (e *sourceFailure) Error() string { return e.cause.Error() }
func (e *sourceFailure) Unwrap() error { return e.cause }

type observationError struct {
	category string
	cause    error
}

func (e *observationError) Error() string { return e.cause.Error() }
func (e *observationError) Unwrap() error { return e.cause }

func runtimeSourceFailure(eid uint32, category string, err error) error {
	if isPriceSourceConfigurationError(err) || workerloop.IsFatal(err) || errors.Is(err, context.Canceled) {
		return err
	}
	return &sourceFailure{eid: eid, category: category, cause: err}
}

// All branches must be source failures: errors.As alone would hide mixed failures.
func onlySourceFailures(err error) bool {
	if err == nil || workerloop.IsFatal(err) || errors.Is(err, context.Canceled) {
		return false
	}
	// Inspect this exact tree node; errors.As would skip siblings in a mixed join.
	switch e := err.(type) { //nolint:errorlint
	case *sourceFailure:
		return true
	case interface{ Unwrap() []error }:
		children := e.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !onlySourceFailures(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return onlySourceFailures(e.Unwrap())
	default:
		return false
	}
}

type sourceCooldown struct {
	err          error
	nextRetryAt  time.Time
	wakeConsumed bool
}

func (b *Bot) runScheduled(ctx context.Context) error {
	if b.now == nil {
		b.now = time.Now
	}
	if err := b.EnqueueOnce(ctx); err != nil && !onlySourceFailures(err) {
		return err
	}
	nextPeriodic := b.now().Add(b.settings.Interval)
	gasInterval := min(b.settings.Interval, 15*time.Second)
	nextGas := b.now().Add(gasInterval)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		wake := minTime(nextPeriodic, nextGas)
		for _, cooldown := range b.sourceCooldowns {
			if !cooldown.wakeConsumed {
				wake = minTime(wake, cooldown.nextRetryAt)
			}
		}
		timer := time.NewTimer(max(0, wake.Sub(b.now())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		now := b.now()
		periodicDue := !now.Before(nextPeriodic)
		retryDue := false
		for eid, cooldown := range b.sourceCooldowns {
			if !cooldown.wakeConsumed && !now.Before(cooldown.nextRetryAt) {
				cooldown.wakeConsumed = true
				b.sourceCooldowns[eid] = cooldown
				retryDue = true
			}
		}
		var err error
		if periodicDue || retryDue {
			err = b.EnqueueOnce(ctx)
			// A retry evaluates every eligible feed too, satisfying the next
			// periodic pass even if its deadline falls during this evaluation.
			nextPeriodic = b.now().Add(b.settings.Interval)
		} else if !now.Before(nextGas) {
			err = b.EnqueueOnGasSpike(ctx)
		}
		if !now.Before(nextGas) {
			nextGas = b.now().Add(gasInterval)
		}
		if err != nil && !onlySourceFailures(err) {
			return err
		}
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
