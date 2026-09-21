package desktop

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
)

/*
Claim is a shortcut another program still holds.

KDE writes an entry per registered shortcut into kglobalshortcutsrc, and those
entries **outlive the script that made them**: unloading it does not remove
them, and neither does restarting KWin. Eighteen `AIOScene*` entries were
sitting in that file on the development machine, holding exactly the sequences
hotaru wanted, long after the program that made them had stopped registering
anything.

That matters because of how this fault presents: the registration succeeds, the
key reports as claimed, and pressing it does nothing at all.
*/
type Claim struct {
	// Entry is the identifier KDE stores it under, e.g. "AIOScene4".
	Entry string
	// Key is the sequence it holds.
	Key string
	// Component is the section of the file, usually "kwin".
	Component string
}

// ShortcutsFile is where KDE keeps them.
func ShortcutsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	if config := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(config) {
		return filepath.Join(config, "kglobalshortcutsrc"), nil
	}
	return filepath.Join(home, ".config", "kglobalshortcutsrc"), nil
}

/*
Claimed reads the file and reports entries holding the given sequences, other
than hotaru's own.

Read-only, always. The file is the desktop's, hotaru's rule is that anything it
can fix is offered rather than done, and a program that quietly edits another
program's configuration is the thing this project keeps declining to be.

What a claim means is narrower than it first appears, and the difference was
measured: hotaru registered its keys with all eighteen of the monitor's entries
present and **every key worked**. The grab belongs to the loaded script, not to
the line in the file. These are leftovers -- a program's recorded claim on a
sequence, outliving the program -- and they matter because KDE will refuse a
sequence to a different component and because nobody can tell from the outside
which case they are looking at.
*/
func Claimed(path string, wanted []string) ([]Claim, error) {
	file, err := os.Open(path) //nolint:gosec // the desktop's own configuration
	if err != nil {
		if os.IsNotExist(err) {
			// No KDE here, or it has never bound anything. Not a fault.
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	want := map[string]bool{}
	for _, key := range wanted {
		want[normalise(key)] = true
	}

	var out []Claim
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}
		entry, value, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(entry, "_k_friendly_name") {
			continue
		}
		// The value is "sequence,alternate,description".
		sequence, _, _ := strings.Cut(value, ",")
		if !want[normalise(sequence)] || strings.HasPrefix(entry, "Hotaru") {
			continue
		}
		out = append(out, Claim{Entry: strings.TrimSpace(entry), Key: sequence, Component: section})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

/*
Release asks KDE to forget entries, through the daemon that owns them.

**Editing the file does not work, and this is where that was learned.** The
entries were deleted from kglobalshortcutsrc, hotaru registered its eighteen
shortcuts, and all eighteen of the old ones were back in the file a second
later: `kglobalaccel` holds the table in memory and writes it out whenever
anything registers, so a deletion survives exactly until the next save.

`org.kde.KGlobalAccel.unregister(component, action)` is the supported way and
it does both halves -- the daemon forgets, and the file loses the line. It is
also the only route that works without logging out, which is what the
alternative amounted to.

Only ever called because somebody said yes.
*/
func Release(conn *dbus.Conn, entries []Claim) (int, error) {
	accel := conn.Object(accelName, accelPath)
	removed := 0
	for _, claim := range entries {
		var gone bool
		err := accel.Call(accelIface+".unregister", 0, claim.Component, claim.Entry).Store(&gone)
		if err != nil {
			return removed, fmt.Errorf("ask KDE to forget %s: %w", claim.Entry, err)
		}
		if gone {
			removed++
		}
	}
	return removed, nil
}

// KDE's shortcut registry, which owns the entries in kglobalshortcutsrc.
const (
	accelName  = "org.kde.kglobalaccel"
	accelPath  = dbus.ObjectPath("/kglobalaccel")
	accelIface = "org.kde.KGlobalAccel"
)

// normalise compares sequences the way KDE writes them, which varies in case
// and spacing between the file and the API.
func normalise(sequence string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(sequence), " ", ""))
}
