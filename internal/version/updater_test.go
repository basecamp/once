package version

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
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

func TestUpdateBinary_VerifiedUpdate(t *testing.T) {
	key := newReleaseKey(t)
	fakeBinary := []byte("new binary content")
	rel := newTestRelease(t, "v2.0.0", fakeBinary)
	rel.sign(t, key)

	u, execPath := rel.updater(t, "v1.0.0", key)
	require.NoError(t, u.UpdateBinary())

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, fakeBinary, got)
	assertNoLeftovers(t, execPath)
}

func TestUpdateBinary_AcceptsAnyTrustedKey(t *testing.T) {
	current, next := newReleaseKey(t), newReleaseKey(t)
	rel := newTestRelease(t, "v2.0.0", []byte("signed with the next key"))
	rel.sign(t, next)

	u, execPath := rel.updater(t, "v1.0.0", current, next)
	require.NoError(t, u.UpdateBinary())
	assertBinary(t, execPath, "signed with the next key")
}

func TestUpdateBinary_RejectsTamperedBinary(t *testing.T) {
	key := newReleaseKey(t)
	rel := newTestRelease(t, "v2.0.0", []byte("the binary that was signed"))
	rel.sign(t, key)
	rel.assets[binaryAssetName()] = []byte("a different binary")

	u, execPath := rel.updater(t, "v1.0.0", key)
	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the checksum")

	assertBinary(t, execPath, "old")
	assertNoLeftovers(t, execPath)
}

func TestUpdateBinary_RejectsUntrustedKey(t *testing.T) {
	rel := newTestRelease(t, "v2.0.0", []byte("new"))
	rel.sign(t, newReleaseKey(t))

	u, execPath := rel.updater(t, "v1.0.0", newReleaseKey(t))
	err := u.UpdateBinary()
	require.ErrorIs(t, err, ErrBadSignature)
	assertBinary(t, execPath, "old")
}

func TestUpdateBinary_RejectsEditedManifest(t *testing.T) {
	key := newReleaseKey(t)
	rel := newTestRelease(t, "v2.0.0", []byte("the binary that was signed"))
	rel.sign(t, key)

	replacement := []byte("a different binary")
	rel.assets[binaryAssetName()] = replacement
	rel.assets[manifestAssetName] = manifestFor("v2.0.0", map[string][]byte{binaryAssetName(): replacement})

	u, execPath := rel.updater(t, "v1.0.0", key)
	require.ErrorIs(t, u.UpdateBinary(), ErrBadSignature)
	assertBinary(t, execPath, "old")
}

func TestUpdateBinary_RejectsUnsignedRelease(t *testing.T) {
	rel := newTestRelease(t, "v2.0.0", []byte("new"))

	u, execPath := rel.updater(t, "v1.0.0", newReleaseKey(t))
	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no release.txt and release.txt.sig")
	assertBinary(t, execPath, "old")
}

func TestUpdateBinary_RejectsManifestSignedForAnotherVersion(t *testing.T) {
	key := newReleaseKey(t)
	older := newTestRelease(t, "v1.5.0", []byte("an older signed build"))
	older.sign(t, key)

	rel := newTestRelease(t, "v2.0.0", []byte("an older signed build"))
	rel.assets[manifestAssetName] = older.assets[manifestAssetName]
	rel.assets[signatureAssetName] = older.assets[signatureAssetName]

	u, execPath := rel.updater(t, "v1.0.0", key)
	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signed for v1.5.0, not v2.0.0")
	assertBinary(t, execPath, "old")
}

func TestUpdateBinary_RefusesWithoutKeys(t *testing.T) {
	key := newReleaseKey(t)
	rel := newTestRelease(t, "v2.0.0", []byte("new"))
	rel.sign(t, key)

	u, execPath := rel.updater(t, "v1.0.0")
	require.ErrorIs(t, u.UpdateBinary(), ErrNoReleaseKeys)
	assertBinary(t, execPath, "old")
}

