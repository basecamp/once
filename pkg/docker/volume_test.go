package docker

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVAPIDKeyPair(t *testing.T) {
	pub, priv, err := generateVAPIDKeyPair()
	require.NoError(t, err)
	assert.NotEmpty(t, pub)
	assert.NotEmpty(t, priv)

	pubBytes, err := base64.RawURLEncoding.DecodeString(pub)
	require.NoError(t, err)
	assert.Len(t, pubBytes, 65)

	privBytes, err := base64.RawURLEncoding.DecodeString(priv)
	require.NoError(t, err)
	assert.Len(t, privBytes, 32)
}

func TestGenerateVAPIDKeyPairUniqueness(t *testing.T) {
	pub1, priv1, err := generateVAPIDKeyPair()
	require.NoError(t, err)

	pub2, priv2, err := generateVAPIDKeyPair()
	require.NoError(t, err)

	assert.NotEqual(t, pub1, pub2)
	assert.NotEqual(t, priv1, priv2)
}

func TestVolumeSettingsMarshalRoundTrip(t *testing.T) {
	original := ApplicationVolumeSettings{
		SecretKeyBase:   "secret",
		VAPIDPublicKey:  "pub123",
		VAPIDPrivateKey: "priv456",
	}

	restored, err := UnmarshalApplicationVolumeSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, original, restored)
}
