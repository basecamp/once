package docker

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
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

func TestOpenLockFile(t *testing.T) {
	t.Run("is readable by other users whatever the umask", func(t *testing.T) {
		old := syscall.Umask(0o077)
		defer syscall.Umask(old)

		path := filepath.Join(t.TempDir(), "once.lock")
		f, err := openLockFile(path)
		require.NoError(t, err)
		f.Close()

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	})

	t.Run("opens an existing file it can only read", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "once.lock")
		require.NoError(t, os.WriteFile(path, nil, 0o444))

		f, err := openLockFile(path)
		require.NoError(t, err)
		f.Close()
	})

	t.Run("refuses a symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		require.NoError(t, os.WriteFile(target, nil, 0o644))
		path := filepath.Join(dir, "once.lock")
		require.NoError(t, os.Symlink(target, path))

		_, err := openLockFile(path)
		assert.ErrorIs(t, err, syscall.ELOOP)
	})

	t.Run("refuses a FIFO without blocking", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "once.lock")
		require.NoError(t, syscall.Mkfifo(path, 0o644))

		done := make(chan error, 1)
		go func() {
			f, err := openLockFile(path)
			if f != nil {
				f.Close()
			}
			done <- err
		}()

		select {
		case err := <-done:
			assert.ErrorContains(t, err, "not a regular file")
		case <-time.After(5 * time.Second):
			t.Fatal("opening a FIFO blocked")
		}
	})
}