func TestUpdateBinary_IgnoresOlderRelease(t *testing.T) {
	key := newReleaseKey(t)
	rel := newTestRelease(t, "v1.0.0", []byte("older"))
	rel.sign(t, key)

	u, execPath := rel.updater(t, "v1.2.0", key)
	require.NoError(t, u.UpdateBinary())
	assertBinary(t, execPath, "old")
	assert.Zero(t, rel.assetRequests.Load())
}

func TestUpdateBinary_RefusesPrereleaseOrUnversionedTags(t *testing.T) {
	key := newReleaseKey(t)
	refuse := func(tag, current string) {
		t.Helper()
		rel := newTestRelease(t, tag, []byte("new"))
		rel.sign(t, key)

		u, execPath := rel.updater(t, current, key)
		require.Error(t, u.UpdateBinary())
		assertBinary(t, execPath, "old")
		assert.Zero(t, rel.assetRequests.Load())
	}

	refuse("v9.9.9-test", "v1.0.0")
	refuse("latest", "v1.0.0")
	refuse("v2.0.0", "dev")
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

	u := newUpdater("v1.0.0", "", badSrv.Client())

	dir := t.TempDir()
	dest := filepath.Join(dir, "tmp")
	_, err := u.downloadBinary(badSrv.URL+"/once-linux-amd64", dest)
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

type testRelease struct {
	tag           string
	assets        map[string][]byte
	assetRequests atomic.Int32
}

func newTestRelease(t *testing.T, tag string, binary []byte) *testRelease {
	t.Helper()
	return &testRelease{
		tag:    tag,
		assets: map[string][]byte{binaryAssetName(): binary},
	}
}

func (r *testRelease) sign(t *testing.T, key *ecdsa.PrivateKey) {
	t.Helper()
	manifest := manifestFor(r.tag, map[string][]byte{binaryAssetName(): r.assets[binaryAssetName()]})
	r.assets[manifestAssetName] = manifest
	r.assets[signatureAssetName] = signManifest(t, key, manifest)
}

// updater serves the release and returns an updater trusting the given keys,
// pointed at a fake installed binary containing "old".
func (r *testRelease) updater(t *testing.T, current string, trusted ...*ecdsa.PrivateKey) (*Updater, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/release" {
			rel := release{TagName: r.tag}
			for name := range r.assets {
				rel.Assets = append(rel.Assets, asset{Name: name, URL: "http://" + req.Host + "/assets/" + name})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rel)
			return
		}

		r.assetRequests.Add(1)
		data, ok := r.assets[strings.TrimPrefix(req.URL.Path, "/assets/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)

	execPath := filepath.Join(t.TempDir(), "once")
	require.NoError(t, os.WriteFile(execPath, []byte("old"), 0755))

	u := newUpdater(current, srv.URL+"/release", srv.Client())
	u.executable = func() (string, error) { return execPath, nil }
	for _, key := range trusted {
		u.keys = append(u.keys, &key.PublicKey)
	}
	return u, execPath
}

func newReleaseKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func manifestFor(tag string, binaries map[string][]byte) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "once %s\ncommit %s\n", tag, strings.Repeat("a", 40))
	for name, data := range binaries {
		sum := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return []byte(b.String())
}

func signManifest(t *testing.T, key *ecdsa.PrivateKey, manifest []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(manifest)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)
	return sig
}

func binaryAssetName() string {
	return fmt.Sprintf("once-%s-%s", runtime.GOOS, runtime.GOARCH)
}

func assertBinary(t *testing.T, execPath, want string) {
	t.Helper()
	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

func assertNoLeftovers(t *testing.T, execPath string) {
	t.Helper()
	dir := filepath.Dir(execPath)
	for _, name := range []string{updateTempFile, filepath.Base(execPath) + ".old"} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.True(t, os.IsNotExist(err), "%s should not exist", name)
	}
}

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
