// Package testfs supplies real permission faults to tests only.
package testfs

import (
	"testing"

	"golang.org/x/sys/windows"
)

// Deny creates the same actual write failure as POSIX directory chmod. An
// explicitly opened WRITE_DAC handle restores the exact original DACL.
func WritableDirectory(t testing.TB, path string) func(bool) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	original, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	control, _, err := original.Control()
	if err != nil {
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	originalACL, _, err := original.DACL()
	if err != nil {
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	deny, err := windows.SecurityDescriptorFromString("D:P(D;;0x46;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	denyACL, _, err := deny.DACL()
	if err != nil {
		windows.CloseHandle(handle)
		t.Fatal(err)
	}
	restore := func() error {
		flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
		if control&windows.SE_DACL_PROTECTED != 0 {
			flags = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
		}
		return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, flags, nil, nil, originalACL, nil)
	}
	t.Cleanup(func() {
		if err := restore(); err != nil {
			t.Error(err)
		}
		windows.CloseHandle(handle)
	})
	return func(writable bool) {
		t.Helper()
		if writable {
			if err := restore(); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, denyACL, nil); err != nil {
			t.Fatal(err)
		}
	}
}
