package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupToFile_EmptyDir(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "chat"}}
	err := app.BackupToFile(context.Background(), "", "backup.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup location is required")
}

func TestBackupToFile_RelativePath(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "chat"}}
	err := app.BackupToFile(context.Background(), "relative/path", "backup.tar.gz")
	require.ErrorIs(t, err, ErrBackupPathRelative)
}

func TestBackup_EmptyPath(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "chat"}}
	err := app.Backup(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup location is required")
}

func TestBackupName(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{Name: "myapp"},
	}

	name := app.BackupName()
	assert.Contains(t, name, "myapp-")
	assert.True(t, len(name) > len("myapp-.tar.gz"))
	assert.Contains(t, name, ".tar.gz")

	// Verify the embedded timestamp is parseable
	_, ok := parseBackupTime("myapp", name)
	assert.True(t, ok)
}

func TestParseBackupTime(t *testing.T) {
	t.Run("valid backup name", func(t *testing.T) {
		ts, ok := parseBackupTime("myapp", "myapp-20250115-093000.tar.gz")
		require.True(t, ok)
		assert.Equal(t, time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC), ts)
	})

	t.Run("wrong app name", func(t *testing.T) {
		_, ok := parseBackupTime("other", "myapp-20250115-093000.tar.gz")
		assert.False(t, ok)
	})

	t.Run("unrelated file", func(t *testing.T) {
		_, ok := parseBackupTime("myapp", "unrelated.txt")
		assert.False(t, ok)
	})

	t.Run("bad date", func(t *testing.T) {
		_, ok := parseBackupTime("myapp", "myapp-baddate.tar.gz")
		assert.False(t, ok)
	})
}

func TestTrimBackups(t *testing.T) {
	dir := t.TempDir()

	app := &Application{
		Settings: ApplicationSettings{
			Name:   "myapp",
			Backup: BackupSettings{Path: dir},
		},
	}

	oldTime := time.Now().Add(-BackupRetention - time.Hour)
	recentTime := time.Now().Add(-time.Minute)

	createFile := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644))
	}

	oldFile := fmt.Sprintf("myapp-%s.tar.gz", oldTime.Format("20060102-150405"))
	recentFile := fmt.Sprintf("myapp-%s.tar.gz", recentTime.Format("20060102-150405"))
	unrelatedFile := "notes.txt"

	createFile(oldFile)
	createFile(recentFile)
	createFile(unrelatedFile)

	err := app.TrimBackups()
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, oldFile))
	assert.FileExists(t, filepath.Join(dir, recentFile))
	assert.FileExists(t, filepath.Join(dir, unrelatedFile))
}

func TestWriteTarEntry(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := writeTarEntry(tw, "test.txt", []byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	tr := tar.NewReader(&buf)
	header, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "test.txt", header.Name)
	assert.Equal(t, int64(len("hello world")), header.Size)
}

func TestCopyTarEntriesWithPrefix(t *testing.T) {
	// Build source tar with entries under "storage/"
	var src bytes.Buffer
	tw := tar.NewWriter(&src)
	require.NoError(t, writeTarEntry(tw, "storage/file.txt", []byte("content")))
	require.NoError(t, tw.Close())

	// Copy with prefix rewrite
	var dst bytes.Buffer
	dstTw := tar.NewWriter(&dst)
	err := copyTarEntriesWithPrefix(&src, dstTw, "storage", "data")
	require.NoError(t, err)
	require.NoError(t, dstTw.Close())

	// Verify renamed entry
	tr := tar.NewReader(&dst)
	header, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "data/file.txt", header.Name)
}

func TestWriteTarEntryAndCopyRoundTrip(t *testing.T) {
	// Build a complete backup-like tar
	var backupBuf bytes.Buffer
	gw := gzip.NewWriter(&backupBuf)
	tw := tar.NewWriter(gw)

	require.NoError(t, writeTarEntry(tw, backupAppSettingsEntry, []byte(`{"name":"app"}`)))
	require.NoError(t, writeTarEntry(tw, backupVolSettingsEntry, []byte(`{"secretKeyBase":"secret","vapidPublicKey":"pub123","vapidPrivateKey":"priv456"}`)))
	require.NoError(t, writeTarEntry(tw, "data/file.txt", []byte("file content")))

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	// Parse the backup
	ns := &Namespace{name: "test"}
	appSettings, volSettings, volumeData, err := ns.parseBackup(&backupBuf)
	require.NoError(t, err)
	assert.Equal(t, "app", appSettings.Name)
	assert.Equal(t, "secret", volSettings.SecretKeyBase)
	assert.Equal(t, "pub123", volSettings.VAPIDPublicKey)
	assert.Equal(t, "priv456", volSettings.VAPIDPrivateKey)
	assert.NotEmpty(t, volumeData)
}
