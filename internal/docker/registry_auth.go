package docker

import (
	"encoding/base64"
	"encoding/json"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

// registryAuthFor returns a base64-encoded JSON auth string for the registry
// that hosts the given image, suitable for use in image.PullOptions.RegistryAuth.
// Credentials in the given RegistrySettings take precedence over the Docker
// credential store. Returns "" on any error or missing credentials, falling
// back to anonymous access.
func registryAuthFor(imageName string, registry RegistrySettings) string {
	if !registry.Empty() {
		return encodeAuthConfig(&authn.AuthConfig{
			Username: registry.Username,
			Password: registry.Password,
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
	return encodeAuthConfig(cfg)
}

// registryHostFor returns the registry hostname for the given image, in the
// form a user would pass to `docker login`.
func registryHostFor(imageName string) string {
	ref, err := name.ParseReference(imageName)
	if err != nil {
		return "the registry"
	}
	registry := ref.Context().RegistryStr()
	if registry == name.DefaultRegistry {
		return "docker.io"
	}
	return registry
}

func encodeAuthConfig(cfg *authn.AuthConfig) string {
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}
