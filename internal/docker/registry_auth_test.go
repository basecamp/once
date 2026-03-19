package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryHostFor(t *testing.T) {
	assert.Equal(t, "ghcr.io", registryHostFor("ghcr.io/basecamp/once:main"))
	assert.Equal(t, "docker.io", registryHostFor("ubuntu"))
	assert.Equal(t, "", registryHostFor(":::bad"))
}

func TestAuthFromInlineEntry(t *testing.T) {
	t.Run("valid base64 with colon separator", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("myuser:mypass"))
		token := authFromInlineEntry(encoded)
		require.NotEmpty(t, token)

		ac := decodeAuthToken(t, token)
		assert.Equal(t, "myuser", ac.Username)
		assert.Equal(t, "mypass", ac.Password)
	})

	t.Run("invalid base64", func(t *testing.T) {
		assert.Equal(t, "", authFromInlineEntry("not-valid-base64!!!"))
	})

	t.Run("base64 with no colon separator", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("nocolon"))
		assert.Equal(t, "", authFromInlineEntry(encoded))
	})
}

func TestEncodeAuthConfig(t *testing.T) {
	token := encodeAuthConfig("alice", "secret")
	require.NotEmpty(t, token)

	decoded := decodeAuthToken(t, token)
	assert.Equal(t, "alice", decoded.Username)
	assert.Equal(t, "secret", decoded.Password)
}

func TestLoadDockerConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &dockerConfigFile{
			Auths:       map[string]dockerAuthEntry{"ghcr.io": {Auth: "dXNlcjpwYXNz"}},
			CredHelpers: map[string]string{"gcr.io": "gcr"},
			CredsStore:  "osxkeychain",
		}
		writeDockerConfig(t, dir, cfg)

		loaded, err := loadDockerConfig(filepath.Join(dir, ".docker", "config.json"))
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, "dXNlcjpwYXNz", loaded.Auths["ghcr.io"].Auth)
		assert.Equal(t, "gcr", loaded.CredHelpers["gcr.io"])
		assert.Equal(t, "osxkeychain", loaded.CredsStore)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := loadDockerConfig("/nonexistent/path/config.json")
		assert.Error(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		require.NoError(t, os.WriteFile(path, []byte("{not json}"), 0600))

		_, err := loadDockerConfig(path)
		assert.Error(t, err)
	})
}

func TestCredHelperServerURL(t *testing.T) {
	assert.Equal(t, "https://index.docker.io/v1/", credHelperServerURL("docker.io"))
	assert.Equal(t, "ghcr.io", credHelperServerURL("ghcr.io"))
	assert.Equal(t, "gcr.io", credHelperServerURL("gcr.io"))
}

func TestAuthEntryFor(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	auths := map[string]dockerAuthEntry{
		"ghcr.io":                        {Auth: encoded},
		"https://index.docker.io/v1/":    {Auth: encoded},
		"https://registry.example.com/":  {Auth: encoded},
	}

	entry, ok := authEntryFor(auths, "ghcr.io")
	assert.True(t, ok)
	assert.Equal(t, encoded, entry.Auth)

	entry, ok = authEntryFor(auths, "docker.io")
	assert.True(t, ok, "should resolve https://index.docker.io/v1/ to docker.io")
	assert.Equal(t, encoded, entry.Auth)

	entry, ok = authEntryFor(auths, "registry.example.com")
	assert.True(t, ok, "should strip scheme and trailing slash from URL keys")
	assert.Equal(t, encoded, entry.Auth)

	_, ok = authEntryFor(auths, "notfound.io")
	assert.False(t, ok)
}

func TestAuthFromCredHelper(t *testing.T) {
	t.Run("valid helper returns JSON", func(t *testing.T) {
		installFakeCredHelper(t, "test-valid", credHelperScript(credHelperResponse{Username: "bob", Secret: "topsecret"}))

		token := authFromCredHelper("test-valid", "ghcr.io")
		require.NotEmpty(t, token)

		ac := decodeAuthToken(t, token)
		assert.Equal(t, "bob", ac.Username)
		assert.Equal(t, "topsecret", ac.Password)
	})

	t.Run("helper exits non-zero", func(t *testing.T) {
		installFakeCredHelper(t, "test-fail", "#!/bin/sh\nexit 1\n")
		assert.Equal(t, "", authFromCredHelper("test-fail", "ghcr.io"))
	})

	t.Run("helper returns malformed JSON", func(t *testing.T) {
		installFakeCredHelper(t, "test-badjson", "#!/bin/sh\necho 'not json'\n")
		assert.Equal(t, "", authFromCredHelper("test-badjson", "ghcr.io"))
	})

	t.Run("helper binary absent", func(t *testing.T) {
		assert.Equal(t, "", authFromCredHelper("nonexistent-helper-xyz", "ghcr.io"))
	})
}

