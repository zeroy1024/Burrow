//go:build darwin

package fileclient

import "golang.org/x/sys/unix"

func renameLocalNoReplace(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_EXCL)
}
