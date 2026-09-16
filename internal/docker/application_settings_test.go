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

func TestKeysRegenerate(t *testing.T) {
	original := Keys{
		SecretKeyBase:   "original-secret",
		VAPIDPublicKey:  "original-pub",
		VAPIDPrivateKey: "original-priv",
	}

	t.Run("secret key base only", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(true, false))
		assert.NotEqual(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.Len(t, keys.SecretKeyBase, 64)
		assert.Equal(t, original.VAPIDPublicKey, keys.VAPIDPublicKey)
		assert.Equal(t, original.VAPIDPrivateKey, keys.VAPIDPrivateKey)
	})

	t.Run("vapid only", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(false, true))
		assert.Equal(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.NotEqual(t, original.VAPIDPublicKey, keys.VAPIDPublicKey)
		assert.NotEqual(t, original.VAPIDPrivateKey, keys.VAPIDPrivateKey)
	})

	t.Run("both", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(true, true))
		assert.NotEqual(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.NotEqual(t, original.VAPIDPublicKey, keys.VAPIDPublicKey)
		assert.NotEqual(t, original.VAPIDPrivateKey, keys.VAPIDPrivateKey)
	})

	t.Run("neither", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(false, false))
		assert.Equal(t, original, keys)
	})
}

func TestKeysSetVAPIDKey(t *testing.T) {
	original := Keys{
		SecretKeyBase:   "original-secret",
		VAPIDPublicKey:  "original-pub",
		VAPIDPrivateKey: "original-priv",
	}

	t.Run("derives public key", func(t *testing.T) {
		pub, priv, err := generateVAPIDKeyPair()
		require.NoError(t, err)

		keys := original
		require.NoError(t, keys.SetVAPIDKey(priv))
		assert.Equal(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.Equal(t, pub, keys.VAPIDPublicKey)
		assert.Equal(t, priv, keys.VAPIDPrivateKey)
	})

	t.Run("normalizes base64 dialects", func(t *testing.T) {
		pub, priv, err := generateVAPIDKeyPair()
		require.NoError(t, err)

		privBytes, err := base64.RawURLEncoding.DecodeString(priv)
		require.NoError(t, err)

		dialects := []string{
			base64.URLEncoding.EncodeToString(privBytes),
			base64.StdEncoding.EncodeToString(privBytes),
			base64.RawStdEncoding.EncodeToString(privBytes),
			" " + priv + "\n",
		}

		for _, dialect := range dialects {
			keys := original
			require.NoError(t, keys.SetVAPIDKey(dialect))
			assert.Equal(t, pub, keys.VAPIDPublicKey)
			assert.Equal(t, priv, keys.VAPIDPrivateKey)
		}
	})

	t.Run("invalid private key", func(t *testing.T) {
		keys := original
		require.Error(t, keys.SetVAPIDKey("not-a-key"))
		assert.Equal(t, original, keys)
	})

	t.Run("empty private key", func(t *testing.T) {
		keys := original
		require.Error(t, keys.SetVAPIDKey(""))
		assert.Equal(t, original, keys)
	})

	t.Run("wrong key length", func(t *testing.T) {
		keys := original
		short := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
		require.Error(t, keys.SetVAPIDKey(short))
		assert.Equal(t, original, keys)
	})
}

func TestContainerResourcesEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 512}}

	differentCPUs := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 2, MemoryMB: 512}}
	assert.False(t, base.Equal(differentCPUs))

	differentMemory := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 1024}}
	assert.False(t, base.Equal(differentMemory))

	zeroResources := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(zeroResources))
}

func TestContainerResourcesMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:      "app",
		Image:     "img:latest",
		Resources: ContainerResources{CPUs: 2, MemoryMB: 512},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, 2, restored.Resources.CPUs)
	assert.Equal(t, 512, restored.Resources.MemoryMB)
	assert.True(t, original.Equal(restored))
}

func TestAutoUpdateEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", AutoUpdate: false}
	different := ApplicationSettings{Name: "app", AutoUpdate: true}
	assert.False(t, base.Equal(different))
}

func TestBackupSettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBackup: true}}

	differentPath := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/other", AutoBackup: true}}
	assert.False(t, base.Equal(differentPath))

	differentAutoBackupup := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBackup: false}}
	assert.False(t, base.Equal(differentAutoBackupup))

	noBackup := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(noBackup))
}

func TestKeysEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Keys: Keys{SecretKeyBase: "secret"}}

	different := ApplicationSettings{Name: "app", Keys: Keys{SecretKeyBase: "rotated"}}
	assert.False(t, base.Equal(different))

	same := ApplicationSettings{Name: "app", Keys: Keys{SecretKeyBase: "secret"}}
	assert.True(t, base.Equal(same))
}

