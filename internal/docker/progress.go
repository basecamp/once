package docker

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/docker/docker/pkg/jsonmessage"
)

const (
	statusDownloading      = "Downloading"
	statusDownloadComplete = "Download complete"
	statusPullComplete     = "Pull complete"
	statusAlreadyExists    = "Already exists"
)

type DeployStage int

const (
	DeployStageDownloading DeployStage = iota
	DeployStageStarting
	DeployStageFinished
)

type DeployProgress struct {
	Stage      DeployStage
	Percentage int
}

type DeployProgressCallback func(DeployProgress)

type pullProgressTracker struct {
	layers         map[string]*layerProgress
	callback       DeployProgressCallback
	lastPercentage int
}

type layerProgress struct {
	total   int64
	current int64
}

func newPullProgressTracker(callback DeployProgressCallback) *pullProgressTracker {
	return &pullProgressTracker{
		layers:   make(map[string]*layerProgress),
		callback: callback,
	}
}

func (t *pullProgressTracker) Track(reader io.Reader) error {
	decoder := json.NewDecoder(reader)

	for {
		var msg jsonmessage.JSONMessage
		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		t.processMessage(msg)
	}

	t.report(100)
	return nil
}

// Private

func (t *pullProgressTracker) processMessage(msg jsonmessage.JSONMessage) {
	if msg.ID == "" {
		return
	}

	layer, exists := t.layers[msg.ID]
	if !exists {
		layer = &layerProgress{}
		t.layers[msg.ID] = layer
	}

	switch msg.Status {
	case statusDownloading:
		if msg.Progress != nil {
			if msg.Progress.Total > 0 {
				layer.total = msg.Progress.Total
			}
			layer.current = msg.Progress.Current
		}
	case statusDownloadComplete, statusPullComplete, statusAlreadyExists:
		if layer.total > 0 {
			layer.current = layer.total
		}
	}

	t.reportProgress()
}

func (t *pullProgressTracker) reportProgress() {
	var totalBytes, downloadedBytes int64

	for _, layer := range t.layers {
		totalBytes += layer.total
		downloadedBytes += layer.current
	}

	var percentage int
	if totalBytes > 0 {
		percentage = min(int(downloadedBytes*100/totalBytes), 100)
	}

	t.lastPercentage = max(percentage, t.lastPercentage)
	t.report(t.lastPercentage)
}

func (t *pullProgressTracker) report(percentage int) {
	t.callback(DeployProgress{
		Stage:      DeployStageDownloading,
		Percentage: percentage,
	})
}
