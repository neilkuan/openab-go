package cronjob

import (
	"context"
	"strings"
	"sync"
)

// Dispatcher fans a fire-due job out into a chat platform.
//
// Fire is responsible for the full chat-side flow: post a placeholder
// message, build the sender_context envelope (merging CronFields into
// the platform's normal envelope), call SessionPool.GetOrCreate, and
// stream the reply by editing the placeholder. Errors are logged by
// the implementation; the scheduler loops on regardless.
//
// NotifyDropped is called when the per-thread bounded gate is full
// and the job had to be dropped. Implementations should post a brief
// chat marker so the user knows the fire was lost. NotifyDropped must
// not block — implementations should send asynchronously if the
// platform API is slow.
type Dispatcher interface {
	Fire(ctx context.Context, job Job) error
	NotifyDropped(ctx context.Context, job Job)
}

// Registry maps a thread-key prefix ("tg" / "discord" / "teams") to
// the Dispatcher that owns that platform's chats. Platform adapters
// register themselves during Start().
type Registry struct {
	mu sync.RWMutex
	m  map[string]Dispatcher
}

func NewRegistry() *Registry {
	return &Registry{m: map[string]Dispatcher{}}
}

func (r *Registry) Register(prefix string, d Dispatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[prefix] = d
}

func (r *Registry) Get(prefix string) Dispatcher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[prefix]
}

// PrefixOf returns the substring before the first ':' in a threadKey,
// or the threadKey itself if no ':' is present.
func PrefixOf(threadKey string) string {
	if i := strings.IndexByte(threadKey, ':'); i >= 0 {
		return threadKey[:i]
	}
	return threadKey
}
