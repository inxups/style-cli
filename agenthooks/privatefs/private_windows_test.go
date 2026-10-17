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
	handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
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

func TestM3A17WindowsLongUnicodePath(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 6; i++ {
		root = filepath.Join(root, "长目录 with spaces 123456789012345678901234567890")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := PrepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(root, "记录.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = Restrict(file); err != nil {
		t.Fatal(err)
	}
	if err = Check(file); err != nil {
		t.Fatal(err)
	}
}

func TestM3A17WindowsRootedPrivateObjects(t *testing.T) {
	home := t.TempDir()
	root, err := os.OpenRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.Mkdir("nested", 0700); err != nil {
		t.Fatal(err)
	}
	if err = PrepareRoot(root, "nested"); err != nil {
		t.Fatal(err)
	}
	nested, err := root.OpenRoot("nested")
	if err != nil {
		t.Fatal(err)
	}
	defer nested.Close()
	file, err := nested.OpenFile("record", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = Prepare(file); err != nil {
		t.Fatal(err)
	}
	if err = CheckRoot(nested, "record"); err != nil {
		t.Fatal(err)
	}
}
