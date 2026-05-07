package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

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
