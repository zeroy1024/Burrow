//go:build !windows

// Package localfs supplies the native file access rules shared by application
// storage and the local transfer client.
package localfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func OpenFile(name string, flags int, mode fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flags, mode)
}

func MkdirAll(name string, mode fs.FileMode) error { return os.MkdirAll(name, mode) }

func Protect(file *os.File) error { return file.Chmod(0600) }

func CheckPrivate(_ string, info fs.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return fs.ErrPermission
	}
	return nil
}

func ValidName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, 0) &&
		filepath.IsLocal(name) && filepath.Base(name) == name
}
