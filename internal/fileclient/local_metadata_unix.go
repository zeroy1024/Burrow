//go:build !windows

package fileclient

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func setLocalMetadata(entry *Entry, info fs.FileInfo) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.Owner = fmt.Sprintf("%d:%d", stat.Uid, stat.Gid)
	}
}

func chmodLocalFile(file *os.File, mode fs.FileMode) error { return file.Chmod(mode) }

// Unix creation already applies mode 0600; existing-file modes remain intact.
func secureLocalWrite(*os.File) error { return nil }
