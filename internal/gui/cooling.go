package gui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

// CoolingSection is the cooler's numbers, and the fact of there being no
// cooler when there is not one.
type CoolingSection struct{ app *App }

// Title is the name in the navigation.
func (c *CoolingSection) Title() string { return "Cooling" }

// Icon is the navigation's icon for this section.
func (c *CoolingSection) Icon() fyne.Resource { return theme.MediaRecordIcon() }

// Changed says this section draws the cooler and nothing else, so it redraws
// when a number it shows moves and not when a light changes colour.
func (c *CoolingSection) Changed(before, after Snapshot) bool {
	return !sameCooling(before.Cooling, after.Cooling) ||
		(before.Err == nil) != (after.Err == nil)
}

// Build draws the section from the last snapshot. Stateless, as the shell
// wants: every change rebuilds it, so only this has to know every reason
// something is or is not shown.
func (c *CoolingSection) Build(_ *shell.Shell) fyne.CanvasObject {
	got := c.app.machine.Read()
	if got.Err != nil {
		return notRunning(got.Err)
	}

	if got.Cooling.Absent {
		/*
			Not an error, and drawn as such.

			"This machine has no cooler" is a fact somebody wants, and most
			machines are that machine. Showing it as a failure would put a red
			mark on every ordinary desktop.
		*/
		detail := got.Cooling.Detail
		if detail == "" {
			detail = "No supported liquid cooler on this machine."
		}
		return container.NewVBox(
			title("No cooler"),
			widgets.Note(detail, fd.StatusInfo),
		)
	}

	status := fd.StatusGood
	if got.Cooling.PumpRPM == 0 {
		// The most alarming number this program can show, and the reason the
		// alert exists at all: a pump died silently once and the processor
		// throttled to 0.20 GHz behind a plausible-looking screen.
		status = fd.StatusBad
	}

	return container.NewVBox(
		title(got.Cooling.Device),
		widgets.Card("Now",
			widgets.FactRow("Coolant", fmt.Sprintf("%.1f °C", got.Cooling.Coolant), coolantStatus(got.Cooling.Coolant)),
			widgets.FactRow("Pump", fmt.Sprintf("%d rpm (%d%%)", got.Cooling.PumpRPM, got.Cooling.PumpDuty), status),
			widgets.PlainRow("Fans", fmt.Sprintf("%d rpm (%d%%)", got.Cooling.FanRPM, got.Cooling.FanDuty)),
			widgets.PlainRow("Taken", taken(got.Cooling.Taken)),
		),
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
