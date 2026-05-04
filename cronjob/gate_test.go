package cronjob

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestGateForwardsFires(t *testing.T) {
	d := &fakeDispatcher{}
	gates := NewGates(10)
	defer gates.Close()

	job := Job{ID: "1", ThreadKey: "tg:1"}
	gates.Submit(context.Background(), d, job)

	// Wait briefly for the worker to drain.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		count := len(d.fires)
		d.mu.Unlock()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	d.mu.Lock()
	count := len(d.fires)
	d.mu.Unlock()
	if count != 1 {
		t.Errorf("Fire count=%d want 1", count)
	}
}

func TestGateOverflowDrops(t *testing.T) {
	// Block the dispatcher so the channel fills up.
	release := make(chan struct{})
	d := &blockingDispatcher{release: release}

	gates := NewGates(2)
	defer func() {
		close(release)
		gates.Close()
	}()

	ctx := context.Background()
	gates.Submit(ctx, d, Job{ID: "1", ThreadKey: "tg:1"}) // worker takes this
	// Worker blocks; channel can hold 2 more.
	time.Sleep(20 * time.Millisecond)
	gates.Submit(ctx, d, Job{ID: "2", ThreadKey: "tg:1"})
	gates.Submit(ctx, d, Job{ID: "3", ThreadKey: "tg:1"})
	gates.Submit(ctx, d, Job{ID: "4", ThreadKey: "tg:1"}) // should overflow → drop

	d.mu.Lock()
	dropped := len(d.dropped)
	d.mu.Unlock()
	if dropped != 1 {
		t.Errorf("dropped count=%d want 1", dropped)
	}
}

type blockingDispatcher struct {
	mu      sync.Mutex
	dropped []Job
	release chan struct{}
}

func (b *blockingDispatcher) Fire(ctx context.Context, job Job) error {
	<-b.release
	return nil
}

func (b *blockingDispatcher) NotifyDropped(ctx context.Context, job Job) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dropped = append(b.dropped, job)
}
