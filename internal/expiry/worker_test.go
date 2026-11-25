package expiry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

const (
	testInterval  = time.Minute
	testBatchSize = 5
)

type fakeExpirer struct {
	mu      sync.Mutex
	results []result
	calls   atomic.Int64
}

type result struct {
	n   int
	err error
}

func (f *fakeExpirer) expire(context.Context, int) (int, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.results) == 0 {
		return 0, nil
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r.n, r.err
}

func startWorker(t *testing.T, f *fakeExpirer, logs io.Writer) (cancel func(), done <-chan struct{}) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan struct{})
	w := Worker{
		Name:      "test",
		Expire:    f.expire,
		Interval:  testInterval,
		BatchSize: testBatchSize,
		Logger:    slog.New(slog.NewTextHandler(logs, nil)),
	}
	go func() {
		defer close(finished)
		w.Run(ctx)
	}()
	return cancel, finished
}

func TestWorker_DrainsFullBatchesRightAway(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeExpirer{results: []result{{n: testBatchSize}, {n: testBatchSize}, {n: 2}}}
		cancel, done := startWorker(t, f, io.Discard)
		defer func() { cancel(); <-done }()

		synctest.Wait()
		if got := f.calls.Load(); got != 3 {
			t.Errorf("calls before the first tick = %d, want 3", got)
		}
	})
}

func TestWorker_RunsOnEveryTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeExpirer{}
		cancel, done := startWorker(t, f, io.Discard)
		defer func() { cancel(); <-done }()

		for want := int64(1); want <= 3; want++ {
			synctest.Wait()
			if got := f.calls.Load(); got != want {
				t.Errorf("calls = %d, want %d", got, want)
			}
			time.Sleep(testInterval)
		}
	})
}

func TestWorker_KeepsRunningAfterAnError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		f := &fakeExpirer{results: []result{{err: errors.New("connection refused")}}}
		cancel, done := startWorker(t, f, &logs)
		defer func() { cancel(); <-done }()

		synctest.Wait()
		time.Sleep(testInterval)
		synctest.Wait()

		if got := f.calls.Load(); got != 2 {
			t.Errorf("calls = %d, want 2", got)
		}
		if !strings.Contains(logs.String(), "connection refused") {
			t.Errorf("logs = %q, want the error", logs.String())
		}
	})
}

func TestWorker_LogsOnlyBatchesThatExpiredReservations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		f := &fakeExpirer{results: []result{{n: 1}}}
		cancel, done := startWorker(t, f, &logs)
		defer func() { cancel(); <-done }()

		synctest.Wait()
		if got := logs.String(); !strings.Contains(got, "worker=test") || !strings.Contains(got, "count=1") {
			t.Errorf("logs after expiring one reservation = %q, want worker=test and count=1", got)
		}

		logs.Reset()
		time.Sleep(testInterval)
		synctest.Wait()
		if got := logs.String(); got != "" {
			t.Errorf("logs after a batch with nothing due = %q, want none", got)
		}
	})
}

func TestWorker_StopsWhenContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeExpirer{}
		cancel, done := startWorker(t, f, io.Discard)

		synctest.Wait()
		cancel()
		synctest.Wait()

		select {
		case <-done:
		default:
			t.Fatal("Run did not return after the context was cancelled")
		}
	})
}
