package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	githubReleasesURL = "https://api.github.com/repos/basecamp/once/releases/latest"
	httpTimeout       = 30 * time.Second
	updateTempFile    = ".once-update-tmp"
)

type Updater struct {
	currentVersion string
	apiURL         string
	client         *http.Client
	githubToken    string
}

func NewUpdater() *Updater {
	u := newUpdater(Version, githubReleasesURL, &http.Client{Timeout: httpTimeout})
	u.githubToken = os.Getenv("GITHUB_TOKEN")
	return u
}

func newUpdater(currentVersion, apiURL string, client *http.Client) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		apiURL:         apiURL,
		client:         client,
	}
}

func (u *Updater) UpdateBinary() error {
	rel, err := u.fetchRelease()
	if err != nil {
		return err
	}

	if rel.TagName == u.currentVersion {
		fmt.Printf("You already have the latest version (%s)\n", u.currentVersion)
		return nil
	}

	assetName := fmt.Sprintf("once-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.URL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s", assetName)
	}

	fmt.Printf("Updating once from %s to %s...\n", u.currentVersion, rel.TagName)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	tmpFile := filepath.Join(filepath.Dir(execPath), updateTempFile)
	if err := u.downloadBinary(downloadURL, tmpFile); err != nil {
		return err
	}

	if err := u.replaceBinary(execPath, tmpFile); err != nil {
		return err
	}

	fmt.Println("Update complete.")
	return nil
}

// Private

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (u *Updater) get(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if u.githubToken != "" {
		req.Header.Set("Authorization", "token "+u.githubToken)
	}
	return u.client.Do(req)
}

func (u *Updater) fetchRelease() (*release, error) {
	resp, err := u.get(u.apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching release: unexpected status %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	return &rel, nil
}

func (u *Updater) downloadBinary(url, dest string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	if u.githubToken != "" {
		req.Header.Set("Authorization", "token "+u.githubToken)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading binary: unexpected status %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()

	pr := &progressReader{r: resp.Body, total: resp.ContentLength}
	if _, err := io.Copy(f, pr); err != nil {
		fmt.Println()
		os.Remove(dest)
		return fmt.Errorf("writing binary: %w", err)
	}
	fmt.Println()

	return nil
}

type progressReader struct {
	r     io.Reader
	total int64 // -1 if unknown
	read  int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.total > 0 {
		fmt.Printf("\r  %.1f / %.1f MB (%d%%)", float64(p.read)/1e6, float64(p.total)/1e6, p.read*100/p.total)
	} else {
		fmt.Printf("\r  %.1f MB", float64(p.read)/1e6)
	}
	return n, err
}

func (u *Updater) replaceBinary(execPath, newPath string) error {
	oldPath := execPath + ".old"

	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := os.Rename(newPath, execPath); err != nil {
		// Attempt to restore from backup
		os.Rename(oldPath, execPath)
		return fmt.Errorf("replacing binary: %w", err)
	}

	os.Remove(oldPath)
	return nil
}
