package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/testsock"
)

/*
The socket's access list is the whole authentication story on Windows, as its
mode is elsewhere: there is no port and no token.

What is checked is the list itself: protected from inheritance, and one entry,
for the user the test runs as. A second user trying to connect is not tried,
because a test cannot make one.
*/
func TestTheSocketIsThisUsersAlone(t *testing.T) {
	socket := testsock.Path(t)
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	sd, err := windows.GetNamedSecurityInfo(socket, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)

	control, _, err := sd.Control()
	require.NoError(t, err)
	require.NotZero(t, control&windows.SE_DACL_PROTECTED, "the directory's entries are inherited")

	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	// SDDL: D: the list, P protected (AI is a flag Windows adds), then one (A)llow of full access (FA) to a SID.
	require.Equal(t, "D:PAI(A;;FA;;;"+me.User.Sid.String()+")", sd.String())
}
