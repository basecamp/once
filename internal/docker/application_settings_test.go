package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildEnvWithSMTP(t *testing.T) {
	settings := ApplicationSettings{
		SMTP: SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
			From:     "noreply@example.com",
		},
	}

	env := settings.BuildEnv(ApplicationVolumeSettings{SecretKeyBase: "test-secret-key"})

	assert.Contains(t, env, "SMTP_ADDRESS=smtp.example.com")
	assert.Contains(t, env, "SMTP_PORT=587")
	assert.Contains(t, env, "SMTP_USERNAME=user@example.com")
	assert.Contains(t, env, "SMTP_PASSWORD=secret")
	assert.Contains(t, env, "MAILER_FROM_ADDRESS=noreply@example.com")
}

func TestBuildEnvWithCPULimit(t *testing.T) {
	settings := ApplicationSettings{Resources: ContainerResources{CPUs: 4}}

	env := settings.BuildEnv(ApplicationVolumeSettings{SecretKeyBase: "test-secret-key"})

	assert.Contains(t, env, "NUM_CPUS=4")
}

func TestBuildEnvWithoutCPULimit(t *testing.T) {
	settings := ApplicationSettings{}

	env := settings.BuildEnv(ApplicationVolumeSettings{SecretKeyBase: "test-secret-key"})

	assert.NotContains(t, env, "NUM_CPUS=0")
}

func TestBuildEnvWithoutSMTP(t *testing.T) {
	settings := ApplicationSettings{}

	env := settings.BuildEnv(ApplicationVolumeSettings{SecretKeyBase: "test-secret-key"})

	for _, e := range env {
		assert.NotContains(t, e, "SMTP_")
	}
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

func TestBuildEnvWithVAPIDKeys(t *testing.T) {
	settings := ApplicationSettings{}

	vol := ApplicationVolumeSettings{
		SecretKeyBase:   "test-secret-key",
		VAPIDPublicKey:  "test-vapid-public",
		VAPIDPrivateKey: "test-vapid-private",
	}
	env := settings.BuildEnv(vol)

	assert.Contains(t, env, "VAPID_PUBLIC_KEY=test-vapid-public")
	assert.Contains(t, env, "VAPID_PRIVATE_KEY=test-vapid-private")
}

func TestBuildEnvWithEnvVars(t *testing.T) {
	settings := ApplicationSettings{
		EnvVars: map[string]string{
			"DB_HOST": "postgres.local",
			"DB_NAME": "mydb",
		},
	}

	env := settings.BuildEnv(ApplicationVolumeSettings{SecretKeyBase: "test-secret-key"})

	assert.Contains(t, env, "DB_HOST=postgres.local")
	assert.Contains(t, env, "DB_NAME=mydb")
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

func TestMountsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:  "app",
		Image: "img:latest",
		Mounts: []MountSetting{
			{Source: "/host/data", Target: "/container/data"},
			{Source: "/host/config", Target: "/container/config"},
		},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	require.Len(t, restored.Mounts, 2)
	assert.Equal(t, "/host/data", restored.Mounts[0].Source)
	assert.Equal(t, "/container/data", restored.Mounts[0].Target)
	assert.Equal(t, "/host/config", restored.Mounts[1].Source)
	assert.Equal(t, "/container/config", restored.Mounts[1].Target)
	assert.True(t, original.Equal(restored))
}

func TestMountsOmittedWhenEmpty(t *testing.T) {
	original := ApplicationSettings{Name: "app", Image: "img:latest"}
	marshalled := original.Marshal()
	assert.NotContains(t, marshalled, "mounts")
}

func TestMountsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{
		Name:   "app",
		Mounts: []MountSetting{{Source: "/a", Target: "/b"}},
	}

	different := ApplicationSettings{
		Name:   "app",
		Mounts: []MountSetting{{Source: "/a", Target: "/c"}},
	}
	assert.False(t, base.Equal(different))

	extra := ApplicationSettings{
		Name: "app",
		Mounts: []MountSetting{
			{Source: "/a", Target: "/b"},
			{Source: "/x", Target: "/y"},
		},
	}
	assert.False(t, base.Equal(extra))

	none := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(none))
}

func TestMountsValidation(t *testing.T) {
	relativeSource := ApplicationSettings{
		Image:  "img:latest",
		Mounts: []MountSetting{{Source: "relative/path", Target: "/container"}},
	}
	assert.ErrorIs(t, relativeSource.Validate(), ErrMountSourceRelative)

	relativeTarget := ApplicationSettings{
		Image:  "img:latest",
		Mounts: []MountSetting{{Source: "/host", Target: "relative/path"}},
	}
	assert.ErrorIs(t, relativeTarget.Validate(), ErrMountTargetRelative)

	duplicateTarget := ApplicationSettings{
		Image: "img:latest",
		Mounts: []MountSetting{
			{Source: "/a", Target: "/same"},
			{Source: "/b", Target: "/same"},
		},
	}
	assert.ErrorIs(t, duplicateTarget.Validate(), ErrMountDuplicateTarget)

	reservedTarget := ApplicationSettings{
		Image:  "img:latest",
		Mounts: []MountSetting{{Source: "/host/data", Target: "/storage"}},
	}
	assert.ErrorIs(t, reservedTarget.Validate(), ErrMountTargetReserved)

	reservedTarget2 := ApplicationSettings{
		Image:  "img:latest",
		Mounts: []MountSetting{{Source: "/host/data", Target: "/rails/storage"}},
	}
	assert.ErrorIs(t, reservedTarget2.Validate(), ErrMountTargetReserved)

	valid := ApplicationSettings{
		Image: "img:latest",
		Mounts: []MountSetting{
			{Source: "/host/a", Target: "/container/a"},
			{Source: "/host/b", Target: "/container/b"},
		},
	}
	assert.NoError(t, valid.Validate())
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
