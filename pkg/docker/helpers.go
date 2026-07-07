package docker

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/distribution/reference"
)

const streamRetryDelay = time.Second

func IsLocalhost(host string) bool {
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func NameFromImageRef(imageRef string) string {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return imageRef
	}
	path := reference.Path(named)
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func ContainerRandomID() (string, error) {
	return randomID(6)
}

func randomID(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

// cutLast splits s around the last occurrence of sep, like strings.Cut but
// from the right.
func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}
