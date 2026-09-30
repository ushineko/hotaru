package cooler_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/sanshoku/hwmon"
)

func TestTheProcessorIsReadByLabel(t *testing.T) {
	/*
		The search is sanshoku's and tested there. This is the wiring: the
		CPU list, and degrees rather than the kernel's thousandths. The
		package chip is deliberately not the first directory.
	*/
	root := t.TempDir()
	write := func(dir string, files map[string]string) {
		d := filepath.Join(root, dir)
		require.NoError(t, os.MkdirAll(d, 0o755))
		for f, v := range files {
			require.NoError(t, os.WriteFile(filepath.Join(d, f), []byte(v+"\n"), 0o644))
		}
	}
	write("hwmon0", map[string]string{"name": "acpitz", "temp1_input": "27000"})
	write("hwmon5", map[string]string{
		"name": "coretemp", "temp1_label": "Core 8", "temp1_input": "43000",
		"temp2_label": "Package id 0", "temp2_input": "85000",
	})

	was := hwmon.Root
	hwmon.Root = root
	t.Cleanup(func() { hwmon.Root = was })

	got, err := cooler.Processor()
	require.NoError(t, err)
	require.InDelta(t, 85, got, 0.001)
}
