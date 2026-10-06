package version

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedReleaseKeys(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "release-key.pub"))
	require.NoError(t, err)
	assert.Equal(t, string(canonical), releaseKeys, "release_keys.go must match release-key.pub")

	_, err = parseReleaseKeys(releaseKeys)
	require.NoError(t, err)
}

func TestParseReleaseKeys(t *testing.T) {
	first, second := newReleaseKey(t), newReleaseKey(t)
	keys, err := parseReleaseKeys(publicKeyPEM(t, &first.PublicKey) + publicKeyPEM(t, &second.PublicKey))
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.True(t, keys[0].Equal(&first.PublicKey))
	assert.True(t, keys[1].Equal(&second.PublicKey))

	keys, err = parseReleaseKeys("no PEM here")
	require.NoError(t, err)
	assert.Empty(t, keys)

	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, err = parseReleaseKeys(publicKeyPEM(t, edPub))
	require.Error(t, err)
}

func TestParseManifest(t *testing.T) {
	sum := strings.Repeat("0", 64)
	commit := "commit " + strings.Repeat("a", 40) + "\n"

	m, err := parseManifest([]byte("once v1.2.3\n" + commit + sum + "  once-linux-amd64\n"))
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", m.Version)
	assert.Equal(t, strings.Repeat("a", 40), m.Commit)
	assert.Equal(t, map[string]string{"once-linux-amd64": sum}, m.Checksums)

	malformed := func(data string) {
		t.Helper()
		_, err := parseManifest([]byte(data))
		require.ErrorIs(t, err, ErrMalformedRelease)
	}
	malformed("")
	malformed("once v1.2.3\n" + commit)
	malformed("v1.2.3\n" + commit + sum + "  once-linux-amd64\n")
	malformed("once v1.2.3\ncommit abc\n" + sum + "  once-linux-amd64\n")
	malformed("once v1.2.3\n" + commit + sum + " once-linux-amd64\n")
	malformed("once v1.2.3\n" + commit + sum + "  once-linux-amd64\n" + sum + "  once-linux-amd64\n")
}

// release.yml signs with `openssl dgst -sha256 -sign`; the updater must accept
// exactly what that produces.
func TestVerifyManifest_OpenSSLSignature(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not installed")
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "release-key.pem")
	manifestPath := filepath.Join(dir, "release.txt")
	sigPath := filepath.Join(dir, "release.txt.sig")

	openssl(t, "ecparam", "-name", "prime256v1", "-genkey", "-noout", "-out", keyPath)
	pub := openssl(t, "pkey", "-in", keyPath, "-pubout")

	manifest := manifestFor("v1.2.3", map[string][]byte{"once-linux-amd64": []byte("binary")})
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o600))
	openssl(t, "dgst", "-sha256", "-sign", keyPath, "-out", sigPath, manifestPath)

	signature, err := os.ReadFile(sigPath)
	require.NoError(t, err)
	keys, err := parseReleaseKeys(pub)
	require.NoError(t, err)

	m, err := verifyManifest(manifest, signature, keys)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", m.Version)

	_, err = verifyManifest(append(manifest, '\n'), signature, keys)
	require.ErrorIs(t, err, ErrBadSignature)
}

func TestIsNewerRelease(t *testing.T) {
	newer := func(candidate, current string) bool {
		t.Helper()
		ok, err := isNewerRelease(candidate, current)
		require.NoError(t, err)
		return ok
	}
	refused := func(candidate, current string) {
		t.Helper()
		_, err := isNewerRelease(candidate, current)
		require.Error(t, err)
	}

	assert.True(t, newer("v0.4.0", "v0.3.3"))
	assert.True(t, newer("v0.10.0", "v0.9.9"))
	assert.True(t, newer("v1.0.0", "v0.99.99"))
	assert.True(t, newer("v0.3.3", "v0.3.3-2-gabcdef0"))

	assert.False(t, newer("v0.3.3", "v0.3.3"))
	assert.False(t, newer("v0.3.2", "v0.3.3"))
	assert.False(t, newer("v0.0.1", "v0.3.3"))

	refused("v0.4.0-rc1", "v0.3.3")
	refused("v0.0.1-test", "v0.3.3")
	refused("0.4.0", "v0.3.3")
	refused("v0.4", "v0.3.3")
	refused("v0.4.0", "dev")
	refused("v0.4.0", "abc1234")
}

// Helpers

func publicKeyPEM(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func openssl(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("openssl", args...).Output()
	require.NoError(t, err, "openssl %s", strings.Join(args, " "))
	return string(out)
}
