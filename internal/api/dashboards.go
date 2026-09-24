package api

import (
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
Dashboards over the wire.

The translation is here and nowhere else: the store's type and the API's are
the same shape, and keeping the conversion in one file means a field added to
one and forgotten in the other is a compile error in this file rather than a
value that silently stops travelling.
*/

func asDashboard(one dashboard.Dashboard) Dashboard {
	out := Dashboard{
		Name: one.Name, Shipped: one.Shipped,
		Arrangement: one.Arrangement, Theme: one.Theme,
		Background: DashboardBackground{
			Kind: one.Background.Kind, Picture: one.Background.Picture, Dim: one.Background.Dim,
		},
		Headline: asSlot(one.Headline),
		Caption:  one.Caption,
		Units:    one.Units,
		Lettering: DashboardLettering{
			Font:   one.Lettering.Font,
			Labels: asText(one.Lettering.Labels),
			Values: asText(one.Lettering.Values),
		},
		Trail: DashboardTrail{
			Off:   one.Trail.Off,
			Below: string(one.Trail.Below),
			Above: string(one.Trail.Above),
		},
	}
	for _, ring := range one.Rings {
		out.Rings = append(out.Rings, string(ring))
	}
	for _, slot := range one.Slots {
		out.Slots = append(out.Slots, asSlot(slot))
	}
	return out
}

func asText(t dashboard.Text) DashboardText {
	return DashboardText{Size: t.Size, Colour: t.Colour, Outline: t.Outline}
}

func fromText(t DashboardText) dashboard.Text {
	return dashboard.Text{Size: t.Size, Colour: t.Colour, Outline: t.Outline}
}

func asSlot(slot dashboard.Slot) DashboardSlot {
	return DashboardSlot{
		Source: string(slot.Source), Second: string(slot.Second),
		Separator: slot.Separator, Label: slot.Label,
	}
}

func toDashboard(one Dashboard) dashboard.Dashboard {
	out := dashboard.Dashboard{
		Name: one.Name, Arrangement: one.Arrangement, Theme: one.Theme,
		Background: dashboard.Background{
			Kind: one.Background.Kind, Picture: one.Background.Picture, Dim: one.Background.Dim,
		},
		Headline: toSlot(one.Headline),
		Caption:  one.Caption,
		Units:    one.Units,
		Lettering: dashboard.Lettering{
			Font:   one.Lettering.Font,
			Labels: fromText(one.Lettering.Labels),
			Values: fromText(one.Lettering.Values),
		},
		Trail: dashboard.Trail{
			Off:   one.Trail.Off,
			Below: readings.Source(one.Trail.Below),
			Above: readings.Source(one.Trail.Above),
		},
	}
	for _, ring := range one.Rings {
		out.Rings = append(out.Rings, readings.Source(ring))
	}
	for _, slot := range one.Slots {
		out.Slots = append(out.Slots, toSlot(slot))
	}
	return out
}

func toSlot(slot DashboardSlot) dashboard.Slot {
	return dashboard.Slot{
		Source: readings.Source(slot.Source), Second: readings.Source(slot.Second),
		Separator: slot.Separator, Label: slot.Label,
	}
}

// arrangements is what an editor offers, named by the service so a window
// does not carry its own copy of the list.
func arrangements() []Arrangement {
	out := make([]Arrangement, 0, len(dashboard.Arrangements))
	for _, name := range dashboard.Arrangements {
		out = append(out, Arrangement{
			Name: name, Slots: dashboard.Slots(name), Rings: dashboard.Rings(name),
		})
	}
	return out
}

func themeNames() []string {
	all := dashboard.Themes()
	out := make([]string, 0, len(all))
	for _, theme := range all {
		out = append(out, theme.Name)
	}
	return out
}
