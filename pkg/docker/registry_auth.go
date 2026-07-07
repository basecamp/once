package docker

import (
	"encoding/base64"
	"encoding/json"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

// registryAuthFor returns a base64-encoded JSON auth string for the registry
// that hosts the given image, suitable for use in image.PullOptions.RegistryAuth.
// Returns "" on any error or missing credentials, falling back to anonymous access.
func registryAuthFor(imageName string) string {
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
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}
