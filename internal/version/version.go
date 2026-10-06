package version

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

var Version = "dev"

var versionPattern = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?$`)

type releaseVersion struct {
	core       [3]int
	prerelease bool
}

// Helpers

// isNewerRelease reports whether candidate is a full release (vMAJOR.MINOR.PATCH,
// no suffix) that is newer than current. current may carry a suffix -- a local
// build reports `git describe`, such as v1.2.3-4-gabcdef0 -- and SemVer ranks
// that below v1.2.3. Anything that is not a version is an error, so a "dev"
// build never self-updates.
func isNewerRelease(candidate, current string) (bool, error) {
	c, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("latest release: %w", err)
	}
	if c.prerelease {
		return false, fmt.Errorf("latest release: %q is a prerelease", candidate)
	}

	r, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("running version: %w", err)
	}

	if order := slices.Compare(c.core[:], r.core[:]); order != 0 {
		return order > 0, nil
	}
	return r.prerelease, nil
}

func parseVersion(s string) (releaseVersion, error) {
	match := versionPattern.FindStringSubmatch(s)
	if match == nil {
		return releaseVersion{}, fmt.Errorf("%q is not a release version", s)
	}

	var v releaseVersion
	for i := range 3 {
		n, err := strconv.Atoi(match[i+1])
		if err != nil {
			return releaseVersion{}, fmt.Errorf("%q is not a release version", s)
		}
		v.core[i] = n
	}
	v.prerelease = match[4] != ""
	return v, nil
}
