package service

import (
	"strings"

	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
)

// twinsByCanvas is each handed OpenRGB listing, by the canvas device that
// draws its hardware.
func twinsByCanvas(found []devices.Device) map[string]*devices.Device {
	out := map[string]*devices.Device{}
	for i := range found {
		if to := found[i].HandedTo; to != "" {
			out[to] = &found[i]
		}
	}
	return out
}

/*
carry is the scene's assignments for a handed OpenRGB listing, written
instead against the canvas device that draws the same hardware.

A scene written before the canvas existed names the listing: a picture
scene colours it light by light ("…/Keyboard[16:17]"), and hotaru no longer
writes that listing, so without this the keyboard would drop out of every
such scene (spec 060). An assignment that covers the whole listing becomes
one for the whole canvas. Otherwise each light is carried by its name: the
server's ("Key: Page Up") and the canvas's ("Page Up") are the same key once
normalised. A light whose name the canvas does not have is left out; the two
devices do not number their lights the same way, so the index means nothing
across them.
*/
func carry(twin, canvas *devices.Device, rule devices.Rule, assignments []devices.Assignment) []devices.Assignment {
	if len(assignments) == 0 {
		return nil
	}
	keys := make(map[string]int, len(canvas.LEDNames))
	for i, name := range canvas.LEDNames {
		keys[keyName(name)] = i
	}
	var out []devices.Assignment
	for _, a := range assignments {
		spans, err := twin.ResolveTarget(a.Target, rule)
		if err != nil {
			continue
		}
		if len(spans) == 1 && spans[0].First == 0 && spans[0].Count == twin.LEDCount {
			out = append(out, devices.Assignment{Target: devices.Target{Device: canvas.Name}, Colour: a.Colour})
			continue
		}
		var picks []config.LEDs
		for _, span := range spans {
			for led := span.First; led <= span.Last(); led++ {
				if led >= len(twin.LEDNames) {
					break
				}
				if at, ok := keys[keyName(twin.LEDNames[led])]; ok {
					picks = append(picks, config.LEDs{First: at, Last: at})
				}
			}
		}
		if len(picks) > 0 {
			out = append(out, devices.Assignment{Target: devices.Target{Device: canvas.Name, Picks: picks}, Colour: a.Colour})
		}
	}
	return out
}

// keyAliases are OpenRGB's key names that a canvas names differently, after
// keyName's other steps.
var keyAliases = map[string]string{
	"up arrow":      "up",
	"down arrow":    "down",
	"left arrow":    "left",
	"right arrow":   "right",
	"left windows":  "left gui",
	"right windows": "right gui",
}

/*
keyName is a light's name reduced to the key it is, so that two devices'
names for one key compare equal: lower case, without OpenRGB's "Key: " and
its layout suffixes (" (ANSI)", " (ISO)"), with the few words that differ
mapped across.
*/
func keyName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "key: ")
	n = strings.TrimSuffix(n, " (ansi)")
	n = strings.TrimSuffix(n, " (iso)")
	if alias, ok := keyAliases[n]; ok {
		return alias
	}
	return n
}
