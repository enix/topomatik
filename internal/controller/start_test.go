package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeStrategy struct {
	setupComplete chan struct{}
	payloads      chan map[string]string
}

func newFakeStrategy() *fakeStrategy {
	return &fakeStrategy{
		setupComplete: make(chan struct{}),
		payloads:      make(chan map[string]string),
	}
}

func (f *fakeStrategy) Setup(_ context.Context) error {
	close(f.setupComplete)
	return nil
}

func (f *fakeStrategy) Watch(ctx context.Context, callback func(data map[string]string, err error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-f.payloads:
			callback(data, nil)
		}
	}
}

type recordingScheduler struct {
	triggers chan struct{}
}

func newRecordingScheduler() *recordingScheduler {
	return &recordingScheduler{triggers: make(chan struct{})}
}

func (r *recordingScheduler) Trigger()           { r.triggers <- struct{}{} }
func (r *recordingScheduler) C() <-chan struct{} { return nil }

func TestStart_ReturnsOnContextCancel(t *testing.T) {
	clientset := fake.NewSimpleClientset(newTestNode("node-1", nil, nil))

	t.Setenv("NODE_NAME", "node-1")
	c, err := New(clientset, nopScheduler{}, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Start: got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestStart_PayloadUpdatesDiscoveryDataAndTriggers(t *testing.T) {
	clientset := fake.NewSimpleClientset(newTestNode("node-1", nil, nil))
	strategy := newFakeStrategy()
	sched := newRecordingScheduler()

	t.Setenv("NODE_NAME", "node-1")
	c, err := New(clientset, sched, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Register("fake", strategy)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	select {
	case <-strategy.setupComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("strategy.Setup was not called")
	}

	payload := map[string]string{"zone": "eu-west", "rack": "r1"}
	select {
	case strategy.payloads <- payload:
	case <-time.After(2 * time.Second):
		t.Fatal("payload was not consumed by the engine watch")
	}

	select {
	case <-sched.triggers:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler.Trigger was not called after payload")
	}

	if diff := cmp.Diff(payload, c.discoveryData["fake"]); diff != "" {
		t.Errorf("discoveryData[fake] (-want +got):\n%s", diff)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}
