package host

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"
)

// LogLimit is the size at which the log is set aside and started again
// (spec 072 R4.1).
const LogLimit = 1 << 20

/*
Log is a file the service writes what it would print, for a service that has
no console: a Windows service. systemd keeps a unit's output itself, so Linux
never needs one.

Each line starts with the time, as the journal would add it. At limit bytes
the file is renamed with ".1", replacing an older one, and a new one is
started, so the log never takes more than twice the limit.
*/
type Log struct {
	mu    sync.Mutex
	path  string
	limit int64
	now   func() time.Time
	file  *os.File
	size  int64
	mid   bool // the last write ended inside a line
}

// OpenLog opens the log at path, appending to what is there.
func OpenLog(path string, limit int64, now func() time.Time) (*Log, error) {
	l := &Log{path: path, limit: limit, now: now}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the log: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("open the log: %w", err)
	}
	l.file, l.size = f, info.Size()
	return nil
}

// Write writes p, starting each line with the time. It reports len(p) on
// success, as an io.Writer must, whatever the stamps added.
func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size >= l.limit && !l.mid {
		if err := l.roll(); err != nil {
			return 0, err
		}
	}
	var b bytes.Buffer
	stamp := l.now().Format(time.RFC3339) + " "
	for rest := p; len(rest) > 0; {
		if !l.mid {
			b.WriteString(stamp)
		}
		line := rest
		if i := bytes.IndexByte(rest, '\n'); i >= 0 {
			line, rest = rest[:i+1], rest[i+1:]
			l.mid = false
		} else {
			rest = nil
			l.mid = true
		}
		b.Write(line)
	}
	n, err := l.file.Write(b.Bytes())
	l.size += int64(n)
	if err != nil {
		return 0, fmt.Errorf("write the log: %w", err)
	}
	return len(p), nil
}

func (l *Log) roll() error {
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close the log: %w", err)
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil {
		return fmt.Errorf("set the log aside: %w", err)
	}
	return l.open()
}

// Close closes the file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close the log: %w", err)
	}
	return nil
}
