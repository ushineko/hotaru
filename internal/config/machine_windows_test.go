package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// machineFor is a made-up machine: %ProgramData% and a profile in temporary
// directories, no XDG variable, and machine mode on until the test ends.
func machineFor(t *testing.T) (programData, roaming string) {
	t.Helper()
	programData, roaming = t.TempDir(), t.TempDir()
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}
	t.Setenv("ProgramData", programData)
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	SetMachine(true)
	t.Cleanup(func() { SetMachine(false) })
	return programData, roaming
}

func TestTheMachinesFilesAreUnderProgramData(t *testing.T) {
	programData, roaming := machineFor(t)
	root := filepath.Join(programData, "hotaru")
	for _, c := range []struct {
		get  func() (string, error)
		want string
	}{
		{RulesPath, filepath.Join(root, "hotaru.yml")},
		{StatePath, filepath.Join(root, "state", "state.yml")},
		{DataDir, filepath.Join(root, "data")},
		{SocketPath, filepath.Join(root, "run", "hotaru.sock")},
		{UserDir, filepath.Join(roaming, "hotaru")},
	} {
		got, err := c.get()
		require.NoError(t, err)
		require.Equal(t, c.want, got)
	}

	SetMachine(false)
	dir, err := Dir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(roaming, "hotaru"), dir, "a person's files without the mode")
	SetMachine(true)

	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	state, err := StatePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(xdg, "hotaru", "state.yml"), state, "an XDG variable still wins")
}

// sddl is path's access list, written as Windows writes it.
func sddl(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	return sd.String()
}

func me(t *testing.T) *windows.SID {
	t.Helper()
	sid, err := processUser()
	require.NoError(t, err)
	return sid
}

func TestPreparingTheMachineSetsWhoMayDoWhat(t *testing.T) {
	programData, _ := machineFor(t)
	require.NoError(t, PrepareMachine())
	self := me(t).String()

	root := filepath.Join(programData, "hotaru")
	got := sddl(t, root)
	require.True(t, strings.HasPrefix(got, "D:PAI"), "protected from %%ProgramData%%'s entries: %s", got)
	for _, entry := range []string{"(A;OICI;FA;;;SY)", "(A;OICI;FA;;;BA)", "(A;OICI;FA;;;" + self + ")", "(A;OICI;0x1301bf;;;IU)"} {
		require.Contains(t, got, entry)
	}
	require.Equal(t, 4, strings.Count(got, "(A;"), got)

	for _, sub := range MachineSubdirs {
		got := sddl(t, filepath.Join(root, sub))
		require.True(t, strings.HasPrefix(got, "D:PAI"), got)
		require.Contains(t, got, "(A;OICI;0x1200a9;;;IU)", "signed-in users read %s and change nothing", sub)
		require.Contains(t, got, "(A;OICI;FA;;;"+self+")")
		require.Equal(t, 4, strings.Count(got, "(A;"), got)
	}

	require.NoError(t, PrepareMachine(), "a second start finds its own directories and keeps them")
}

func TestTheSharedSocketIsOpenToSignedInUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hotaru.sock")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, SharedSocketAccess(path))
	got := sddl(t, path)
	require.True(t, strings.HasPrefix(got, "D:PAI"), got)
	require.Contains(t, got, "(A;;FA;;;"+me(t).String()+")")
	require.Contains(t, got, "(A;;0x12019f;;;IU)", "read and write, which connecting needs")
	require.Equal(t, 4, strings.Count(got, "(A;"), got)
}

func TestADirectoryThatIsNotOneIsRefused(t *testing.T) {
	programData, _ := machineFor(t)
	require.NoError(t, os.WriteFile(filepath.Join(programData, "hotaru"), nil, 0o600))
	require.ErrorContains(t, PrepareMachine(), "an administrator should remove it")
}

func TestOnlySystemAdministratorsOrTheServiceAreTrustedOwners(t *testing.T) {
	self := me(t)
	well := func(kind windows.WELL_KNOWN_SID_TYPE) *windows.SID {
		sid, err := windows.CreateWellKnownSid(kind)
		require.NoError(t, err)
		return sid
	}
	require.True(t, trusted(self, self))
	require.True(t, trusted(well(windows.WinLocalSystemSid), self))
	require.True(t, trusted(well(windows.WinBuiltinAdministratorsSid), self))
	require.False(t, trusted(well(windows.WinBuiltinUsersSid), self), "any user may make a directory in %%ProgramData%%")
	require.False(t, trusted(well(windows.WinInteractiveSid), self))
}
