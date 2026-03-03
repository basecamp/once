package userstats

import (
	"context"
	"log/slog"
	"sync"

	"github.com/docker/docker/client"
)

type Reader struct {
	containerName string

	mu      sync.RWMutex
	c       copyClient
	summary *Summary
}

func NewReader(namespace string) *Reader {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		slog.Error("Creating Docker client for user stats reader", "error", err)
	}

	return &Reader{
		containerName: namespace + "-proxy",
		c:             c,
	}
}

func (r *Reader) Fetch(service string) *ServiceSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.summary == nil {
		return nil
	}

	s, ok := r.summary.Services[service]
	if !ok {
		return nil
	}

	return &s
}

func (r *Reader) Scrape(ctx context.Context) {
	if r.c == nil {
		return
	}

	summary, err := LoadSummary(ctx, r.c, r.containerName)
	if err != nil || summary == nil {
		return
	}

	r.mu.Lock()
	r.summary = summary
	r.mu.Unlock()
}