func TestRegistryAuthFor(t *testing.T) {
	t.Run("invalid image string", func(t *testing.T) {
		fakeHome(t)
		assert.Equal(t, "", registryAuthFor(":::bad"))
	})

	t.Run("no docker config in fake HOME", func(t *testing.T) {
		fakeHome(t)
		assert.Equal(t, "", registryAuthFor("ghcr.io/basecamp/once:main"))
	})

	t.Run("config has credHelpers for host", func(t *testing.T) {
		home := fakeHome(t)
		installFakeCredHelper(t, "myhelper", credHelperScript(credHelperResponse{Username: "helper-user", Secret: "helper-pass"}))
		writeDockerConfig(t, home, &dockerConfigFile{
			CredHelpers: map[string]string{"ghcr.io": "myhelper"},
		})

		token := registryAuthFor("ghcr.io/basecamp/once:main")
		require.NotEmpty(t, token)
		ac := decodeAuthToken(t, token)
		assert.Equal(t, "helper-user", ac.Username)
		assert.Equal(t, "helper-pass", ac.Password)
	})

	t.Run("config has credsStore only", func(t *testing.T) {
		home := fakeHome(t)
		installFakeCredHelper(t, "mystore", credHelperScript(credHelperResponse{Username: "store-user", Secret: "store-pass"}))
		writeDockerConfig(t, home, &dockerConfigFile{CredsStore: "mystore"})

		token := registryAuthFor("ghcr.io/basecamp/once:main")
		require.NotEmpty(t, token)
		ac := decodeAuthToken(t, token)
		assert.Equal(t, "store-user", ac.Username)
		assert.Equal(t, "store-pass", ac.Password)
	})

	t.Run("credHelpers wins over credsStore", func(t *testing.T) {
		home := fakeHome(t)
		installFakeCredHelper(t, "specific-helper", credHelperScript(credHelperResponse{Username: "helper-user", Secret: "helper-pass"}))
		installFakeCredHelper(t, "global-store", credHelperScript(credHelperResponse{Username: "store-user", Secret: "store-pass"}))
		writeDockerConfig(t, home, &dockerConfigFile{
			CredHelpers: map[string]string{"ghcr.io": "specific-helper"},
			CredsStore:  "global-store",
		})

		token := registryAuthFor("ghcr.io/basecamp/once:main")
		require.NotEmpty(t, token)
		ac := decodeAuthToken(t, token)
		assert.Equal(t, "helper-user", ac.Username)
		assert.Equal(t, "helper-pass", ac.Password)
	})

	t.Run("config has inline auths entry", func(t *testing.T) {
		home := fakeHome(t)
		encoded := base64.StdEncoding.EncodeToString([]byte("inline-user:inline-pass"))
		writeDockerConfig(t, home, &dockerConfigFile{
			Auths: map[string]dockerAuthEntry{"ghcr.io": {Auth: encoded}},
		})

		token := registryAuthFor("ghcr.io/basecamp/once:main")
		require.NotEmpty(t, token)
		ac := decodeAuthToken(t, token)
		assert.Equal(t, "inline-user", ac.Username)
		assert.Equal(t, "inline-pass", ac.Password)
	})

	t.Run("credHelpers entry but helper fails - no fallback", func(t *testing.T) {
		home := fakeHome(t)
		installFakeCredHelper(t, "failing-helper", "#!/bin/sh\nexit 1\n")
		writeDockerConfig(t, home, &dockerConfigFile{
			CredHelpers: map[string]string{"ghcr.io": "failing-helper"},
		})

		assert.Equal(t, "", registryAuthFor("ghcr.io/basecamp/once:main"))
	})

	t.Run("no matching entry for host", func(t *testing.T) {
		home := fakeHome(t)
		writeDockerConfig(t, home, &dockerConfigFile{
			Auths: map[string]dockerAuthEntry{"docker.io": {Auth: "dXNlcjpwYXNz"}},
		})

		assert.Equal(t, "", registryAuthFor("ghcr.io/basecamp/once:main"))
	})
}

// Helpers

func fakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func writeDockerConfig(t *testing.T, dir string, cfg *dockerConfigFile) {
	t.Helper()
	dockerDir := filepath.Join(dir, ".docker")
	require.NoError(t, os.MkdirAll(dockerDir, 0700))
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dockerDir, "config.json"), data, 0600))
}

func installFakeCredHelper(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-credential-"+name), []byte(script), 0755))
}

func credHelperScript(response credHelperResponse) string {
	data, _ := json.Marshal(response)
	return fmt.Sprintf("#!/bin/sh\necho '%s'\n", data)
}

func decodeAuthToken(t *testing.T, token string) encodedAuthConfig {
	t.Helper()
	data, err := base64.URLEncoding.DecodeString(token)
	require.NoError(t, err)
	var ac encodedAuthConfig
	require.NoError(t, json.Unmarshal(data, &ac))
	return ac
}
