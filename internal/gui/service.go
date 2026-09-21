package gui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

// ServiceSection is what the service is doing, and the one thing the window
// can ask it to do: put the lights back.
type ServiceSection struct{ app *App }

// Title is the name in the navigation.
func (s *ServiceSection) Title() string { return "Service" }

// Icon is the navigation's icon for this section.
func (s *ServiceSection) Icon() fyne.Resource { return theme.ComputerIcon() }

// Build draws the section from the last snapshot. Stateless, as the shell
// wants: every change rebuilds it, so only this has to know every reason
// something is or is not shown.
func (s *ServiceSection) Build(sh *shell.Shell) fyne.CanvasObject {
	got := s.app.machine.Read()
	if got.Err != nil {
		return notRunning(got.Err)
	}

	health := got.Health
	rows := []fyne.CanvasObject{
		widgets.FactRow("State", health.State, healthStatus(health.State)),
		widgets.PlainRow("Version", health.Version),
		widgets.PlainRow("OpenRGB", health.Address),
		widgets.PlainRow("Devices", fmt.Sprintf("%d seen, %d in scope",
			health.Devices, health.InScope)),
	}
	if health.Detail != "" {
		rows = append(rows, widgets.Note(health.Detail, healthStatus(health.State)))
	}
	for _, remedy := range health.Remedies {
		// Health's whole point is that it names the remedy rather than the
		// symptom. Dropping them here would throw away the useful half.
		rows = append(rows, widgets.Note(remedy, fd.StatusInfo))
	}

	reconcile := widget.NewButtonWithIcon("Put the lights back", theme.ViewRefreshIcon(), func() {
		sh.Perform("reconciling", func(ctx context.Context) error {
			done, err := s.app.client.Reconcile(ctx)
			if err != nil {
				return err
			}
			// Said in full: a restore that reached fewer devices than were
			// recorded is unfinished, not successful, and a cold boot has
			// been seen finding two devices of six.
			if !done.Complete {
				sh.Flash(fmt.Sprintf("%d device(s) restored; still waiting for %v",
					done.Applied, done.Missing), fd.StatusWarn)
				return nil
			}
			sh.Flash(fmt.Sprintf("%d device(s) restored.", done.Applied), fd.StatusGood)
			return nil
		})
	})
	reconcile.Importance = widget.MediumImportance

	return container.NewVBox(
		title("The service"),
		widgets.Card("Health", rows...),
		widgets.Card("Desired state", reconcile),
	)
}

/*
notRunning is the ordinary first-run answer, not an error.

A window that opens onto a red failure because the thing it talks to has not
been started yet is a window that has misread its own situation. It says what
to type.
*/
func notRunning(err error) fyne.CanvasObject {
	return container.NewVBox(
		title("The service is not running"),
		widgets.Card("Start it",
			widget.NewLabel("systemctl --user start hotaru"),
			widgets.Note(err.Error(), fd.StatusWarn),
		),
	)
}

func healthStatus(state string) fd.Status {
	switch state {
	case "healthy":
		return fd.StatusGood
	case "unreachable":
		return fd.StatusBad
	}
	return fd.StatusWarn
}

// scoped counts the devices hotaru would drive, which is the number worth
// showing next to a device count.
func scoped(devices []api.Device) int {
	n := 0
	for _, device := range devices {
		if device.InScope {
			n++
		}
	}
	return n
}
