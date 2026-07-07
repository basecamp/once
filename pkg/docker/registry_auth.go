package docker

import (
	"encoding/base64"
	"encoding/json"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

// registryAuthFor returns a base64-encoded JSON auth string suitable for use in
// image.PullOptions.RegistryAuth. Returns "" on any error or missing credentials.
func registryAuthFor(imageName string, credentials *RegistryCredentials) string {
	if !registryCredentialsEmpty(credentials) {
		return registryAuthToken(&authn.AuthConfig{
			Username: credentials.Username,
			Password: credentials.Password,
		})
	}

	ref, err := name.ParseReference(imageName)
	if err != nil {
		return ""
	}
	authenticator, err := authn.DefaultKeychain.Resolve(ref.Context())
	if err != nil || authenticator == authn.Anonymous {
		return ""
	}
	cfg, err := authenticator.Authorization()
	if err != nil {
		return ""
	}
	return registryAuthToken(cfg)
}

func registryAuthToken(cfg *authn.AuthConfig) string {
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}
