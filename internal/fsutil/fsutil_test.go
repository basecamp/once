package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenFile_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "file.txt")

	f, err := OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	f.Close()

	assertOwnership(t, filepath.Dir(path))
	assertOwnership(t, path)
}

func TestOpenFile_ExistingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")

	f, err := OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	f.Close()

	assertOwnership(t, path)
}

func TestOpenFile_AppendsToExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	f, err := OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	f.Write([]byte(" world"))
	f.Close()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(content))
}

func TestCreateFile_RefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("old content"), 0o644))

	_, err := CreateFile(path)
	require.ErrorIs(t, err, fs.ErrExist)

	assertContent(t, path, "old content")
}

func TestCreateFile_DoesNotFollowSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o644))

	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.Symlink(target, path))

	_, err := CreateFile(path)
	require.ErrorIs(t, err, fs.ErrExist)

	assertContent(t, target, "original")
}

func TestCreateFile_DoesNotFollowDanglingSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")

	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.Symlink(target, path))

	_, err := CreateFile(path)
	require.ErrorIs(t, err, fs.ErrExist)

	assert.NoFileExists(t, target)
}

func TestCreateFile_FollowsSymlinkedDirectory(t *testing.T) {
	target := t.TempDir()
	dir := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, os.Symlink(target, dir))

	f, err := CreateFile(filepath.Join(dir, "sub", "file.txt"))
	require.NoError(t, err)
	f.Close()

	assert.FileExists(t, filepath.Join(target, "sub", "file.txt"))
}

func TestCreateFile_NewFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")

	f, err := CreateFile(path)
	require.NoError(t, err)
	f.Close()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestOpenFile_UnwritableParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to any directory")
	}

	dir := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	_, err := OpenFile(filepath.Join(dir, "sub", "file.txt"), os.O_CREATE|os.O_WRONLY, 0o644)
	require.Error(t, err)
}

func TestCreateFile_OwnedByDirectoryOwner(t *testing.T) {
	dir := dirOwnedByAnotherUser(t)
	path := filepath.Join(dir, "a", "b", "file.txt")

	f, err := CreateFile(path)
	require.NoError(t, err)
	f.Close()

	assertOwnedBy(t, filepath.Join(dir, "a"), anotherUser)
	assertOwnedBy(t, filepath.Join(dir, "a", "b"), anotherUser)
	assertOwnedBy(t, path, anotherUser)
}

func TestCreateFile_DoesNotTakeOverSymlinkTargetInAnotherUsersDir(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))

	path := filepath.Join(dirOwnedByAnotherUser(t), "file.txt")
	require.NoError(t, os.Symlink(target, path))

	_, err := CreateFile(path)
	require.Error(t, err)

	assertContent(t, target, "original")
	assertOwnedBy(t, target, 0)
}

func TestOpenFile_LeavesOwnershipOfExistingFile(t *testing.T) {
	path := filepath.Join(dirOwnedByAnotherUser(t), "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	f, err := OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	f.Close()

	assertOwnedBy(t, path, 0)
}

func TestCreateFile_FailsFastOnFIFOInPlaceOfDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, syscall.Mkfifo(dir, 0o644))

	err := createWithin(t, filepath.Join(dir, "file.txt"))
	require.Error(t, err)
}

func TestCreateFile_FailsFastOnFIFOSwappedInAfterMkdir(t *testing.T) {
	dir := t.TempDir()
	afterMkdirHook(t, func(name string) {
		assert.NoError(t, os.Remove(filepath.Join(dir, name)))
		assert.NoError(t, syscall.Mkfifo(filepath.Join(dir, name), 0o644))
	})

	err := createWithin(t, filepath.Join(dir, "backups", "file.txt"))
	require.Error(t, err)
}

func TestCreateFile_DoesNotChownNonEmptyDirectorySwappedInAfterMkdir(t *testing.T) {
	dir := dirOwnedByAnotherUser(t)
	vault := filepath.Join(dir, "vault")
	require.NoError(t, os.Mkdir(vault, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "secret"), []byte("secret"), 0o644))

	assertRefusesSwappedInDirectory(t, dir, "vault")
	assertOwnedBy(t, vault, 0)
}

func TestCreateFile_DoesNotChownThirdPartyDirectorySwappedInAfterMkdir(t *testing.T) {
	dir := dirOwnedByAnotherUser(t)
	vault := filepath.Join(dir, "vault")
	require.NoError(t, os.Mkdir(vault, 0o700))
	require.NoError(t, os.Chown(vault, thirdUser, thirdUser))

	assertRefusesSwappedInDirectory(t, dir, "vault")
	assertOwnedBy(t, vault, thirdUser)
}

func TestCreateFile_ChownsGroupOfEmptyDirectoryAlreadyOwnedByOwner(t *testing.T) {
	dir := dirOwnedByAnotherUser(t)
	afterMkdirHook(t, func(name string) {
		assert.NoError(t, os.Chown(filepath.Join(dir, name), anotherUser, 0))
	})

	f, err := CreateFile(filepath.Join(dir, "backups", "file.txt"))
	require.NoError(t, err)
	f.Close()

	info, err := os.Stat(filepath.Join(dir, "backups"))
	require.NoError(t, err)
	assert.Equal(t, uint32(anotherUser), info.Sys().(*syscall.Stat_t).Gid)
}

// Helpers

const (
	anotherUser = 65534
	thirdUser   = 65533
)

func dirOwnedByAnotherUser(t *testing.T) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("changing ownership to another user requires root")
	}

	dir := filepath.Join(t.TempDir(), "other")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Chown(dir, anotherUser, anotherUser))
	return dir
}

func afterMkdirHook(t *testing.T, fn func(name string)) {
	t.Helper()
	afterMkdir = func(name string) {
		afterMkdir = nil
		fn(name)
	}
	t.Cleanup(func() { afterMkdir = nil })
}

func assertRefusesSwappedInDirectory(t *testing.T, dir, target string) {
	t.Helper()
	afterMkdirHook(t, func(name string) {
		assert.NoError(t, os.Rename(filepath.Join(dir, name), filepath.Join(dir, "moved")))
		assert.NoError(t, os.Symlink(target, filepath.Join(dir, name)))
	})

	_, err := CreateFile(filepath.Join(dir, "backups", "file.txt"))
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, target, "file.txt"))
}

func createWithin(t *testing.T, path string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		f, err := CreateFile(path)
		if f != nil {
			f.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("CreateFile blocked")
		return nil
	}
}

func assertOwnedBy(t *testing.T, path string, uid int) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, uid, int(info.Sys().(*syscall.Stat_t).Uid))
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, string(content))
}

func assertOwnership(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	stat := info.Sys().(*syscall.Stat_t)
	assert.Equal(t, os.Getuid(), int(stat.Uid))
	assert.Equal(t, os.Getgid(), int(stat.Gid))
}
