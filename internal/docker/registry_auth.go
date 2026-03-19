package docker

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/distribution/reference"
)

type dockerConfigFile struct {
	Auths       map[string]dockerAuthEntry `json:"auths"`
	CredHelpers map[string]string          `json:"credHelpers"`
	CredsStore  string                     `json:"credsStore"`
}

type dockerAuthEntry struct {
	Auth string `json:"auth"`
}

type credHelperResponse struct {
	Username string `json:"Username"`
	Secret   string `json:"Secret"`
}

type encodedAuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// registryAuthFor returns a base64-encoded JSON auth string for the registry
// that hosts the given image, suitable for use in image.PullOptions.RegistryAuth.
// Returns "" on any error or missing credentials, falling back to anonymous access.
func registryAuthFor(imageName string) string {
	host := registryHostFor(imageName)
	if host == "" {
		return ""
	}

	cfg, err := loadDockerConfig(dockerConfigPath())
	if err != nil {
		return ""
	}

	if helper, ok := cfg.CredHelpers[host]; ok {
		return authFromCredHelper(helper, host)
	}

	if cfg.CredsStore != "" {
		return authFromCredHelper(cfg.CredsStore, host)
	}

	if entry, ok := authEntryFor(cfg.Auths, host); ok && entry.Auth != "" {
		return authFromInlineEntry(entry.Auth)
	}

	return ""
}

// Helpers

func dockerConfigPath() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

func registryHostFor(imageName string) string {
	named, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return ""
	}
	return reference.Domain(named)
}

func loadDockerConfig(configPath string) (*dockerConfigFile, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg dockerConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// authEntryFor looks up the auth entry for host in auths, handling keys stored
// as full URLs (e.g. https://index.docker.io/v1/) in addition to bare hostnames.
func authEntryFor(auths map[string]dockerAuthEntry, host string) (dockerAuthEntry, bool) {
	if entry, ok := auths[host]; ok {
		return entry, true
	}
	for key, entry := range auths {
		if u, err := url.Parse(key); err == nil && u.Host != "" {
			if canonicalHost(u.Host) == host {
				return entry, true
			}
		}
	}
	return dockerAuthEntry{}, false
}

func canonicalHost(host string) string {
	switch strings.ToLower(host) {
	case "index.docker.io", "registry-1.docker.io":
		return "docker.io"
	default:
		return strings.ToLower(host)
	}
}

func authFromCredHelper(helper, serverURL string) string {
	cmd := exec.Command("docker-credential-"+helper, "get")
	cmd.Stdin = strings.NewReader(serverURL)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var resp credHelperResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return ""
	}
	return encodeAuthConfig(resp.Username, resp.Secret)
}

func authFromInlineEntry(encodedAuth string) string {
	decoded, err := base64.StdEncoding.DecodeString(encodedAuth)
	if err != nil {
		return ""
	}
	username, password, found := strings.Cut(string(decoded), ":")
	if !found {
		return ""
	}
	return encodeAuthConfig(username, password)
}

func encodeAuthConfig(username, password string) string {
	data, err := json.Marshal(encodedAuthConfig{Username: username, Password: password})
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}
