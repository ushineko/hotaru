package desktop

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
Release removes named entries from the file, keeping everything else.

Only ever called because somebody said yes. Rewritten line by line rather than
parsed and re-serialised: it is not hotaru's file, and the parts it does not
understand -- every other program's shortcuts, comments, ordering -- must come
out exactly as they went in.
*/
func Release(path string, entries []Claim) (int, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the desktop's own configuration
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}

	drop := map[string]bool{}
	for _, claim := range entries {
		drop[claim.Entry] = true
	}

	var kept []string
	removed := 0
	for _, line := range strings.Split(string(body), "\n") {
		entry, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && drop[strings.TrimSpace(entry)] {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if removed == 0 {
		return 0, nil
	}

	//nolint:gosec // the caller passes ShortcutsFile's path, which is built from $HOME
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o600); err != nil {
		return 0, fmt.Errorf("write %s: %w", path, err)
	}
	return removed, nil
}

// normalise compares sequences the way KDE writes them, which varies in case
// and spacing between the file and the API.
func normalise(sequence string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(sequence), " ", ""))
}
