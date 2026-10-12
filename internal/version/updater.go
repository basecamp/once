package version

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
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
	maxManifestSize   = 64 << 10
)

type Updater struct {
	currentVersion string
	apiURL         string
	client         *http.Client
	githubToken    string
	keys           []*ecdsa.PublicKey
	executable     func() (string, error)
}

func NewUpdater() *Updater {
	u := newUpdater(Version, githubReleasesURL, &http.Client{Timeout: httpTimeout})
	u.githubToken = os.Getenv("GITHUB_TOKEN")
	// A malformed key leaves none trusted, so every update is refused;
	// TestEmbeddedReleaseKeys keeps one from being built in.
	u.keys, _ = parseReleaseKeys(releaseKeys)
	return u
}

func newUpdater(currentVersion, apiURL string, client *http.Client) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		apiURL:         apiURL,
		client:         client,
		executable:     os.Executable,
	}
}

// UpdateBinary replaces the running binary with the latest release, but only
// when that release is newer than this one and its release.txt is signed by a
// built-in release key and lists the downloaded binary's SHA-256. Anything
// else leaves the binary untouched and returns an error.
func (u *Updater) UpdateBinary() error {
	rel, err := u.fetchRelease()
	if err != nil {
		return err
	}

	newer, err := isNewerRelease(rel.TagName, u.currentVersion)
	if err != nil {
		return fmt.Errorf("not updating: %w", err)
	}
	if !newer {
		fmt.Printf("You already have the latest version (%s)\n", u.currentVersion)
		return nil
	}

	assetName := fmt.Sprintf("once-%s-%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := rel.assetURL(assetName)
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s", assetName)
	}

	expected, err := u.verifiedChecksum(rel, assetName)
	if err != nil {
		return fmt.Errorf("not updating to %s: %w", rel.TagName, err)
	}

	fmt.Printf("Updating once from %s to %s...\n", u.currentVersion, rel.TagName)

	execPath, err := u.executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	tmpFile := filepath.Join(filepath.Dir(execPath), updateTempFile)
	sum, err := u.downloadBinary(downloadURL, tmpFile)
	if err != nil {
		return err
	}
	if !checksumMatches(expected, sum) {
		os.Remove(tmpFile)
		return fmt.Errorf("not updating to %s: downloaded %s does not match the checksum in release.txt", rel.TagName, assetName)
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

func (r *release) assetURL(name string) string {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
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

func (u *Updater) verifiedChecksum(rel *release, assetName string) (string, error) {
	manifestURL := rel.assetURL(manifestAssetName)
	signatureURL := rel.assetURL(signatureAssetName)
	if manifestURL == "" || signatureURL == "" {
		return "", fmt.Errorf("the release has no %s and %s to verify it with", manifestAssetName, signatureAssetName)
	}

	manifest, err := u.fetchAsset(manifestURL)
	if err != nil {
		return "", err
	}
	signature, err := u.fetchAsset(signatureURL)
	if err != nil {
		return "", err
	}

	m, err := verifyManifest(manifest, signature, u.keys)
	if err != nil {
		return "", err
	}
	if m.Version != rel.TagName {
		return "", fmt.Errorf("release.txt is signed for %s, not %s", m.Version, rel.TagName)
	}

	expected, ok := m.Checksums[assetName]
	if !ok {
		return "", fmt.Errorf("release.txt lists no checksum for %s", assetName)
	}
	return expected, nil
}

func (u *Updater) fetchAsset(url string) ([]byte, error) {
	resp, err := u.getAsset(url)
	if err != nil {
		return nil, fmt.Errorf("downloading release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading release asset: unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("downloading release asset: %w", err)
	}
	if len(data) > maxManifestSize {
		return nil, fmt.Errorf("downloading release asset: larger than %d bytes", maxManifestSize)
	}
	return data, nil
}

func (u *Updater) getAsset(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if u.githubToken != "" {
		req.Header.Set("Authorization", "token "+u.githubToken)
	}
	return u.client.Do(req)
}

// downloadBinary writes the asset to dest and returns its SHA-256. On error,
// dest is removed.
func (u *Updater) downloadBinary(url, dest string) ([]byte, error) {
	resp, err := u.getAsset(url)
	if err != nil {
		return nil, fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading binary: unexpected status %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()

	hash := sha256.New()
	pr := &progressReader{r: resp.Body, total: resp.ContentLength}
	if _, err := io.Copy(io.MultiWriter(f, hash), pr); err != nil {
		fmt.Println()
		os.Remove(dest)
		return nil, fmt.Errorf("writing binary: %w", err)
	}
	fmt.Println()

	if err := f.Close(); err != nil {
		os.Remove(dest)
		return nil, fmt.Errorf("writing binary: %w", err)
	}

	return hash.Sum(nil), nil
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
