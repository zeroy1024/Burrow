package fileclient

import (
	"errors"
	"io/fs"
	"os"

	"sshdesk/internal/localfs"
)

func setLocalMetadata(entry *Entry, _ fs.FileInfo) {
	// Windows access is governed by ACLs, not the mode bits synthesized by Stat.
	entry.ModeKnown, entry.Permissions = false, "ACL"
}

func chmodLocalFile(*os.File, fs.FileMode) error { return errors.ErrUnsupported }

func secureLocalWrite(file *os.File) error { return localfs.Protect(file) }
