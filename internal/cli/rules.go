package cli

import (
	"strings"

	"github.com/ushineko/hotaru/internal/config"
)

// yamlFor renders a whole configuration: config.Render, kept under the name
// the wizard and light match use.
func yamlFor(cfg *config.Config) string { return config.Render(cfg) }

/*
merge puts what the wizard learned into the configuration that was there.

A rule for a device the wizard did not ask about is left exactly as it was:
somebody mapping one stick of RAM should not lose the rules for their cooler.
A device it did map has its segments replaced rather than added to, because the
answers just given are the current truth about that hardware.
*/
func merge(existing *config.Config, segmentsFound []namedSegment, learned map[string]notes, present []string) *config.Config {
	out := &config.Config{Scope: existing.Scope}
	out.Devices = append(out.Devices, existing.Devices...)

	byDevice := map[string][]namedSegment{}
	for _, segment := range segmentsFound {
		byDevice[segment.Device] = append(byDevice[segment.Device], segment)
	}

	for device, segments := range byDevice {
		rule := ruleOf(out, device, present)
		if rule == nil {
			out.Devices = append(out.Devices, config.DeviceRule{Match: matchFor(device, present)})
			rule = &out.Devices[len(out.Devices)-1]
		}

		note := learned[device]
		if len(note.solidModes) > 0 {
			rule.SolidModes = note.solidModes
		}
		// Written only when the wizard found it to be true. A file that says a
		// device was checked when somebody pressed return is worse than one
		// that says nothing about it.
		if note.neverBlank {
			rule.NeverBlank = true
		}
		if note.brightness != nil {
			rule.Brightness = note.brightness
		}
		if note.plainAgain {
			rule.SolidModes = nil
		}
		if note.undimmable {
			// Including one this wizard wrote itself, before it knew that a
			// brightness belongs to a mode rather than to a device.
			rule.Brightness = nil
		}
		for zone, n := range note.lengths {
			if rule.Lengths == nil {
				rule.Lengths = map[string]int{}
			}
			rule.Lengths[zone] = n
		}
		/*
			The segments are this run's answers, and the switches kept: a
			toggle (caps lock's key) is named by hand, never asked about, and
			replacing it lost the keyboard's caps and num (the live run).
		*/
		kept := map[string]config.Segment{}
		for _, name := range rule.Toggles {
			if segment, ok := rule.Segments[name]; ok {
				kept[name] = segment
			}
		}
		rule.Segments = kept
		for _, segment := range segments {
			entry := config.Segment{Zone: segment.Zone}
			if !segment.Whole {
				entry.LEDs = &config.LEDs{First: segment.First, Last: segment.Last}
			}
			rule.Segments[segment.Name] = entry
		}
	}
	return out
}

/*
ruleOf is the rule a device's answers belong in: the one with the match the
wizard would write, else the last whose match is in the device's name or the
name it formerly had, as the service matches them. Exact-only found nothing
for a rule written by hand ("kraken" for "NZXT Kraken Elite V2"), so nothing
was offered back and a second rule was added (the live run).
*/
func ruleOf(cfg *config.Config, device string, present []string) *config.DeviceRule {
	if rule := findRule(cfg, matchFor(device, present)); rule != nil {
		return rule
	}
	lowered := strings.ToLower(device)
	for i := len(cfg.Devices) - 1; i >= 0; i-- {
		rule := &cfg.Devices[i]
		match := strings.ToLower(rule.Match)
		if match == "" {
			continue
		}
		if strings.Contains(lowered, match) {
			return rule
		}
	}
	return nil
}

func findRule(cfg *config.Config, match string) *config.DeviceRule {
	for i := range cfg.Devices {
		if strings.EqualFold(cfg.Devices[i].Match, match) {
			return &cfg.Devices[i]
		}
	}
	return nil
}

/*
knownAs is what this zone was called last time, for offering as a default.

A person re-running the wizard has already answered these questions once, and
making them type "rad-front" again to keep it is the sort of thing that stops
people re-running anything.
*/
func knownAs(cfg *config.Config, device, zone string, present []string) string {
	rule := ruleOf(cfg, device, present)
	if rule == nil {
		return ""
	}
	for name, segment := range rule.Segments {
		if sameZone(rule, segment.Zone, zone) && segment.LEDs == nil {
			return name
		}
	}
	return ""
}

// knownRange is what a range of a zone was called last time, or nothing.
func knownRange(cfg *config.Config, device, zone string, first, last int, present []string) string {
	rule := ruleOf(cfg, device, present)
	if rule == nil {
		return ""
	}
	for name, segment := range rule.Segments {
		if sameZone(rule, segment.Zone, zone) && segment.LEDs != nil &&
			segment.LEDs.First == first && segment.LEDs.Last == last {
			return name
		}
	}
	return ""
}

// sameZone is whether a segment's zone is this zone, under its name now or
// the one it had before the device was drawn by hotaru (formerly).
func sameZone(rule *config.DeviceRule, segment, zone string) bool {
	if strings.EqualFold(segment, zone) {
		return true
	}
	if rule.Formerly == nil {
		return false
	}
	for was, now := range rule.Formerly.Zones {
		if strings.EqualFold(was, segment) && strings.EqualFold(now, zone) {
			return true
		}
	}
	return false
}
