package fsutil

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

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

func TestCreateFile_TruncatesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("old content"), 0o644))

	f, err := CreateFile(path)
	require.NoError(t, err)
	f.Write([]byte("new"))
	f.Close()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
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

func TestOpenFile_NewDirPermissionsFollowTheFile(t *testing.T) {
	assertDirPerm := func(perm, want os.FileMode) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "a", "b", "file.txt")

		f, err := OpenFile(path, os.O_CREATE|os.O_WRONLY, perm)
		require.NoError(t, err)
		f.Close()

		for _, dir := range []string{filepath.Dir(path), filepath.Dir(filepath.Dir(path))} {
			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, want, info.Mode().Perm(), dir)
			require.NoError(t, os.Chmod(dir, 0o700)) // so t.TempDir can clean up
		}
	}

	assertDirPerm(0o644, 0o755)
	assertDirPerm(0o600, 0o700)
	assertDirPerm(0o200, 0o300)
}

func TestCreateFile_NewDirIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backups", "file.tar.gz")

	f, err := CreateFile(path)
	require.NoError(t, err)
	f.Close()

	info, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestOpenFile_ExistingDirIsLeftAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	require.NoError(t, os.Mkdir(dir, 0o755))

	f, err := CreateFile(filepath.Join(dir, "file.tar.gz"))
	require.NoError(t, err)
	f.Close()

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestOpenFile_UnwritableParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	_, err := OpenFile(filepath.Join(dir, "sub", "file.txt"), os.O_CREATE|os.O_WRONLY, 0o644)
	require.Error(t, err)
}

// Helpers

func assertOwnership(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	stat := info.Sys().(*syscall.Stat_t)
	assert.Equal(t, os.Getuid(), int(stat.Uid))
	assert.Equal(t, os.Getgid(), int(stat.Gid))
}
