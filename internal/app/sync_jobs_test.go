package app

import (
	"context"
	"testing"
)

func TestCancelSyncJobCancelsRegisteredWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &Server{
		syncCancels: map[string]context.CancelFunc{"job-1": cancel},
	}
	server.cancelSyncJob("job-1")

	select {
	case <-ctx.Done():
	default:
		t.Fatal("worker context was not canceled")
	}
}

func TestCancelSyncJobIsSafeForUnknownJob(t *testing.T) {
	server := &Server{syncCancels: make(map[string]context.CancelFunc)}
	server.cancelSyncJob("missing")
}
