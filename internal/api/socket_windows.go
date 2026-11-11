package api

import (
	"fmt"

	"golang.org/x/sys/windows"
)

/*
restrict makes the socket this user's alone.

A file mode means nothing here: Windows reports 0666 for a socket whoever may
open it. What decides who can connect is the file's access list, so the list
is replaced with one entry, full control for the user the service runs as,
and marked protected so the directory's entries are not inherited back in.

Connecting to a Unix socket on Windows needs write access to its file, which
is what the list withholds from everyone else.
*/
func restrict(socket string) error {
	user, err := currentUser()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user),
		},
	}}, nil)
	if err != nil {
		return fmt.Errorf("build the access list: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(socket, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		return fmt.Errorf("replace its access list: %w", err)
	}
	return nil
}

// currentUser is the SID of the user this process runs as.
func currentUser() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("find the user this runs as: %w", err)
	}
	return user.User.Sid, nil
}
