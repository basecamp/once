package userstats

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
)

const NumBuckets = 168 // 7 days × 24 hours

const (
	stateFileDir      = "/home/kamal-proxy/.config/kamal-proxy"
	binaryFileName    = "once-userstats.bin"
	binaryFilePath    = stateFileDir + "/" + binaryFileName
	summaryFileName   = "once-userstats.json"
	summaryFilePath   = stateFileDir + "/" + summaryFileName
)

type Summary struct {
	UpdatedAt time.Time                  `json:"updated_at"`
	Services  map[string]ServiceSummary  `json:"services"`
}

type ServiceSummary struct {
	UniqueUsers24h uint64 `json:"unique_users_24h"`
	UniqueUsers7d  uint64 `json:"unique_users_7d"`
}

type BucketStore struct {
	LastTimestamp time.Time
	Services     map[string]*ServiceData
}

type ServiceData struct {
	Buckets     [NumBuckets][]byte // serialized HLL sketches
	BucketHours [NumBuckets]int64
}

type copyClient interface {
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, container.PathStat, error)
	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options container.CopyToContainerOptions) error
}

func Save(ctx context.Context, client copyClient, containerName string, store *BucketStore) error {
	data, err := encodeStore(store)
	if err != nil {
		return fmt.Errorf("encoding store: %w", err)
	}

	return writeToContainer(ctx, client, containerName, binaryFileName, data)
}

func Load(ctx context.Context, client copyClient, containerName string) (*BucketStore, error) {
	data, err := readFromContainer(ctx, client, containerName, binaryFilePath)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return &BucketStore{Services: make(map[string]*ServiceData)}, nil
		}
		return nil, fmt.Errorf("reading store: %w", err)
	}

	store, err := decodeStore(data)
	if err != nil {
		return nil, fmt.Errorf("decoding store: %w", err)
	}

	return store, nil
}

func SaveSummary(ctx context.Context, client copyClient, containerName string, store *BucketStore) error {
	summary := computeSummary(store)

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling summary: %w", err)
	}

	return writeToContainer(ctx, client, containerName, summaryFileName, data)
}

func LoadSummary(ctx context.Context, client copyClient, containerName string) (*Summary, error) {
	data, err := readFromContainer(ctx, client, containerName, summaryFilePath)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading summary: %w", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("decoding summary: %w", err)
	}

	return &summary, nil
}

// Helpers

func computeSummary(store *BucketStore) *Summary {
	now := time.Now()
	currentHour := now.Unix() / 3600
	cutoff24h := currentHour - 24
	cutoff7d := currentHour - NumBuckets

	summary := &Summary{
		UpdatedAt: now,
		Services:  make(map[string]ServiceSummary),
	}

	for name, svc := range store.Services {
		sketch24h := hyperloglog.New()
		sketch7d := hyperloglog.New()

		for i := range NumBuckets {
			if svc.BucketHours[i] == 0 || svc.Buckets[i] == nil {
				continue
			}

			if svc.BucketHours[i] >= cutoff7d {
				s := deserializeSketch(svc.Buckets[i])
				if s != nil {
					sketch7d.Merge(s)
				}
			}

			if svc.BucketHours[i] >= cutoff24h {
				s := deserializeSketch(svc.Buckets[i])
				if s != nil {
					sketch24h.Merge(s)
				}
			}
		}

		summary.Services[name] = ServiceSummary{
			UniqueUsers24h: sketch24h.Estimate(),
			UniqueUsers7d:  sketch7d.Estimate(),
		}
	}

	return summary
}

func deserializeSketch(data []byte) *hyperloglog.Sketch {
	var s hyperloglog.Sketch
	if err := s.UnmarshalBinary(data); err != nil {
		return nil
	}
	return &s
}

type gobStore struct {
	LastTimestamp time.Time
	Services     map[string]*gobServiceData
}

type gobServiceData struct {
	Buckets     [NumBuckets][]byte
	BucketHours [NumBuckets]int64
}

func encodeStore(store *BucketStore) ([]byte, error) {
	gs := gobStore{
		LastTimestamp: store.LastTimestamp,
		Services:     make(map[string]*gobServiceData),
	}

	for name, svc := range store.Services {
		gs.Services[name] = &gobServiceData{
			Buckets:     svc.Buckets,
			BucketHours: svc.BucketHours,
		}
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(gs); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeStore(data []byte) (*BucketStore, error) {
	var gs gobStore
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&gs); err != nil {
		return nil, err
	}

	store := &BucketStore{
		LastTimestamp: gs.LastTimestamp,
		Services:     make(map[string]*ServiceData),
	}

	for name, svc := range gs.Services {
		store.Services[name] = &ServiceData{
			Buckets:     svc.Buckets,
			BucketHours: svc.BucketHours,
		}
	}

	return store, nil
}

func writeToContainer(ctx context.Context, client copyClient, containerName, fileName string, data []byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	header := &tar.Header{
		Name: fileName,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing tar data: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar writer: %w", err)
	}

	if err := client.CopyToContainer(ctx, containerName, stateFileDir, &buf, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("copying to container: %w", err)
	}

	return nil
}

func readFromContainer(ctx context.Context, client copyClient, containerName, filePath string) ([]byte, error) {
	reader, _, err := client.CopyFromContainer(ctx, containerName, filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("reading tar: %w", err)
	}

	return io.ReadAll(tr)
}
