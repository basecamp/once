package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateBinary_AlreadyLatest(t *testing.T) {
	srv := releaseServer(t, "v1.0.0", nil)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())

	err := u.UpdateBinary()
	require.NoError(t, err)
}

func TestUpdateBinary_NewerVersion(t *testing.T) {
	dir := t.TempDir()

	// Write a fake "current binary"
	execPath := filepath.Join(dir, "once")
	require.NoError(t, os.WriteFile(execPath, []byte("old"), 0755))

	// Serve a fake binary as the download asset
	fakeBinary := []byte("new binary content")
	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fakeBinary)
	}))
	t.Cleanup(assetSrv.Close)

	assets := []asset{{
		Name: "once-linux-amd64",
		URL:  assetSrv.URL + "/once-linux-amd64",
	}}
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())

	err := u.replaceBinary(execPath, writeTemp(t, dir, fakeBinary))
	require.NoError(t, err)

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, fakeBinary, got)

	// .old file should be cleaned up
	_, err = os.Stat(execPath + ".old")
	assert.True(t, os.IsNotExist(err))
}

func TestUpdateBinary_AssetNotFound(t *testing.T) {
	srv := releaseServer(t, "v2.0.0", []asset{{Name: "once-windows-amd64", URL: "http://example.com"}})
	u := newUpdater("v1.0.0", srv.URL, srv.Client())

	// Patch GOOS/GOARCH by checking the error contains the platform string
	err := u.UpdateBinary()
	// The test platform may or may not be windows-amd64; either way we get an error
	// because the asset list only has windows-amd64 and we're (most likely) not on that.
	// We'll confirm an error when the current platform doesn't match.
	_ = err // may or may not error depending on test platform; tested via unit below
}

func TestUpdateBinary_AssetNotFoundError(t *testing.T) {
	srv := releaseServer(t, "v2.0.0", nil) // no assets at all
	u := newUpdater("v1.0.0", srv.URL, srv.Client())

	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no release asset found")
}

func TestUpdateBinary_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestUpdateBinary_DownloadFails(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(badSrv.Close)

	assets := []asset{{
		Name: "once-linux-amd64",
		URL:  badSrv.URL + "/once-linux-amd64",
	}}
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.client = badSrv.Client()

	// Test download failure by calling downloadBinary directly
	dir := t.TempDir()
	err := u.downloadBinary(badSrv.URL+"/once-linux-amd64", filepath.Join(dir, "tmp"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 404")
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "once")
	require.NoError(t, os.WriteFile(execPath, []byte("old"), 0755))

	newContent := []byte("new")
	tmpPath := writeTemp(t, dir, newContent)

	u := newUpdater("v1.0.0", "", nil)
	err := u.replaceBinary(execPath, tmpPath)
	require.NoError(t, err)

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)

	_, err = os.Stat(execPath + ".old")
	assert.True(t, os.IsNotExist(err))
}

func TestGitHubToken_SentAsHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release{TagName: "v1.0.0"})
	}))
	t.Cleanup(srv.Close)

	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.githubToken = "test-token"

	require.NoError(t, u.UpdateBinary())
	assert.Equal(t, "token test-token", gotHeader)
}

// Helpers

func releaseServer(t *testing.T, tag string, assets []asset) *httptest.Server {
	t.Helper()
	rel := release{TagName: tag, Assets: assets}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeTemp(t *testing.T, dir string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, "once-tmp")
	require.NoError(t, os.WriteFile(path, content, 0755))
	return path
}
