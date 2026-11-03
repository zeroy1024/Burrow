package localfs

import (
	"errors"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type renameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func RenameAt(fromRoot *os.Root, from string, toRoot *os.Root, to string, noReplace bool) error {
	if !ValidName(from) || !ValidName(to) {
		return fs.ErrInvalid
	}
	source, err := fromRoot.Open(".")
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := toRoot.Open(".")
	if err != nil {
		return err
	}
	defer target.Close()
	handle, _, err := openRelative(source, from, openOptions{
		access: windows.DELETE | windows.FILE_READ_ATTRIBUTES, disposition: windows.FILE_OPEN,
	})
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	defer windows.CloseHandle(handle)
	name, err := windows.UTF16FromString(to)
	if err != nil {
		return err
	}
	var layout renameInformation
	length := int(unsafe.Offsetof(layout.FileName)) + len(name)*2
	// uintptr storage keeps the Win32 structure aligned on both architectures.
	storage := make([]uintptr, (length+int(unsafe.Sizeof(uintptr(0)))-1)/int(unsafe.Sizeof(uintptr(0))))
	info := (*renameInformation)(unsafe.Pointer(&storage[0]))
	if !noReplace {
		info.ReplaceIfExists = 1
	}
	info.FileNameLength = uint32((len(name) - 1) * 2)
	copy(unsafe.Slice(&info.FileName[0], len(name)), name)
	raw, err := target.SyscallConn()
	if err != nil {
		return err
	}
	var renameErr error
	err = raw.Control(func(fd uintptr) {
		info.RootDirectory = windows.Handle(fd)
		renameErr = windows.SetFileInformationByHandle(
			handle,
			windows.FileRenameInfo,
			(*byte)(unsafe.Pointer(info)),
			uint32(length),
		)
	})
	if err != nil || renameErr != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.Join(err, renameErr)}
	}
	return nil
}
