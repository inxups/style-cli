package privatefs

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestM3A17WindowsDACLRefusesPublicRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = Restrict(file); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(name, windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	windows.CloseHandle(handle)
	if err != nil {
		t.Fatal(err)
	}
	if Check(file) == nil || CheckPath(path) == nil {
		t.Fatal("public Windows DACL accepted as 0600")
	}
	if err = Restrict(file); err != nil {
		t.Fatal(err)
	}
}
