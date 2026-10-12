package version

// releaseKeys holds the PEM public keys trusted to sign release.txt. A release
// verifies when any one of them signed it, so a new key is added here one
// release before it starts signing, and the old one removed once it has stopped.
//
// It must match release-key.pub at the repository root byte for byte, and so
// must the copy in installer/templates/install.sh; tests check both.
const releaseKeys = `PLACEHOLDER - this is not a key, and nothing verifies against it.

Replace this whole file with the PEM public key (or keys) that sign
release.txt, and paste the same text into internal/version/release_keys.go
and installer/templates/install.sh. Until then every self-update and every
install from get.once.com refuses to proceed.
`
