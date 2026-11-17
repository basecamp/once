package docker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockNamespace(t *testing.T) {
	lockDir = t.TempDir()
	t.Cleanup(func() { lockDir = "/tmp" })

	t.Run("is exclusive until released", func(t *testing.T) {
		unlock, err := lockNamespace(context.Background(), "once")
		require.NoError(t, err)

		acquired := make(chan func())
		go func() {
			second, err := lockNamespace(context.Background(), "once")
			assert.NoError(t, err)
			acquired <- second
		}()

		select {
		case <-acquired:
			t.Fatal("second lock acquired while the first was held")
		case <-time.After(300 * time.Millisecond):
		}

		unlock()

		select {
		case second := <-acquired:
			second()
		case <-time.After(5 * time.Second):
			t.Fatal("second lock not acquired after the first was released")
		}
	})

	t.Run("namespaces lock independently", func(t *testing.T) {
		unlock, err := lockNamespace(context.Background(), "once")
		require.NoError(t, err)
		defer unlock()

		other, err := lockNamespace(context.Background(), "staging")
		require.NoError(t, err)
		other()
	})

	t.Run("gives up when the context ends", func(t *testing.T) {
		unlock, err := lockNamespace(context.Background(), "once")
		require.NoError(t, err)
		defer unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		_, err = lockNamespace(ctx, "once")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
