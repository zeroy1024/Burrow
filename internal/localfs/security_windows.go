package localfs

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func privateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	// Protect the DACL from parent inheritance. Directory children inherit the
	// same account and SYSTEM rules; Unix permission bits cannot express this.
	return windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
}

func aclText(acl *windows.ACL) (string, error) {
	descriptor, err := windows.NewSecurityDescriptor()
	if err != nil {
		return "", err
	}
	if err := descriptor.SetDACL(acl, true, false); err != nil {
		return "", err
	}
	text := descriptor.String()
	if text == "" {
		return "", errors.New("cannot describe Windows file ACL")
	}
	return text, nil
}

func applyACL(file *os.File, descriptor *windows.SECURITY_DESCRIPTOR, protected bool) error {
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	expected, err := aclText(acl)
	if err != nil {
		return err
	}
	// Reopen the exact object through its handle, never through a path that
	// could have changed after the caller's identity check.
	handle, _, err := openRelative(file, "", openOptions{
		access: windows.READ_CONTROL | windows.WRITE_DAC, disposition: windows.FILE_OPEN,
	})
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if protected {
		flags = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, flags, nil, nil, acl, nil); err != nil {
		return err
	}
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	actualACL, _, err := actual.DACL()
	if err != nil {
		return err
	}
	text, err := aclText(actualACL)
	if err != nil {
		return err
	}
	if text != expected {
		return errors.New("Windows file ACL was not preserved")
	}
	return nil
}

func Protect(file *os.File) error {
	descriptor, err := privateDescriptor()
	if err != nil {
		return err
	}
	return applyACL(file, descriptor, true)
}

func fileSecurity(file *os.File, information windows.SECURITY_INFORMATION) (*windows.SECURITY_DESCRIPTOR, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var descriptor *windows.SECURITY_DESCRIPTOR
	var readErr error
	err = raw.Control(func(fd uintptr) {
		descriptor, readErr = windows.GetSecurityInfo(windows.Handle(fd), windows.SE_FILE_OBJECT, information)
	})
	return descriptor, errors.Join(err, readErr)
}

func CheckPrivate(name string, info fs.FileInfo) error {
	file, err := OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return fs.ErrInvalid
	}
	descriptor, err := fileSecurity(file, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	ownerID := owner.String()
	if ownerID != user.User.Sid.String() && ownerID != "S-1-5-18" && ownerID != "S-1-5-32-544" {
		return fs.ErrPermission
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return fs.ErrPermission
	}
	expected, err := privateDescriptor()
	if err != nil {
		return err
	}
	wantedACL, _, err := expected.DACL()
	if err != nil {
		return err
	}
	actualACL, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	wantedText, err := aclText(wantedACL)
	if err != nil {
		return err
	}
	actualText, err := aclText(actualACL)
	if err != nil {
		return err
	}
	if actualText != wantedText {
		return fs.ErrPermission
	}
	return nil
}

func CopyPermissions(source, destination *os.File) error {
	descriptor, err := fileSecurity(source, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	return applyACL(destination, descriptor, control&windows.SE_DACL_PROTECTED != 0)
}
