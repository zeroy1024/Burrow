//go:build darwin

package fileclient

import "golang.org/x/sys/unix"

func renameLocalNoReplaceAt(fromFD int, from string, toFD int, to string) error {
	return unix.RenameatxNp(fromFD, from, toFD, to, unix.RENAME_EXCL)
}
