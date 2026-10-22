package rumable

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var securityDLL = syscall.NewLazyDLL("advapi32.dll")
var convertSecurityDescriptor = securityDLL.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
var getSecurityDescriptorDACL = securityDLL.NewProc("GetSecurityDescriptorDacl")
var setNamedSecurityInfo = securityDLL.NewProc("SetNamedSecurityInfoW")
var freeSecurityDescriptor = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")

// Chmod(0600) does not set Windows ACLs. A protected DACL prevents inherited
// Users/IIS access to private JSON, bridge scripts and their temporary files.
func protectPrivateFile(path string) error {
	return setProtectedDACL(path, "D:P(A;;FA;;;SY)(A;;FA;;;BA)")
}

func protectSiteDirectory(root string, s Site) error {
	if s.IIS == nil {
		return nil
	}
	dir := siteDir(root, s)
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}
	err := os.Mkdir(dir, 0755)
	if err == nil {
		if err := setProtectedDACL(dir, "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"); err != nil {
			return err
		}
	} else if !os.IsExist(err) {
		return err
	}
	// Preserve existing directory/public ACLs during an old-journal rollback.
	// Harden known private files individually; never recursively reset a user's
	// state root or interrupt access to the previously published SDK assets.
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid managed site directory: %s", dir)
	}
	for _, name := range []string{"state.json", "transaction.json", "iis.ps1", "otel-iis.ps1", "iis-rum.ps1", "iis-acl.ps1"} {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := protectPrivateFile(path); err != nil {
			return err
		}
	}
	return nil
}

func setProtectedDACL(path, sddl string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing ACL change through a reparse point: %s", path)
	}
	text, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		return err
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var descriptor, dacl uintptr
	var present, defaulted uint32
	ok, _, callErr := convertSecurityDescriptor.Call(uintptr(unsafe.Pointer(text)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		return fmt.Errorf("create private DACL: %w", callErr)
	}
	defer freeSecurityDescriptor.Call(descriptor)
	ok, _, callErr = getSecurityDescriptorDACL.Call(descriptor, uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&dacl)), uintptr(unsafe.Pointer(&defaulted)))
	if ok == 0 || present == 0 || dacl == 0 {
		return fmt.Errorf("read private DACL: %v", callErr)
	}
	// SE_FILE_OBJECT, DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION.
	status, _, _ := setNamedSecurityInfo.Call(uintptr(unsafe.Pointer(name)), 1, 0x80000004, 0, 0, dacl, 0)
	if status != 0 {
		return fmt.Errorf("set private DACL for %s: %w", path, syscall.Errno(status))
	}
	return nil
}
