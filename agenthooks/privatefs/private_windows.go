package privatefs

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func userSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

// Check inspects the opened object's DACL, never the emulated POSIX mode bits.
func Check(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return errors.New("private regular object required")
	}
	user, err := userSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return errors.New("private owner unavailable")
	}
	allowed := map[string]bool{user.String(): true, "S-1-5-18": true, "S-1-5-32-544": true}
	if !allowed[owner.String()] {
		return errors.New("private owner required")
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return errors.New("private DACL required")
	}
	ownAccess := false
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceSize < 12 {
			return errors.New("unsupported private ACE")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() || !allowed[sid.String()] {
			return errors.New("private DACL grants another principal")
		}
		if sid.Equals(user) && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 && uint32(ace.Mask)&0x1f01ff == 0x1f01ff {
			ownAccess = true
		}
	}
	if !ownAccess {
		return errors.New("private owner access required")
	}
	return nil
}

// Restrict reopens for WRITE_DAC, verifies identity, then updates that handle.
// OPEN_REPARSE_POINT prevents changing a replacement symlink's destination.
func Restrict(file *os.File) error {
	before, err := file.Stat()
	if err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(file.Name())
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(path, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	reopened := os.NewFile(uintptr(handle), file.Name())
	defer reopened.Close()
	after, err := reopened.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) || after.Mode()&os.ModeSymlink != 0 {
		return errors.New("private object changed")
	}
	user, err := userSID()
	if err != nil {
		return err
	}
	flags := ""
	if before.IsDir() {
		flags = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)(A;" + flags + ";FA;;;" + user.String() + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return err
	}
	return Check(file)
}
func Prepare(file *os.File) error { return Restrict(file) }
func PrepareDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("private directory required")
	}
	return Restrict(file)
}
