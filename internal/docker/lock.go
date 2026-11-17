package docker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// The lock lives in /tmp rather than a per-user directory because the
// background service runs as root while interactive commands often don't, and
// both must see the same file.
var lockDir = "/tmp"

const (
	lockTimeout      = 5 * time.Minute
	lockPollInterval = 100 * time.Millisecond
)

var ErrLockTimeout = errors.New("timed out waiting for another once process to finish with the proxy")

// lockNamespace takes an exclusive lock shared by every once process on this
// host working in the namespace. The lock is released when the returned
// function is called, or when the process exits.
func lockNamespace(ctx context.Context, namespace string) (func(), error) {
	f, err := openLockFile(filepath.Join(lockDir, "once-"+namespace+".lock"))
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	ctx, cancel := context.WithTimeoutCause(ctx, lockTimeout, ErrLockTimeout)
	defer cancel()

	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("locking: %w", err)
		}

		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("waiting for lock: %w", context.Cause(ctx))
		case <-time.After(lockPollInterval):
		}
	}
}

// Helpers

// openLockFile opens the file read-only, which is all flock needs, so that
// processes running as different users can share a file only one of them
// owns. It avoids O_CREATE on an existing file, which protected_regular
// refuses in sticky directories like /tmp when the file has another owner.
//
// Anyone can create files in /tmp, so it opens only a regular file: never
// through a symlink, which could point a root process at another file, and
// without blocking, which opening a FIFO would do before the lock's timeout
// applies.
func openLockFile(path string) (*os.File, error) {
	for range 10 {
		f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err == nil {
			return checkLockFile(f)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}

		f, err = os.OpenFile(path, os.O_RDONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}

		// The umask may have narrowed the mode; other users need to open it.
		if err := f.Chmod(0o644); err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	}
	return nil, fmt.Errorf("%s keeps appearing and disappearing", path)
}

func checkLockFile(f *os.File) (*os.File, error) {
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%s is not a regular file", f.Name())
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
