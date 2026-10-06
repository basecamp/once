package version

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A release publishes release.txt, which names the version, the commit it was
// built from, and the SHA-256 of every binary:
//
//	once v1.2.3
//	commit <40 hex>
//	<64 hex>  once-linux-amd64
//	...
//
// release.txt.sig is an ECDSA P-256 signature over it (SHA-256, ASN.1 DER), as
// made by `openssl dgst -sha256 -sign`.
const (
	manifestAssetName  = "release.txt"
	signatureAssetName = "release.txt.sig"
)

var (
	ErrNoReleaseKeys    = errors.New("no release signing keys are built in, so no release can be verified")
	ErrBadSignature     = errors.New("release.txt is not signed by a trusted release key")
	ErrMalformedRelease = errors.New("release.txt is malformed")
)

var (
	commitPattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	checksumPattern = regexp.MustCompile(`^([0-9a-f]{64})  (\S+)$`)
)

type releaseManifest struct {
	Version   string
	Commit    string
	Checksums map[string]string
}

// Helpers

func parseReleaseKeys(data string) ([]*ecdsa.PublicKey, error) {
	var keys []*ecdsa.PublicKey
	rest := []byte(data)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return keys, nil
		}
		if block.Type != "PUBLIC KEY" {
			return nil, fmt.Errorf("release key: unexpected PEM block %q", block.Type)
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("release key: %w", err)
		}
		key, ok := pub.(*ecdsa.PublicKey)
		if !ok || key.Curve != elliptic.P256() {
			return nil, errors.New("release key: not an ECDSA P-256 public key")
		}
		keys = append(keys, key)
	}
}

func verifyManifest(data, signature []byte, keys []*ecdsa.PublicKey) (*releaseManifest, error) {
	if len(keys) == 0 {
		return nil, ErrNoReleaseKeys
	}

	digest := sha256.Sum256(data)
	signed := slices.ContainsFunc(keys, func(key *ecdsa.PublicKey) bool {
		return ecdsa.VerifyASN1(key, digest[:], signature)
	})
	if !signed {
		return nil, ErrBadSignature
	}

	return parseManifest(data)
}

func parseManifest(data []byte) (*releaseManifest, error) {
	m := &releaseManifest{Checksums: map[string]string{}}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for n := 0; scanner.Scan(); n++ {
		line := scanner.Text()
		switch n {
		case 0:
			version, ok := strings.CutPrefix(line, "once ")
			if !ok || version == "" {
				return nil, fmt.Errorf("%w: first line is not the version", ErrMalformedRelease)
			}
			m.Version = version
		case 1:
			commit, ok := strings.CutPrefix(line, "commit ")
			if !ok || !commitPattern.MatchString(commit) {
				return nil, fmt.Errorf("%w: second line is not the commit", ErrMalformedRelease)
			}
			m.Commit = commit
		default:
			match := checksumPattern.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("%w: line %d is not a checksum", ErrMalformedRelease, n+1)
			}
			if _, dup := m.Checksums[match[2]]; dup {
				return nil, fmt.Errorf("%w: %s is listed twice", ErrMalformedRelease, match[2])
			}
			m.Checksums[match[2]] = match[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedRelease, err)
	}
	if m.Commit == "" || len(m.Checksums) == 0 {
		return nil, fmt.Errorf("%w: incomplete", ErrMalformedRelease)
	}

	return m, nil
}

func checksumMatches(expected string, sum []byte) bool {
	return expected == hex.EncodeToString(sum)
}
