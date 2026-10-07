package fsutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// OpenFile behaves like os.OpenFile, except that:
//  1. Any missing parent directories in the path are created implicitly
//  2. Ownership of the file (and any created parent directories) are set to
//     match that of the nearest existing ancestor, when OpenFile creates them.
//
// Everything below the nearest existing ancestor is created through handles
// rather than paths, files are created exclusively, and ownership only changes
// on what OpenFile created, so a planted symlink or file cannot make OpenFile
// hand over an existing file, or make CreateFile write to one. Symlinked
// directories are still followed, so a planted one can change where a new file
// is created; it still takes the ownership of the nearest existing ancestor.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	dir := filepath.Dir(path)

	ancestor, err := findExistingAncestor(dir)
	if err != nil {
		return nil, fmt.Errorf("determining ownership for %s: %w", dir, err)
	}

	root, err := os.OpenRoot(asDir(ancestor))
	if err != nil {
		return nil, fmt.Errorf("determining ownership for %s: %w", dir, err)
	}
	defer root.Close()

	uid, gid, err := ownership(root)
	if err != nil {
		return nil, fmt.Errorf("determining ownership for %s: %w", dir, err)
	}

	rel, err := filepath.Rel(ancestor, dir)
	if err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", dir, err)
	}

	parent, err := makeDirs(root, rel, uid, gid)
	if err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", dir, err)
	}
	defer parent.Close()

	return openInDir(parent, filepath.Base(path), flag, perm, uid, gid)
}

// CreateFile creates a new file, failing if anything already exists at the
// path. It is equivalent to OpenFile with O_RDWR|O_CREATE|O_EXCL flags and
// 0600 permissions (owner read/write only).
func CreateFile(path string) (*os.File, error) {
	return OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
}

// afterMkdir lets tests replace a directory between its creation and opening.
var afterMkdir func(name string)

// Helpers

func findExistingAncestor(dir string) (string, error) {
	for path := dir; ; path = filepath.Dir(path) {
		_, err := os.Lstat(path)
		if err == nil {
			return path, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if path == "/" {
			return "", fmt.Errorf("no existing parent directory found for %s", dir)
		}
	}
}

// asDir appends "/." so that opening the path fails if it is not a directory,
// rather than blocking on a FIFO. filepath.Join would clean the "." away.
func asDir(path string) string {
	return path + string(filepath.Separator) + "."
}

func ownership(root *os.Root) (int, int, error) {
	info, err := root.Stat(".")
	if err != nil {
		return 0, 0, err
	}
	stat := info.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid), nil
}

// makeDirs creates each missing directory of rel inside root, through a
// handle on its parent, and returns a handle on the last of them.
func makeDirs(root *os.Root, rel string, uid, gid int) (*os.Root, error) {
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if rel == "." {
		return current, nil
	}

	for name := range strings.SplitSeq(rel, string(filepath.Separator)) {
		created := true
		if err := current.Mkdir(name, 0o755); errors.Is(err, fs.ErrExist) {
			created = false
		} else if err != nil {
			current.Close()
			return nil, err
		}
		if created && afterMkdir != nil {
			afterMkdir(name)
		}

		next, err := current.OpenRoot(asDir(name))
		current.Close()
		if err != nil {
			return nil, err
		}
		current = next

		if created {
			if err := chownNewDir(current, uid, gid); err != nil {
				current.Close()
				return nil, err
			}
		}
	}

	return current, nil
}

// chownNewDir only changes the ownership of a directory that still looks like
// the one just created, since it may have been swapped for another between
// being created and opened. A new directory can already belong to uid when
// root is squashed on NFS, leaving only its group to change.
func chownNewDir(dir *os.Root, uid, gid int) error {
	dirUID, dirGID, err := ownership(dir)
	if err != nil {
		return err
	}
	if dirUID == uid && dirGID == gid {
		return nil
	}

	empty, err := isEmpty(dir)
	if err != nil {
		return err
	}
	if (dirUID != os.Geteuid() && dirUID != uid) || !empty {
		return fmt.Errorf("%s was replaced after it was created", dir.Name())
	}
	return dir.Chown(".", uid, gid)
}

func isEmpty(dir *os.Root) (bool, error) {
	f, err := dir.Open(".")
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err
}

// openInDir creates the file exclusively when the flags allow creation, which
// never follows a symlink and never touches an existing file, and only changes
// the ownership of a file it created.
func openInDir(dir *os.Root, name string, flag int, perm os.FileMode, uid, gid int) (*os.File, error) {
	if flag&os.O_CREATE != 0 {
		file, err := dir.OpenFile(name, flag|os.O_EXCL, perm)
		if err == nil {
			_ = file.Chown(uid, gid)
			return file, nil
		}
		if flag&os.O_EXCL != 0 || !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}

	return dir.OpenFile(name, flag&^os.O_CREATE, perm)
}