func TestEnsureKeys(t *testing.T) {
	newApp := func(keys Keys) *Application {
		return NewApplication(nil, ApplicationSettings{Name: "app", Keys: keys})
	}
	volWithKeys := func(keys Keys) *ApplicationVolume {
		return &ApplicationVolume{Settings: ApplicationLegacyVolumeSettings{Keys: keys}}
	}

	t.Run("keeps existing keys", func(t *testing.T) {
		keys := Keys{SecretKeyBase: "existing"}
		app := newApp(keys)
		require.NoError(t, app.ensureKeys(volWithKeys(Keys{SecretKeyBase: "legacy"})))
		assert.Equal(t, keys, app.Settings.Keys)
	})

	t.Run("falls back to legacy volume keys", func(t *testing.T) {
		legacy := Keys{SecretKeyBase: "legacy", VAPIDPublicKey: "pub", VAPIDPrivateKey: "priv"}
		app := newApp(Keys{})
		require.NoError(t, app.ensureKeys(volWithKeys(legacy)))
		assert.Equal(t, legacy, app.Settings.Keys)
	})

	t.Run("generates keys when none exist", func(t *testing.T) {
		app := newApp(Keys{})
		require.NoError(t, app.ensureKeys(volWithKeys(Keys{})))
		assert.NotEmpty(t, app.Settings.Keys.SecretKeyBase)
		assert.NotEmpty(t, app.Settings.Keys.VAPIDPublicKey)
		assert.NotEmpty(t, app.Settings.Keys.VAPIDPrivateKey)
	})
}

func TestEnvVarsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:  "app",
		Image: "img:latest",
		EnvVars: map[string]string{
			"FOO": "bar",
			"BAZ": "qux",
		},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, "bar", restored.EnvVars["FOO"])
	assert.Equal(t, "qux", restored.EnvVars["BAZ"])
	assert.True(t, original.Equal(restored))
}

func TestEnvVarsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", EnvVars: map[string]string{"A": "1"}}

	different := ApplicationSettings{Name: "app", EnvVars: map[string]string{"A": "2"}}
	assert.False(t, base.Equal(different))

	extra := ApplicationSettings{Name: "app", EnvVars: map[string]string{"A": "1", "B": "2"}}
	assert.False(t, base.Equal(extra))

	none := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(none))
}

func TestRegistrySettingsValidate(t *testing.T) {
	base := ApplicationSettings{Name: "app", Image: "img:latest"}

	t.Run("empty registry settings", func(t *testing.T) {
		assert.NoError(t, base.Validate())
	})

	t.Run("username and password pair", func(t *testing.T) {
		s := base
		s.Registry = RegistrySettings{Host: "docker.io", Username: "user", Password: "pass"}
		assert.NoError(t, s.Validate())
	})

	t.Run("password without username", func(t *testing.T) {
		s := base
		s.Registry = RegistrySettings{Host: "docker.io", Password: "pass"}
		assert.ErrorIs(t, s.Validate(), ErrRegistryPasswordWithoutUsername)
	})

	t.Run("username without password", func(t *testing.T) {
		s := base
		s.Registry = RegistrySettings{Host: "docker.io", Username: "user"}
		assert.ErrorIs(t, s.Validate(), ErrRegistryUsernameWithoutPassword)
	})
}

func TestRegistrySettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Registry: RegistrySettings{Host: "docker.io", Username: "user", Password: "pass"}}

	differentPassword := base
	differentPassword.Registry.Password = "rotated"
	assert.False(t, base.Equal(differentPassword))

	differentScope := base
	differentScope.Registry.Host = "ghcr.io"
	assert.False(t, base.Equal(differentScope))

	same := base
	assert.True(t, base.Equal(same))
}

func TestRegistrySettingsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:     "app",
		Image:    "img:latest",
		Registry: RegistrySettings{Host: "docker.io", Username: "user", Password: "pass"},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, original.Registry, restored.Registry)
	assert.True(t, original.Equal(restored))
}

func TestAutoUpdateAndBackupMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:       "app",
		Image:      "img:latest",
		AutoUpdate: true,
		Backup:     BackupSettings{Path: "/backups", AutoBackup: true},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.True(t, restored.AutoUpdate)
	assert.Equal(t, "/backups", restored.Backup.Path)
	assert.True(t, restored.Backup.AutoBackup)
	assert.True(t, original.Equal(restored))
}
