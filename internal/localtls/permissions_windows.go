//go:build windows

package localtls

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

// Unix mode bits do not describe Windows security. Allow only the current
// account, LocalSystem and Administrators in the owner-controlled DACL.
func privatePermissions(path string, _ os.FileInfo) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if !owner.Equals(user.User.Sid) {
		return fmt.Errorf("local TLS path must be owned by the current account: %s", path)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil {
		return fmt.Errorf("local TLS path must have a private DACL: %s", path)
	}
	for i := uint16(0); i < acl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, uint32(i), &ace); err != nil {
			return err
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("unsupported local TLS ACL entry: %s", path)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Mask != 0 && !sid.Equals(user.User.Sid) && sid.String() != "S-1-5-18" && sid.String() != "S-1-5-32-544" {
			return fmt.Errorf("local TLS path grants access to another account; restrict its Windows ACL: %s", path)
		}
	}
	return nil
}
