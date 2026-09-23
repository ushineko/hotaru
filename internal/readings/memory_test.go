package readings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// meminfo writes a fixture and returns a reader over it.
func meminfo(t *testing.T, body string) *Memory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "meminfo")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return &Memory{path: path}
}

const realMeminfo = `MemTotal:       32659284 kB
MemFree:         1204100 kB
MemAvailable:   18395372 kB
Buffers:          621844 kB
Cached:         13820996 kB
SwapTotal:       8388604 kB
`

func TestMemoryIsWhatIsUsedNotWhatIsFree(t *testing.T) {
	/*
		Free memory on a machine that has been up an hour is small on almost
		every Linux box, because the kernel spends what nobody is using on
		cache and hands it back on demand. A panel drawing that would read
		alarming on a healthy machine, and would be wrong.

		Available is the kernel's estimate of what a workload could take
		without swapping. Used is the rest: 32659284 - 18395372 kB, which is
		about 13.6 GiB of 31.1, or 44%.
	*/
	percent, gigabytes, ok := meminfo(t, realMeminfo).Used()
	require.True(t, ok)
	require.InDelta(t, 43.7, percent, 0.1)
	require.InDelta(t, 13.6, gigabytes, 0.1)

	// And not the 96% that MemFree would have given.
	require.Less(t, percent, 90.0, "the reading is free memory, not used memory")
}

func TestAKernelWithoutMemAvailableReportsAbsence(t *testing.T) {
	/*
		MemAvailable arrived in Linux 3.14. Deriving it from MemFree on an
		older kernel would put a number on the panel that is confidently
		wrong, and the panel already knows how to draw a reading that is not
		there.
	*/
	_, _, ok := meminfo(t, "MemTotal:       32659284 kB\nMemFree:         1204100 kB\n").Used()
	require.False(t, ok)
}

func TestAMachineWithNoMeminfoReportsAbsence(t *testing.T) {
	_, _, ok := (&Memory{path: filepath.Join(t.TempDir(), "nothing")}).Used()
	require.False(t, ok)
}

func TestNonsenseInMeminfoIsNotAReading(t *testing.T) {
	// Zero total would be a division; available above total is not a machine.
	for _, body := range []string{
		"MemTotal:       0 kB\nMemAvailable:   100 kB\n",
		"MemTotal:       100 kB\nMemAvailable:   200 kB\n",
		"MemTotal:       banana kB\nMemAvailable:   100 kB\n",
	} {
		_, _, ok := meminfo(t, body).Used()
		require.False(t, ok, "drew a number from %q", body)
	}
}

func TestBothMemorySourcesAreOffered(t *testing.T) {
	// A source not in All is a source the editor and `hotaru readings` do
	// not know about, which is a reading nobody can choose.
	require.Contains(t, All, MemUsed)
	require.Contains(t, All, MemBytes)

	label, unit := Describe(MemUsed)
	require.Equal(t, "Memory", label)
	require.Equal(t, "%", unit)

	label, unit = Describe(MemBytes)
	require.Equal(t, "Memory", label)
	require.Equal(t, "GB", unit)
}
