package host_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/host"
)

func TestTheLogStampsEachLineAndRollsAtItsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hotaru.log")
	at := time.Date(2026, 11, 10, 23, 0, 0, 0, time.UTC)
	log, err := host.OpenLog(path, 64, func() time.Time { return at })
	require.NoError(t, err)

	_, err = log.Write([]byte("listening\nhalf a "))
	require.NoError(t, err)
	n, err := log.Write([]byte("line\n"))
	require.NoError(t, err)
	require.Equal(t, 5, n, "the bytes given, not the bytes written")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "2026-11-10T23:00:00Z listening\n2026-11-10T23:00:00Z half a line\n", string(got),
		"one stamp per line, however the line arrived")

	_, err = log.Write([]byte("past the limit\n"))
	require.NoError(t, err)
	old, err := os.ReadFile(path + ".1")
	require.NoError(t, err)
	require.Equal(t, string(got), string(old), "the full log is set aside whole")
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "2026-11-10T23:00:00Z past the limit\n", string(got))
	require.NoError(t, log.Close())

	again, err := host.OpenLog(path, 64, time.Now)
	require.NoError(t, err)
	_, err = again.Write([]byte("restarted\n"))
	require.NoError(t, err)
	require.NoError(t, again.Close())
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(got), "past the limit\n", "a restart appends")
}
