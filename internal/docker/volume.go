package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/volume"
)

var ErrVolumeNotFound = errors.New("volume not found")

type ApplicationVolumeSettings struct {
	SecretKeyBase string `json:"skb"`
}

func UnmarshalApplicationVolumeSettings(s string) (ApplicationVolumeSettings, error) {
	var settings ApplicationVolumeSettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ApplicationVolumeSettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

type ApplicationVolume struct {
	namespace *Namespace
	name      string
	Settings  ApplicationVolumeSettings
}

func (v *ApplicationVolume) SecretKeyBase() string {
	return v.Settings.SecretKeyBase
}

func (v *ApplicationVolume) Name() string {
	return v.name
}

func (v *ApplicationVolume) Destroy(ctx context.Context) error {
	if err := v.namespace.client.VolumeRemove(ctx, v.name, true); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("removing volume: %w", err)
		}
	}
	return nil
}

func FindVolume(ctx context.Context, ns *Namespace, name string) (*ApplicationVolume, error) {
	volumeName := fmt.Sprintf("%s-app-%s", ns.name, name)

	vol, err := ns.client.VolumeInspect(ctx, volumeName)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, ErrVolumeNotFound
		}
		return nil, fmt.Errorf("inspecting volume: %w", err)
	}

	label := vol.Labels[labelKey]
	if label == "" {
		return nil, fmt.Errorf("volume %s exists but has no once label", volumeName)
	}

	settings, err := UnmarshalApplicationVolumeSettings(label)
	if err != nil {
		return nil, fmt.Errorf("parsing volume settings: %w", err)
	}

	return &ApplicationVolume{
		namespace: ns,
		name:      volumeName,
		Settings:  settings,
	}, nil
}

func CreateVolume(ctx context.Context, ns *Namespace, name string, settings ApplicationVolumeSettings) (*ApplicationVolume, error) {
	volumeName := fmt.Sprintf("%s-app-%s", ns.name, name)

	_, err := ns.client.VolumeCreate(ctx, volume.CreateOptions{
		Name: volumeName,
		Labels: map[string]string{
			labelKey: settings.Marshal(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}

	return &ApplicationVolume{
		namespace: ns,
		name:      volumeName,
		Settings:  settings,
	}, nil
}

// Helpers

func generateSecretKeyBase() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
