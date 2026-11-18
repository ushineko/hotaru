package gui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hotaru/internal/api"
)

/*
A device's headers and names, on its card (spec 078 R5), as `hotaru light
length`, `light fans` and `light name` set them: each lights what it is about
while it is open, and writes through the service's rules route.
*/

// settable is a device's zones whose length it cannot count.
func settable(device api.Device) []api.Zone {
	var out []api.Zone
	for _, z := range device.Zones {
		if z.Max > 0 {
			out = append(out, z)
		}
	}
	return out
}

// lightsOf is a zone's lights first to last as a target.
func lightsOf(device, zone string, first, last int) string {
	return fmt.Sprintf("%s/%s[%d:%d]", device, zone, first, last)
}

// endMarked is a zone of n lit green, every tenth white and the last red.
func endMarked(device, zone string, n int) []api.Assignment {
	out := []api.Assignment{{Target: lightsOf(device, zone, 0, n-1), Colour: "green"}}
	for i := 9; i < n-1; i += 10 {
		out = append(out, api.Assignment{Target: lightsOf(device, zone, i, i), Colour: "white"})
	}
	return append(out, api.Assignment{Target: lightsOf(device, zone, n-1, n-1), Colour: "red"})
}

// holds are the choices for what a header holds; the index is the fans.
var holds = []string{"a strip", "1 fan", "2 fans", "3 fans", "4 fans"}

/*
holding is what a zone holds now: its named rings in the listing, or one fan
where the zone is a ring with no rings named in it, as the rear fan is, named
whole. Read as a strip, saving would have made it one (the live run).
*/
func holding(device api.Device, zone api.Zone) string {
	fans := 0
	for _, part := range device.Named {
		if strings.EqualFold(part.Zone, zone.Name) && part.First != nil {
			fans++
		}
	}
	if fans == 0 && zone.Shape == api.ShapeRing {
		fans = 1
	}
	if fans > 0 && fans < len(holds) {
		return holds[fans]
	}
	return holds[0]
}

/*
headers is the card's button for a device with headers: each header's length,
moved until its red end sits on the strip's last light, and what it holds.
*/
func (s *SystemSection) headers(sh *shell.Shell, device api.Device) fyne.CanvasObject {
	zones := settable(device)
	if len(zones) == 0 {
		return nil
	}
	return widget.NewButtonWithIcon("Headers…", theme.SettingsIcon(), func() { s.openHeaders(sh, device, zones) })
}

func (s *SystemSection) openHeaders(sh *shell.Shell, device api.Device, zones []api.Zone) {
	lengths := map[string]int{}
	held := map[string]string{}
	show := func(zone string, n int) {
		go func() {
			ctx := context.Background()
			if err := s.app.client.Length(ctx, device.Name, zone, n); err != nil {
				onScreen(func() { sh.Flash(err.Error(), fd.StatusBad) })
				return
			}
			_, _ = s.app.client.Apply(ctx, api.ApplyRequest{
				Devices: []string{device.Name}, Assignments: endMarked(device.Name, zone, n), Preview: true,
			})
		}()
	}

	rows := []fyne.CanvasObject{widgets.Note(
		"Move each header's end until the red light is on the strip's last light.", fd.StatusInfo)}
	for _, zone := range zones {
		name := zone.Name
		lengths[name] = max(zone.Count, 1)
		held[name] = holding(device, zone)
		entry := widget.NewEntry()
		entry.SetText(strconv.Itoa(lengths[name]))
		set := func(n int) {
			n = min(max(n, 1), zone.Max)
			lengths[name] = n
			entry.SetText(strconv.Itoa(n))
			show(name, n)
		}
		entry.OnSubmitted = func(text string) {
			if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
				set(n)
			}
		}
		less := widget.NewButton("−", func() { set(lengths[name] - 1) })
		more := widget.NewButton("+", func() { set(lengths[name] + 1) })
		choice := widget.NewSelect(holds, func(picked string) { held[name] = picked })
		choice.Selected = held[name]
		rows = append(rows,
			widgets.Dim(name),
			container.NewHBox(widget.NewLabel("Lights"), less, widgets.FixedWidth(entry, 70), more,
				widget.NewLabel("holds"), choice))
	}

	ask := dialog.NewCustomConfirm("Headers on "+device.Name, "Save", "Cancel", container.NewVBox(rows...),
		func(save bool) {
			chosen := map[string]int{}
			for k, v := range lengths {
				chosen[k] = v
			}
			kept := map[string]string{}
			for k, v := range held {
				kept[k] = v
			}
			sh.Perform("setting "+device.Name+"'s headers", func(ctx context.Context) error {
				defer func() { _, _ = s.app.client.Reconcile(context.WithoutCancel(ctx)) }()
				if !save {
					for _, zone := range zones {
						if zone.Count > 0 {
							_ = s.app.client.Length(ctx, device.Name, zone.Name, zone.Count)
						}
					}
					return nil
				}
				// Only what was changed: a header left alone keeps its rule.
				req := api.RuleRequest{Device: device.Name, Lengths: map[string]int{}, Fans: map[string]int{}, Shapes: map[string]string{}}
				for _, zone := range zones {
					if chosen[zone.Name] != zone.Count {
						req.Lengths[zone.Name] = chosen[zone.Name]
					}
				}
				for _, zone := range zones {
					picked := kept[zone.Name]
					if picked == holding(device, zone) {
						continue
					}
					if fans := slices.Index(holds, picked); fans > 0 {
						req.Fans[zone.Name] = fans
					} else {
						req.Shapes[zone.Name] = string(api.ShapeLine)
					}
				}
				if len(req.Lengths)+len(req.Fans)+len(req.Shapes) == 0 {
					return nil
				}
				if _, err := s.app.client.Rule(ctx, req); err != nil {
					return err
				}
				onScreen(func() { sh.Flash(device.Name+"'s headers are written to the rules.", fd.StatusGood) })
				return nil
			})
		}, sh.Window)
	ask.Resize(fyne.NewSize(560, ask.MinSize().Height))
	ask.Show()
	for _, zone := range zones {
		show(zone.Name, lengths[zone.Name])
	}
}

