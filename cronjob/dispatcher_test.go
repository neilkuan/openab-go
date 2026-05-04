package cronjob

import (
	"context"
	"sync"
	"testing"
)

type fakeDispatcher struct {
	mu      sync.Mutex
	fires   []Job
	dropped []Job
}

func (f *fakeDispatcher) Fire(ctx context.Context, job Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fires = append(f.fires, job)
	return nil
}

func (f *fakeDispatcher) NotifyDropped(ctx context.Context, job Job) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropped = append(f.dropped, job)
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	d := &fakeDispatcher{}
	r.Register("tg", d)

	if got := r.Get("tg"); got != d {
		t.Errorf("Get(tg)=%v want %v", got, d)
	}
	if got := r.Get("missing"); got != nil {
		t.Errorf("Get(missing)=%v want nil", got)
	}
}

func TestRegistryPrefixOf(t *testing.T) {
	cases := []struct {
		threadKey string
		prefix    string
	}{
		{"tg:1234", "tg"},
		{"tg:1234:5", "tg"},
		{"discord:abc", "discord"},
		{"teams:room/1", "teams"},
		{"weird", "weird"},
		{"", ""},
	}
	for _, c := range cases {
		if got := PrefixOf(c.threadKey); got != c.prefix {
			t.Errorf("PrefixOf(%q)=%q want %q", c.threadKey, got, c.prefix)
		}
	}
}
