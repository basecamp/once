package docker

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type EventWatcher struct {
	client    *client.Client
	namespace string
}

func NewEventWatcher(c *client.Client, namespace string) *EventWatcher {
	return &EventWatcher{
		client:    c,
		namespace: namespace,
	}
}

// Watch returns a channel that signals when a container in this namespace
// changes state. The channel is closed when the context is cancelled.
func (w *EventWatcher) Watch(ctx context.Context) <-chan struct{} {
	out := make(chan struct{})

	go func() {
		defer close(out)
		w.watchLoop(ctx, out)
	}()

	return out
}

// Private

func (w *EventWatcher) watchLoop(ctx context.Context, out chan<- struct{}) {
	for {
		w.streamEvents(ctx, out)

		select {
		case <-ctx.Done():
			return
		case <-time.After(streamRetryDelay): // Pause before retrying
		}
	}
}

func (w *EventWatcher) streamEvents(ctx context.Context, out chan<- struct{}) {
	filterArgs := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "start"),
		filters.Arg("event", "stop"),
		filters.Arg("event", "die"),
		filters.Arg("event", "restart"),
	)

	eventChan, errChan := w.client.Events(ctx, events.ListOptions{
		Filters: filterArgs,
	})

	prefix := w.namespace + "-"

	for {
		select {
		case <-ctx.Done():
			return
		case <-errChan:
			return
		case event := <-eventChan:
			name := event.Actor.Attributes["name"]
			if strings.HasPrefix(name, prefix) {
				select {
				case out <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}