// part is one thing a name is given to: a zone, or lights of one.
type part struct {
	zone        string
	first, last int
	whole       bool
}

func (p part) target(device string) string {
	if p.whole {
		return device + "/" + p.zone
	}
	return lightsOf(device, p.zone, p.first, p.last)
}

func (p part) segment() *api.RuleSegment {
	if p.whole {
		return &api.RuleSegment{Zone: p.zone}
	}
	first, last := p.first, p.last
	return &api.RuleSegment{Zone: p.zone, First: &first, Last: &last}
}

func (p part) said() string {
	if p.whole {
		return p.zone
	}
	return fmt.Sprintf("%s, lights %d to %d", p.zone, p.first, p.last)
}

/*
parts is what a device's names are given to, in order, with the name each has:
each zone, or each of its fans where the device counts them or the rules
divide it.
*/
func parts(device api.Device) ([]part, []string) {
	var out []part
	var names []string
	nameOf := func(p part) string {
		for _, n := range device.Named {
			if !strings.EqualFold(n.Zone, p.zone) {
				continue
			}
			if p.whole && n.First == nil || !p.whole && n.First != nil && *n.First == p.first && *n.Last == p.last {
				return n.Name
			}
		}
		return ""
	}
	for _, zone := range device.Zones {
		var split []part
		first := 0
		for _, n := range zone.Parts {
			split = append(split, part{zone: zone.Name, first: first, last: first + n - 1})
			first += n
		}
		if len(split) < 2 {
			split = nil
			for _, n := range device.Named {
				if strings.EqualFold(n.Zone, zone.Name) && n.First != nil {
					split = append(split, part{zone: zone.Name, first: *n.First, last: *n.Last})
				}
			}
			slices.SortFunc(split, func(a, b part) int { return a.first - b.first })
		}
		if len(split) < 2 {
			split = []part{{zone: zone.Name, whole: true}}
		}
		for _, p := range split {
			out = append(out, p)
			names = append(names, nameOf(p))
		}
	}
	return out, names
}

// names is the card's button that names a device's zones and fans.
func (s *SystemSection) names(sh *shell.Shell, device api.Device) fyne.CanvasObject {
	return widget.NewButtonWithIcon("Names…", theme.DocumentCreateIcon(), func() { s.openNames(sh, device) })
}

// nameChanges is the segments that turn the names a device's parts had into
// the ones typed: a new name added, an old one gone, a cleared one removed.
func nameChanges(all []part, had, typed []string) map[string]*api.RuleSegment {
	out := map[string]*api.RuleSegment{}
	for i, p := range all {
		now := slug(typed[i])
		if now == had[i] {
			continue
		}
		if had[i] != "" {
			out[had[i]] = nil
		}
		if now != "" {
			out[now] = p.segment()
		}
	}
	return out
}

func (s *SystemSection) openNames(sh *shell.Shell, device api.Device) {
	all, had := parts(device)
	entries := make([]*widget.Entry, len(all))
	rows := []fyne.CanvasObject{widgets.Note(
		"Names let a scene say rear-fan=red. Show lights a part white; an empty name removes it.", fd.StatusInfo)}
	for i, p := range all {
		entries[i] = widget.NewEntry()
		entries[i].SetText(had[i])
		entries[i].SetPlaceHolder("no name")
		target := p.target(device.Name)
		lit := widget.NewButton("Show", func() {
			go func() {
				_, _ = s.app.client.Apply(context.Background(), api.ApplyRequest{
					Devices: []string{device.Name}, Preview: true,
					Assignments: []api.Assignment{{Target: target, Colour: "white"}},
				})
			}()
		})
		rows = append(rows, container.NewBorder(nil, nil, widgets.FixedWidth(widgets.Dim(p.said()), 240), lit, entries[i]))
	}
	ask := dialog.NewCustomConfirm("Names on "+device.Name, "Save", "Cancel",
		container.NewVScroll(container.NewVBox(rows...)), func(save bool) {
			typed := make([]string, len(entries))
			for i, e := range entries {
				typed[i] = e.Text
			}
			sh.Perform("naming "+device.Name+"'s parts", func(ctx context.Context) error {
				defer func() { _, _ = s.app.client.Reconcile(context.WithoutCancel(ctx)) }()
				changes := nameChanges(all, had, typed)
				if !save || len(changes) == 0 {
					return nil
				}
				if _, err := s.app.client.Rule(ctx, api.RuleRequest{Device: device.Name, Segments: changes}); err != nil {
					return err
				}
				onScreen(func() { sh.Flash(device.Name+"'s names are written to the rules.", fd.StatusGood) })
				return nil
			})
		}, sh.Window)
	roomy(ask, sh)
}

// slug is a name as the rules keep it: lowercase words and dashes.
func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var out strings.Builder
	dash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			dash = false
		case !dash && out.Len() > 0:
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(out.String(), "-")
}
