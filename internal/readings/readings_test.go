package readings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestANumberThatWasNotTakenIsAbsent(t *testing.T) {
	/*
		The whole reason a reading is a lookup rather than a struct of pairs.
		A sensor that has gone away costs its own number and nothing else,
		and the panel draws a placeholder rather than a zero -- a screen
		showing a pump at 0 RPM is reporting the most alarming number this
		machine can show, on no evidence.
	*/
	var r Reading
	r.Set(Coolant, 37.5)

	got, ok := r.Value(Coolant)
	require.True(t, ok)
	require.InDelta(t, 37.5, got, 0.001)

	_, ok = r.Value(PumpRPM)
	require.False(t, ok)
	require.Equal(t, Placeholder, r.Text(PumpRPM))
}

func TestACoolantKeepsItsDecimalAndTheRestDoNot(t *testing.T) {
	// 37.5 is a coolant temperature; 37.5 RPM is a pump nobody has.
	var r Reading
	r.Set(Coolant, 37.46)
	r.Set(PumpRPM, 2608)
	r.Set(CPULoad, 12.7)

	require.Equal(t, "37.5", r.Text(Coolant))
	require.Equal(t, "2608", r.Text(PumpRPM))
	require.Equal(t, "13", r.Text(CPULoad))
}

func TestEverySourceHasAName(t *testing.T) {
	// The CLI prints these and so does the window; a source with no label
	// would be a column of raw identifiers in one of them.
	for _, source := range All {
		label, _ := Describe(source)
		require.NotEmpty(t, label, "%s has no label", source)
	}
}

// stat writes a /proc/stat fixture with the given aggregate line.
func stat(t *testing.T, line string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stat")
	require.NoError(t, os.WriteFile(path, []byte(line+"\ncpu0 1 2 3 4 5 6 7 0 0 0\n"), 0o600))
	return path
}

func TestTheFirstProcessorReadingIsAbsent(t *testing.T) {
	/*
		Utilisation is a rate. The first call after start-up has nothing to
		difference against, and the since-boot average is the wrong answer
		rather than a rough one: a machine up for a week reads 4% while it
		compiles.
	*/
	c := &CPU{path: stat(t, "cpu  100 0 100 800 0 0 0 0 0 0")}

	_, ok := c.Load()
	require.False(t, ok, "a rate was reported from one sample")
}

func TestProcessorLoadIsTheBusyShareSinceTheLastCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stat")
	write := func(line string) {
		require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0o600))
	}

	// 200 busy jiffies, 800 idle.
	write("cpu  100 0 100 800 0 0 0 0 0 0")
	c := &CPU{path: path}
	_, ok := c.Load()
	require.False(t, ok)

	// 60 more busy, 40 more idle: 60% of the interval.
	write("cpu  140 0 120 830 10 0 0 0 0 0")
	load, ok := c.Load()
	require.True(t, ok)
	require.InDelta(t, 60, load, 0.001)
}

func TestWaitingOnADiskIsNotWorking(t *testing.T) {
	/*
		iowait counts as idle. Counting it as busy reads 100% through a large
		copy on a machine that is asleep, which is the opposite of what the
		number is for.
	*/
	dir := t.TempDir()
	path := filepath.Join(dir, "stat")
	require.NoError(t, os.WriteFile(path, []byte("cpu  0 0 0 0 0 0 0 0 0 0\n"), 0o600))

	c := &CPU{path: path}
	_, _ = c.Load()

	require.NoError(t, os.WriteFile(path, []byte("cpu  0 0 0 0 100 0 0 0 0 0\n"), 0o600))
	load, ok := c.Load()
	require.True(t, ok)
	require.Zero(t, load)
}

func TestAMachineWithNoProcStatSaysNothing(t *testing.T) {
	c := &CPU{path: filepath.Join(t.TempDir(), "absent")}
	_, ok := c.Load()
	require.False(t, ok)
}

func TestTheCardsOwnBusyFileIsPreferred(t *testing.T) {
	// AMD puts it in sysfs, where reading it costs a file open rather than a
	// process.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "card1", "device"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "card1", "device", "gpu_busy_percent"), []byte("42\n"), 0o600))

	load, ok := busyIn(filepath.Join(dir, "card*", "device", "gpu_busy_percent"))
	require.True(t, ok)
	require.InDelta(t, 42, load, 0.001)
}

func TestNoBusyFileIsNotAnError(t *testing.T) {
	// Every machine running NVIDIA's own driver, including the one this was
	// written on.
	_, ok := busyIn(filepath.Join(t.TempDir(), "card*", "device", "gpu_busy_percent"))
	require.False(t, ok)
}

func TestBothNumbersComeFromOneAnswer(t *testing.T) {
	/*
		Recorded from the development machine:

			$ nvidia-smi --query-gpu=temperature.gpu,utilization.gpu --format=csv,noheader
			40, 3 %

		One spawn for both. This project refuses to spawn a process per frame
		on the write path, and asking twice here would double a cost it only
		pays once per dashboard tick.
	*/
	temperature, load, gotTemp, gotLoad := ParseSMI("40, 3 %\n")

	require.True(t, gotTemp)
	require.InDelta(t, 40, temperature, 0.001)
	require.True(t, gotLoad)
	require.InDelta(t, 3, load, 0.001)
}

func TestAnAnswerThatIsNotANumberIsAbsence(t *testing.T) {
	// A format is not an API, and this one has changed before.
	for _, out := range []string{"", "\n", "[N/A], [N/A]", "no devices were found"} {
		_, _, gotTemp, gotLoad := ParseSMI(out)
		require.False(t, gotTemp, "%q was read as a temperature", out)
		require.False(t, gotLoad, "%q was read as a load", out)
	}
}
