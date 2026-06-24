package docker

import (
	"encoding/json"
	"path/filepath"
	"strconv"
)

type SMTPSettings struct {
	Server   string `json:"server,omitempty"`
	Port     string `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
}

func (s SMTPSettings) BuildEnv() []string {
	if s.Server == "" {
		return nil
	}
	return []string{
		"SMTP_ADDRESS=" + s.Server,
		"SMTP_PORT=" + s.Port,
		"SMTP_USERNAME=" + s.Username,
		"SMTP_PASSWORD=" + s.Password,
		"MAILER_FROM_ADDRESS=" + s.From,
	}
}

type ContainerResources struct {
	CPUs     int `json:"cpus,omitempty"`
	MemoryMB int `json:"memoryMB,omitempty"`
}

type BackupSettings struct {
	Path       string `json:"path,omitempty"`
	AutoBackup bool   `json:"autoBackup,omitempty"`
}

type MountSetting struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type ApplicationSettings struct {
	Name       string             `json:"name"`
	Image      string             `json:"image"`
	Host       string             `json:"host"`
	DisableTLS bool               `json:"disableTLS"`
	EnvVars    map[string]string  `json:"env"`
	SMTP       SMTPSettings       `json:"smtp"`
	Resources  ContainerResources `json:"resources"`
	AutoUpdate bool               `json:"autoUpdate"`
	Backup     BackupSettings     `json:"backup"`
	Mounts     []MountSetting     `json:"mounts,omitempty"`
}

func UnmarshalApplicationSettings(s string) (ApplicationSettings, error) {
	var settings ApplicationSettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ApplicationSettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s ApplicationSettings) Validate() error {
	if s.Image == "" {
		return ErrImageRequired
	}
	if s.Backup.AutoBackup && s.Backup.Path == "" {
		return ErrAutoBackupWithoutPath
	}
	if err := ValidateMounts(s.Mounts); err != nil {
		return err
	}
	return nil
}

func ValidateMounts(mounts []MountSetting) error {
	reserved := make(map[string]bool, len(AppVolumeMountTargets))
	for _, t := range AppVolumeMountTargets {
		reserved[t] = true
	}
	seen := make(map[string]bool)
	for _, m := range mounts {
		if !filepath.IsAbs(m.Source) {
			return ErrMountSourceRelative
		}
		if !filepath.IsAbs(m.Target) {
			return ErrMountTargetRelative
		}
		if reserved[m.Target] {
			return ErrMountTargetReserved
		}
		if seen[m.Target] {
			return ErrMountDuplicateTarget
		}
		seen[m.Target] = true
	}
	return nil
}

func (s ApplicationSettings) TLSEnabled() bool {
	return s.Host != "" && !s.DisableTLS && !IsLocalhost(s.Host)
}

func (s ApplicationSettings) Equal(other ApplicationSettings) bool {
	if s.Name != other.Name || s.Image != other.Image || s.Host != other.Host || s.DisableTLS != other.DisableTLS {
		return false
	}
	if s.Resources != other.Resources {
		return false
	}
	if s.SMTP != other.SMTP {
		return false
	}
	if s.AutoUpdate != other.AutoUpdate {
		return false
	}
	if s.Backup != other.Backup {
		return false
	}
	if len(s.EnvVars) != len(other.EnvVars) {
		return false
	}
	for k, v := range s.EnvVars {
		if other.EnvVars[k] != v {
			return false
		}
	}
	if len(s.Mounts) != len(other.Mounts) {
		return false
	}
	for i, m := range s.Mounts {
		if m != other.Mounts[i] {
			return false
		}
	}
	return true
}

func (s ApplicationSettings) BuildEnv(vol ApplicationVolumeSettings) []string {
	env := []string{
		"SECRET_KEY_BASE=" + vol.SecretKeyBase,
		"VAPID_PUBLIC_KEY=" + vol.VAPIDPublicKey,
		"VAPID_PRIVATE_KEY=" + vol.VAPIDPrivateKey,
	}

	if !s.TLSEnabled() {
		env = append(env, "DISABLE_SSL=true")
	}

	if s.Resources.CPUs > 0 {
		env = append(env, "NUM_CPUS="+strconv.Itoa(s.Resources.CPUs))
	}

	env = append(env, s.SMTP.BuildEnv()...)

	for k, v := range s.EnvVars {
		env = append(env, k+"="+v)
	}

	return env
}
