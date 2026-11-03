package localfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type openOptions struct {
	access, disposition, flags uint32
	security                   *windows.SECURITY_DESCRIPTOR
}

// openRelative resolves one component against an already open directory. The
// reparse point itself is opened, so callers can reject it before touching data.
func openRelative(parent *os.File, name string, options openOptions) (windows.Handle, bool, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, false, err
	}
	raw, err := parent.SyscallConn()
	if err != nil {
		return windows.InvalidHandle, false, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE,
		SecurityDescriptor: options.security,
	}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	var openErr error
	err = raw.Control(func(fd uintptr) {
		attributes.RootDirectory = windows.Handle(fd)
		openErr = windows.NtCreateFile(
			&handle,
			options.access|windows.SYNCHRONIZE,
			&attributes,
			&status,
			nil,
			windows.FILE_ATTRIBUTE_NORMAL,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			options.disposition,
			options.flags|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
			0,
			0,
		)
	})
	if err != nil {
		return windows.InvalidHandle, false, err
	}
	if openErr != nil {
		var code windows.NTStatus
		if errors.As(openErr, &code) {
			openErr = code.Errno()
		}
		return windows.InvalidHandle, false, openErr
	}
	return handle, status.Information == 2, nil // FILE_CREATED
}

func openAt(parent *os.Root, name string, flags int, mode fs.FileMode) (*os.File, bool, error) {
	if !ValidName(name) {
		return nil, false, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	directory, err := parent.Open(".")
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()
	options := openOptions{disposition: windows.FILE_OPEN, flags: windows.FILE_NON_DIRECTORY_FILE}
	switch flags & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDONLY:
		options.access = windows.FILE_GENERIC_READ
	case os.O_WRONLY:
		options.access = windows.FILE_GENERIC_WRITE
	case os.O_RDWR:
		options.access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE
	default:
		return nil, false, fs.ErrInvalid
	}
	// Every returned handle is inspected with File.Stat, including write-only
	// handles. FILE_GENERIC_WRITE does not include this metadata query right.
	options.access |= windows.FILE_READ_ATTRIBUTES
	if flags&os.O_APPEND != 0 {
		options.access = options.access&^windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA
	}
	if flags&os.O_CREATE != 0 {
		options.disposition = windows.FILE_OPEN_IF
		if flags&os.O_EXCL != 0 {
			options.disposition = windows.FILE_CREATE
		}
		if mode.Perm()&0077 == 0 {
			options.security, err = privateDescriptor()
			if err != nil {
				return nil, false, err
			}
			options.access |= windows.DELETE
		}
	}
	handle, created, err := openRelative(directory, name, options)
	if err != nil {
		return nil, false, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(handle), filepath.Join(parent.Name(), name))
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fs.ErrInvalid
	}
	if err == nil && created && options.security != nil {
		err = Protect(file)
	}
	if err != nil {
		if created {
			// Discard only the file created by this call, using its own handle.
			deleted := byte(1)
			err = errors.Join(err, windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo, &deleted, 1))
		}
		return nil, created, errors.Join(err, file.Close())
	}
	return file, created, nil
}

func OpenAt(parent *os.Root, name string, flags int, mode fs.FileMode) (*os.File, error) {
	file, _, err := openAt(parent, name, flags, mode)
	return file, err
}

func OpenFile(name string, flags int, mode fs.FileMode) (*os.File, error) {
	parent, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	file, _, err := openAt(parent, filepath.Base(name), flags&^os.O_TRUNC, mode)
	if err != nil {
		return nil, err
	}
	if flags&os.O_TRUNC != 0 {
		err = file.Truncate(0)
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func MkdirAll(name string, mode fs.FileMode) error {
	if err := os.MkdirAll(name, mode); err != nil {
		return err
	}
	if mode.Perm()&0077 != 0 {
		return nil
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(Protect(directory), directory.Close())
}

func ValidName(name string) bool {
	// Win32 path normalization otherwise aliases names ending in a dot/space.
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, 0) &&
		filepath.IsLocal(name) && filepath.Base(name) == name &&
		!strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}
