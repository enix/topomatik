package schedulers

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSometimesWithDebounce_FirstCallRunsImmediately(t *testing.T) {
	swd := NewSometimesWithDebounce(200 * time.Millisecond)
	var calls atomic.Int32

	swd.Do(func() { calls.Add(1) })

	if got := calls.Load(); got != 1 {
		t.Fatalf("first call: got %d, want 1", got)
	}
}

func TestSometimesWithDebounce_BurstCoalescesIntoOneDeferredCall(t *testing.T) {
	const interval = 200 * time.Millisecond
	swd := NewSometimesWithDebounce(interval)
	var calls atomic.Int32

	for i := 0; i < 5; i++ {
		swd.Do(func() { calls.Add(1) })
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("immediately after burst: got %d, want 1", got)
	}

	time.Sleep(interval + 150*time.Millisecond)

	if got := calls.Load(); got != 2 {
		t.Fatalf("after debounce window: got %d, want 2", got)
	}
}

func TestSometimesWithDebounce_CallAfterIntervalRunsImmediately(t *testing.T) {
	const interval = 100 * time.Millisecond
	swd := NewSometimesWithDebounce(interval)
	var calls atomic.Int32

	swd.Do(func() { calls.Add(1) })
	time.Sleep(interval + 80*time.Millisecond)
	swd.Do(func() { calls.Add(1) })

	if got := calls.Load(); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestSometimesWithDebounce_LatestCallbackWinsWhenDebounced(t *testing.T) {
	const interval = 200 * time.Millisecond
	swd := NewSometimesWithDebounce(interval)
	var firstHits, secondHits atomic.Int32

	swd.Do(func() {})
	swd.Do(func() { firstHits.Add(1) })
	swd.Do(func() { secondHits.Add(1) })

	time.Sleep(interval + 150*time.Millisecond)

	if got := secondHits.Load(); got != 1 {
		t.Errorf("latest callback: got %d, want 1", got)
	}
	if got := firstHits.Load(); got != 0 {
		t.Errorf("replaced callback: got %d, want 0", got)
	}
}

func TestSometimesWithDebounceChannel_TriggerEmitsOnChannel(t *testing.T) {
	c := NewSometimesWithDebounceChannel(100 * time.Millisecond)

	c.Trigger()

	select {
	case <-c.C():
	case <-time.After(time.Second):
		t.Fatal("expected event on channel after Trigger")
	}
}

func TestSometimesWithDebounceChannel_BurstCoalescesIntoOneEvent(t *testing.T) {
	const interval = 200 * time.Millisecond
	c := NewSometimesWithDebounceChannel(interval)

	c.Trigger()
	<-c.C()

	for i := 0; i < 5; i++ {
		c.Trigger()
	}

	deadline := time.After(interval + 200*time.Millisecond)
	count := 0
loop:
	for {
		select {
		case <-c.C():
			count++
		case <-deadline:
			break loop
		}
	}

	if count != 1 {
		t.Errorf("coalesced events: got %d, want 1", count)
	}
}
