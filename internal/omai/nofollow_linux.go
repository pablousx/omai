package omai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Resolve each directory through an already-open parent. O_NOFOLLOW on every
// component closes the check/open race that a standalone Lstat cannot prevent.
func openParent(path string, create bool) (*os.File, string, error) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, "", e
	}
	fd, e := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, "", e
	}
	parts := strings.Split(strings.TrimPrefix(abs, "/"), "/")
	for _, part := range parts[:len(parts)-1] {
		next, err := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err == syscall.ENOENT && create {
			if err = syscall.Mkdirat(fd, part, 0700); err == nil || err == syscall.EEXIST {
				err = syscall.Fsync(fd)
				if err == nil {
					next, err = syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
				}
			}
		}
		_ = syscall.Close(fd)
		if err != nil {
			return nil, "", &os.PathError{Op: "open parent without symlinks", Path: path, Err: err}
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), filepath.Dir(abs)), parts[len(parts)-1], nil
}
func openRegularNoFollow(path string) (*os.File, error) {
	parent, name, e := openParent(path, false)
	if e != nil {
		return nil, e
	}
	defer parent.Close()
	fd, e := syscall.Openat(int(parent.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, &os.PathError{Op: "open without symlinks", Path: path, Err: e}
	}
	return os.NewFile(uintptr(fd), path), nil
}
func tempAt(parent *os.File, name string) (*os.File, string, error) {
	if name == "" {
		var nonce [12]byte
		if _, e := rand.Read(nonce[:]); e != nil {
			return nil, "", e
		}
		name = ".omai-write-" + hex.EncodeToString(nonce[:])
	}
	if filepath.Base(name) != name {
		return nil, "", fmt.Errorf("invalid temporary name")
	}
	fd, e := syscall.Openat(int(parent.Fd()), name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, "", e
	}
	return os.NewFile(uintptr(fd), name), name, nil
}
