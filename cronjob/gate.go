package cronjob

import (
	"context"
	"log/slog"
	"sync"
)

// Gates owns a per-thread bounded fire channel + worker goroutine
// pair. Submit is non-blocking: if the channel is full, the job is
// dropped via Dispatcher.NotifyDropped.
//
// One worker goroutine per thread reads the channel and calls
// Dispatcher.Fire synchronously, ensuring per-thread FIFO ordering
// across both the queue and the dispatcher call. Close() shuts down
// every worker.
type Gates struct {
	queueSize int

	mu sync.RWMutex
	g  map[string]*gate

	wg sync.WaitGroup
}

type gate struct {
	ch chan submission
}

type submission struct {
	job Job
	d   Dispatcher
	ctx context.Context
}

func NewGates(queueSize int) *Gates {
	if queueSize <= 0 {
		queueSize = 50
	}
	return &Gates{queueSize: queueSize, g: map[string]*gate{}}
}

// Submit hands a job to the per-thread worker. Non-blocking: returns
// immediately if the worker is busy and the channel has room, or
// drops the job (via d.NotifyDropped) when the channel is full.
func (gs *Gates) Submit(ctx context.Context, d Dispatcher, job Job) {
	g := gs.gateFor(job.ThreadKey)
	select {
	case g.ch <- submission{job: job, d: d, ctx: ctx}:
		return
	default:
		slog.Warn("cron fire dropped: per-thread queue full",
			"thread_key", job.ThreadKey, "job_id", job.ID, "queue_size", gs.queueSize)
		d.NotifyDropped(ctx, job)
	}
}

func (gs *Gates) gateFor(threadKey string) *gate {
	gs.mu.RLock()
	g, ok := gs.g[threadKey]
	gs.mu.RUnlock()
	if ok {
		return g
	}

	gs.mu.Lock()
	defer gs.mu.Unlock()
	if g, ok := gs.g[threadKey]; ok {
		return g
	}
	g = &gate{ch: make(chan submission, gs.queueSize)}
	gs.g[threadKey] = g
	gs.wg.Add(1)
	go gs.runWorker(threadKey, g)
	return g
}

func (gs *Gates) runWorker(threadKey string, g *gate) {
	defer gs.wg.Done()
	for sub := range g.ch {
		if err := sub.d.Fire(sub.ctx, sub.job); err != nil {
			slog.Warn("cron dispatcher Fire returned error",
				"thread_key", threadKey, "job_id", sub.job.ID, "error", err)
		}
	}
}

// Close stops every worker by closing each gate's channel and waiting
// for in-flight Fires to complete.
func (gs *Gates) Close() {
	gs.mu.Lock()
	for _, g := range gs.g {
		close(g.ch)
	}
	gs.g = map[string]*gate{}
	gs.mu.Unlock()
	gs.wg.Wait()
}
