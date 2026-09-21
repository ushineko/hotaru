package gui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
coolingCard is the cooler's numbers, for the System section to draw.

It was a section of its own and was too small to be one: four readings about
a device the same page already lists, with its own entry in the navigation.
Folded in, it sits under the machine it belongs to.
*/
func coolingCard(got Snapshot) fyne.CanvasObject {
	if got.Cooling.Absent {
		/*
			Not an error, and drawn as such.

			"This machine has no cooler" is a fact somebody wants, and most
			machines are that machine. Showing it as a failure would put a
			red mark on every ordinary desktop.
		*/
		detail := got.Cooling.Detail
		if detail == "" {
			detail = "No supported liquid cooler on this machine."
		}
		return widgets.Note(detail, fd.StatusInfo)
	}

	status := fd.StatusGood
	if got.Cooling.PumpRPM == 0 {
		// The most alarming number this program can show, and the reason the
		// alert exists at all: a pump died silently once and the processor
		// throttled to 0.20 GHz behind a plausible-looking screen.
		status = fd.StatusBad
	}

	return widgets.Card(got.Cooling.Device,
		widgets.FactRow("Coolant", fmt.Sprintf("%.1f °C", got.Cooling.Coolant), coolantStatus(got.Cooling.Coolant)),
		widgets.FactRow("Pump", fmt.Sprintf("%d rpm (%d%%)", got.Cooling.PumpRPM, got.Cooling.PumpDuty), status),
		widgets.PlainRow("Fans", fmt.Sprintf("%d rpm (%d%%)", got.Cooling.FanRPM, got.Cooling.FanDuty)),
		widgets.PlainRow("Taken", taken(got.Cooling.Taken)),
	)
}

/*
coolantStatus grades the coolant, at the same thresholds the dashboard and the
alerts use.

One set of numbers across the program. A screen showing calm green while a
notification says critical is worse than either alone.
*/
func coolantStatus(c float64) fd.Status {
	switch {
	case c >= 60:
		return fd.StatusBad
	case c >= 50:
		return fd.StatusWarn
	}
	return fd.StatusGood
}

func taken(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	since := time.Since(at).Round(time.Second)
	if since < time.Second {
		return "just now"
	}
	return fmt.Sprintf("%s ago", since)
}

// cooler reports whether this machine has one, for the status bar.
func cooler(got Snapshot) (api.Cooling, bool) {
	return got.Cooling, got.Err == nil && !got.Cooling.Absent
}
