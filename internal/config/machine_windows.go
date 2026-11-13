package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// The rights in the access lists below, as Explorer names them.
const (
	fullControl   = 0x1f01ff // everything, as FILE_ALL_ACCESS
	modify        = 0x1301bf // read, write, execute and delete what is inside
	readAndRun    = 0x1200a9 // list, read and traverse
	readAndWrite  = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE
	inheritAll    = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	inheritNone   = windows.NO_INHERITANCE
	trustedOwners = "SYSTEM, Administrators or the service's own account"
)

// MachineSubdirs are the service's own directories under MachineDir.
var MachineSubdirs = []string{"state", "data", "run"} //nolint:gochecknoglobals // a fixed list

/*
PrepareMachine makes hotaru's directories of the machine and sets who may do
what in them (spec 072 R3.4). The Windows service calls it before it opens
anything.

Any user may make a directory in %ProgramData%, so a directory that is there
already is checked before it is trusted: its owner must be SYSTEM,
Administrators or this service. One made by somebody else stops the service,
because whoever made it could read and change what hotaru keeps there.

The settings directory is open to interactive users, who may change the
rules as the owner does on Linux. State, pictures and the socket's directory
are the service's, and interactive users may only read them.
*/
func PrepareMachine() error {
	root, err := MachineDir()
	if err != nil {
		return err
	}
	self, err := processUser()
	if err != nil {
		return err
	}
	if err := ownDir(root, self); err != nil {
		return err
	}
	if err := setAccess(root, self, access{windows.WinInteractiveSid, modify}); err != nil {
		return err
	}
	for _, sub := range MachineSubdirs {
		dir := filepath.Join(root, sub)
		if err := ownDir(dir, self); err != nil {
			return err
		}
		if err := setAccess(dir, self, access{windows.WinInteractiveSid, readAndRun}); err != nil {
			return err
		}
	}
	return nil
}

/*
SharedSocketAccess is the socket's access list as the service of the machine
sets it (spec 072 R3.5): full control for SYSTEM, Administrators and the
service, and read and write, which is what connecting needs, for interactive
users.
*/
func SharedSocketAccess(socket string) error {
	self, err := processUser()
	if err != nil {
		return err
	}
	entries, err := entriesFor(self, inheritNone, access{windows.WinInteractiveSid, readAndWrite})
	if err != nil {
		return err
	}
	return protect(socket, entries)
}

// ownDir makes dir, or checks that the one there is a directory with an
// owner hotaru can trust.
func ownDir(dir string, self *windows.SID) error {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.Mkdir(dir, 0o700); err != nil {
			return fmt.Errorf("make %s: %w", dir, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("look at %s: %w", dir, err)
	case !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0:
		return fmt.Errorf("%s is not a directory hotaru made; an administrator should remove it", dir)
	}
	owner, err := ownerOf(dir)
	if err != nil {
		return err
	}
	if !trusted(owner, self) {
		return fmt.Errorf("%s belongs to %s, not to %s, so hotaru will not keep its files there; an administrator should remove it",
			dir, accountName(owner), trustedOwners)
	}
	return nil
}

// trusted is whether owner is SYSTEM, Administrators or self.
func trusted(owner, self *windows.SID) bool {
	if owner.Equals(self) {
		return true
	}
	for _, known := range []windows.WELL_KNOWN_SID_TYPE{windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid} {
		sid, err := windows.CreateWellKnownSid(known)
		if err == nil && owner.Equals(sid) {
			return true
		}
	}
	return false
}

func ownerOf(path string) (*windows.SID, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return nil, fmt.Errorf("find who owns %s: %w", path, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return nil, fmt.Errorf("find who owns %s: %w", path, err)
	}
	return owner, nil
}

func accountName(sid *windows.SID) string {
	account, domain, _, err := sid.LookupAccount("")
	switch {
	case err != nil:
		return sid.String()
	case domain == "":
		return account
	}
	return domain + `\` + account
}

// access is one more entry beside the three every list has.
type access struct {
	who  windows.WELL_KNOWN_SID_TYPE
	mask windows.ACCESS_MASK
}

// setAccess replaces dir's list with full control for SYSTEM,
// Administrators and self, and extra, all inherited.
func setAccess(dir string, self *windows.SID, extra access) error {
	entries, err := entriesFor(self, inheritAll, extra)
	if err != nil {
		return err
	}
	return protect(dir, entries)
}

func entriesFor(self *windows.SID, inherit uint32, extra access) ([]windows.EXPLICIT_ACCESS, error) {
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, fmt.Errorf("name SYSTEM: %w", err)
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, fmt.Errorf("name Administrators: %w", err)
	}
	other, err := windows.CreateWellKnownSid(extra.who)
	if err != nil {
		return nil, fmt.Errorf("name the users: %w", err)
	}
	entry := func(sid *windows.SID, kind windows.TRUSTEE_TYPE, mask windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
		return windows.EXPLICIT_ACCESS{
			AccessPermissions: mask,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       inherit,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  kind,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		}
	}
	return []windows.EXPLICIT_ACCESS{
		entry(system, windows.TRUSTEE_IS_WELL_KNOWN_GROUP, fullControl),
		entry(admins, windows.TRUSTEE_IS_WELL_KNOWN_GROUP, fullControl),
		entry(self, windows.TRUSTEE_IS_USER, fullControl),
		entry(other, windows.TRUSTEE_IS_WELL_KNOWN_GROUP, extra.mask),
	}, nil
}

// protect replaces path's list with entries, and stops the parent's
// entries being inherited back in.
func protect(path string, entries []windows.EXPLICIT_ACCESS) error {
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("build the access list of %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		return fmt.Errorf("set who may use %s: %w", path, err)
	}
	return nil
}

// processUser is the SID of the account this process runs as.
func processUser() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("find the account this runs as: %w", err)
	}
	return user.User.Sid, nil
}
