//go:build windows

package config

import (
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

// On Windows the Unix mode bits carried by os.FileInfo are synthesized and
// enforce nothing, so confidentiality comes from an explicit DACL granting the
// current user and no one else. The DACL is protected, which stops permissive
// entries from being inherited from the parent directory.

func restrictDir(dir string) error { return applyOwnerOnlyACL(dir, true) }

// checkDirMode has no Windows counterpart: the ACL applied on write is the
// check, and it is verified in the CI matrix rather than from mode bits.
func checkDirMode(string) error { return nil }

func restrictFile(_ *os.File, path string) error { return applyOwnerOnlyACL(path, false) }

// checkSecretFile relies on the DACL instead of mode bits.
func checkSecretFile(fs.FileInfo, string) error { return nil }

func applyOwnerOnlyACL(path string, container bool) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	inheritance := uint32(windows.NO_INHERITANCE)
	if container {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	access := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}
	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return fmt.Errorf("build access control list for %s: %w", path, err)
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	)
	if err != nil {
		return fmt.Errorf("restrict %s to the current user: %w", path, err)
	}
	return nil
}

func currentUserSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read current user token: %w", err)
	}
	return user.User.Sid, nil
}
