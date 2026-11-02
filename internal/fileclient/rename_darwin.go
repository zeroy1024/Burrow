//go:build darwin

package fileclient

import "golang.org/x/sys/unix"

func renameLocalNoReplace(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_EXCL)
}

func renameLocalNoReplaceAt(fromFD int, from string, toFD int, to string) error {
	return unix.RenameatxNp(fromFD, from, toFD, to, unix.RENAME_EXCL)
}
