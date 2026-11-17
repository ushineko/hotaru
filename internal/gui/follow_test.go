package gui_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/gui"
)

/*
A device's card says what it follows and changes it (spec 077 R3.2), and a
follower offers a match of its pace (R3.9), each through the service.
*/

// systemPage builds the System section headless, for a test that presses
// something on it.
func systemPage(t *testing.T, client *api.Client) fyne.CanvasObject {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	app := gui.New(client)
	opts := app.Options("/run/nowhere/hotaru.sock")
	opts.SettingsPath = filepath.Join(t.TempDir(), "gui.yml")
	sh := shell.Headless(a, opts)
	app.Refresh(context.Background())
	for _, s := range sh.Sections() {
		if s.Title() == "System" {
			return s.Build(sh)
		}
	}
	t.Fatal("no System section")
	return nil
}

// following is the healthy machine with the Aura following the Kraken.
func following() map[string]any {
	routes := healthy()
	devices := routes["GET /"+api.Version+"/devices"].(api.DevicesResponse)
	devices.Devices[0].Stored = []string{"Breathing", "Tai Chi"}
	devices.Devices[0].Modes = append(devices.Devices[0].Modes, "Tai Chi")
	devices.Devices = append(devices.Devices, api.Device{
		Name: "ASUS ROG MAXIMUS Z790 HERO", LEDs: 4, ActiveMode: "Static", InScope: true,
		Modes: []string{"Static", "Breathing", "Tai Chi"}, Follows: "kraken",
		Pace: map[string]float64{"breathing": 2.16},
	})
	routes["GET /"+api.Version+"/devices"] = devices
	routes["POST /"+api.Version+"/rules/device"] = api.RuleResponse{RulesFile: "hotaru.yml", Match: "maximus"}
	return routes
}

func TestEveryCardSaysWhatItFollows(t *testing.T) {
	said := screen(t, service(t, following()), "System")
	require.Contains(t, said, "Follows")
	require.Contains(t, said, "No one", "the Kraken follows nothing and does not say so")
	require.Contains(t, said, "Match pace", "a follower has no match")
}

func TestOnlyAFollowerOffersAMatch(t *testing.T) {
	said := screen(t, service(t, healthy()), "System")
	require.Contains(t, said, "Follows")
	require.NotContains(t, said, "Match pace")
}

// Choosing a leader reaches the service's rules route.
func TestChoosingALeaderWritesTheRule(t *testing.T) {
	client, asked := watching(t, following())
	page := systemPage(t, client)
	choice := fynetest.FindSelect(page)
	require.NotNil(t, choice)
	require.Equal(t, "No one", choice.Selected, "the first card is the Kraken's")
	drain(asked)

	choice.SetSelected("ASUS ROG MAXIMUS Z790 HERO")
	require.Eventually(t, func() bool {
		for {
			select {
			case route := <-asked:
				if route == "POST /"+api.Version+"/rules/device" {
					return true
				}
			default:
				return false
			}
		}
	}, 5*time.Second, 10*time.Millisecond)
}

// drain empties what the page asked while it was built.
func drain(asked chan string) {
	for {
		select {
		case <-asked:
		default:
			return
		}
	}
}
