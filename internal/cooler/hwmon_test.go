package cooler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// hwmon writes a sensor tree, with the wanted chip deliberately not first.
func hwmon(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(dir, name string, files map[string]string) {
		d := filepath.Join(root, dir)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "name"), []byte(name+"\n"), 0o644))
		for f, v := range files {
			require.NoError(t, os.WriteFile(filepath.Join(d, f), []byte(v+"\n"), 0o644))
		}
	}
	write("hwmon0", "acpitz", map[string]string{"temp1_input": "27000"})
	write("hwmon4", "nct6798", map[string]string{
		"temp1_label": "SYSTIN", "temp1_input": "35000",
	})
	write("hwmon5", "coretemp", map[string]string{
		"temp1_label": "Core 8", "temp1_input": "43000",
		"temp2_label": "Package id 0", "temp2_input": "85000",
	})
	return root
}

func TestASensorIsFoundByItsLabel(t *testing.T) {
	/*
		By label, never by hwmon index. The numbers are assigned in probe
		order and move between boots, so a program that remembers hwmon5
		reports some other chip's temperature after a reboot -- and reports it
		confidently.
	*/
	got, err := CPUPackage.read(hwmon(t))
	require.NoError(t, err)
	require.Equal(t, 85, got)
}

func TestALabelOnTheWrongChipIsNotMatched(t *testing.T) {
	// "SYSTIN" exists on the board chip and not on the CPU. Asking the wrong
	// chip for it must fail rather than find something similar.
	_, err := Sensor{Chip: "coretemp", Label: "SYSTIN"}.read(hwmon(t))
	require.ErrorContains(t, err, "no sensor")
}

func TestAMissingSensorIsAnErrorNotAZero(t *testing.T) {
	// Zero degrees is a plausible-looking lie, and the dashboard would draw
	// it without comment.
	_, err := Sensor{Chip: "nosuchchip", Label: "whatever"}.read(hwmon(t))
	require.Error(t, err)
}
