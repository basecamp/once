package docker

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Keys struct {
	SecretKeyBase   string `json:"secretKeyBase,omitempty"`
	VAPIDPublicKey  string `json:"vapidPublicKey,omitempty"`
	VAPIDPrivateKey string `json:"vapidPrivateKey,omitempty"`
}

func (k Keys) Empty() bool {
	return k == Keys{}
}

func (k *Keys) SetVAPIDKey(privateKey string) error {
	key, err := parseVAPIDPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("invalid VAPID private key: %w", err)
	}

	k.VAPIDPublicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	k.VAPIDPrivateKey = base64.RawURLEncoding.EncodeToString(key.Bytes())

	return nil
}

func (k *Keys) Regenerate(secretKeyBase, vapid bool) error {
	if secretKeyBase {
		skb, err := generateSecretKeyBase()
		if err != nil {
			return err
		}
		k.SecretKeyBase = skb
	}

	if vapid {
		pub, priv, err := generateVAPIDKeyPair()
		if err != nil {
			return err
		}
		k.VAPIDPublicKey = pub
		k.VAPIDPrivateKey = priv
	}

	return nil
}

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
	Keys       Keys               `json:"keys"`
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

// Host stores one or more hostnames as a comma-separated list, keeping the
// serialized settings backward compatible with single-host installs.
func (s ApplicationSettings) Hosts() []string {
	if s.Host == "" {
		return nil
	}

	var hosts []string
	for _, h := range strings.Split(s.Host, ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func (s ApplicationSettings) PrimaryHost() string {
	if hosts := s.Hosts(); len(hosts) > 0 {
		return hosts[0]
	}
	return ""
}

func (s ApplicationSettings) Validate() error {
	if s.Image == "" {
		return ErrImageRequired
	}
	if s.Backup.AutoBackup && s.Backup.Path == "" {
		return ErrAutoBackupWithoutPath
	}
	return nil
}

func (s ApplicationSettings) TLSEnabled() bool {
	return s.Host != "" && !s.DisableTLS && !IsLocalhost(s.PrimaryHost())
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
	if s.Keys != other.Keys {
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
	return true
}

func (s ApplicationSettings) BuildEnv() []string {
	env := []string{
		"SECRET_KEY_BASE=" + s.Keys.SecretKeyBase,
		"VAPID_PUBLIC_KEY=" + s.Keys.VAPIDPublicKey,
		"VAPID_PRIVATE_KEY=" + s.Keys.VAPIDPrivateKey,
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

// Helpers

func generateSecretKeyBase() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateVAPIDKeyPair() (publicKey, privateKey string, err error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	privateKey = base64.RawURLEncoding.EncodeToString(key.Bytes())
	publicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())

	return publicKey, privateKey, nil
}

// parseVAPIDPrivateKey accepts any common base64 dialect (url-safe or
// standard, padded or not) of a raw P-256 private key.
func parseVAPIDPrivateKey(s string) (*ecdh.PrivateKey, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("+", "-", "/", "_").Replace(s)
	s = strings.TrimRight(s, "=")

	bytes, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	return ecdh.P256().NewPrivateKey(bytes)
}
