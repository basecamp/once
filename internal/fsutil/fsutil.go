package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// OpenFile behaves like os.OpenFile, except that:
//  1. Any missing parent directories in the path are created implicitly,
//     readable by whoever the file itself is readable by
//  2. Ownership of the file (and any created parent directories) are set to
//     match that of the nearest existing ancestor.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	dir := filepath.Dir(path)

	uid, gid, err := findOwnership(dir)
	if err != nil {
		return nil, fmt.Errorf("determining ownership for %s: %w", dir, err)
	}

	if err := os.MkdirAll(dir, dirPerm(perm)); err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", dir, err)
	}

	if err := chownNewDirs(dir, uid, gid); err != nil {
		return nil, fmt.Errorf("setting directory ownership for %s: %w", dir, err)
	}

	file, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}

	_ = os.Chown(path, uid, gid)

	return file, nil
}

// CreateFile creates a new file, truncating it if it already exists. It is
// equivalent to OpenFile with O_RDWR|O_CREATE|O_TRUNC flags and 0600
// permissions (owner read/write only).
func CreateFile(path string) (*os.File, error) {
	return OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
}

// Helpers

// dirPerm is the mode for a directory created to hold a file of mode perm:
// the same bits, plus search wherever there is read or write. A 0600 file gets
// a 0700 directory, so an owner-only file is not listed to everyone else, and
// a write-only 0200 file still gets a directory its owner can traverse.
func dirPerm(perm os.FileMode) os.FileMode {
	return perm | (perm&0o444)>>2 | (perm&0o222)>>1
}

func findOwnership(dir string) (int, int, error) {
	for path := dir; ; path = filepath.Dir(path) {
		info, err := os.Stat(path)
		if err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			return int(stat.Uid), int(stat.Gid), nil
		}
		if !os.IsNotExist(err) {
			return 0, 0, err
		}
		if path == "/" {
			return 0, 0, fmt.Errorf("no existing parent directory found for %s", dir)
		}
	}
}

func chownNewDirs(dir string, uid, gid int) error {
	var dirs []string
	for path := dir; ; path = filepath.Dir(path) {
		info, err := os.Stat(path)
		if err != nil {
			break
		}
		stat := info.Sys().(*syscall.Stat_t)
		if int(stat.Uid) == uid && int(stat.Gid) == gid {
			break
		}
		dirs = append(dirs, path)
		if path == "/" {
			break
		}
	}

	for _, d := range dirs {
		if err := os.Chown(d, uid, gid); err != nil {
			return err
		}
	}
	return nil
}
