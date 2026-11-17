package devices

import (
	"strings"
	"time"

	"github.com/ushineko/hotaru/internal/config"
)

/*
Rule is the corrections that apply to one device: every matching rule in the
configuration file, merged.

Several rules may match one device — "kraken" and "nzxt" both match the same
cooler — and they apply in file order with later values winning. Merging here
rather than at every call site means the precedence is defined once and is
testable on its own.
*/
type Rule struct {
	SolidModes []string
	NeverBlank bool
	Brightness *int
	Reassert   time.Duration
	Segments   map[string]config.Segment
	// Toggles are the segments that are switches rather than decoration. See
	// config.DeviceRule.
	Toggles []string
	// FrameInterval and Twin are for a device hotaru draws itself. See
	// config.DeviceRule.
	FrameInterval time.Duration
	Twin          string
	// Formerly is the device's OpenRGB name and zones, for one OpenRGB no
	// longer lists (spec 073). See config.DeviceRule.
	Formerly *config.Former
	// Lengths are the LED counts of zones the device cannot read (spec
	// 075). See config.DeviceRule.
	Lengths map[string]int
	// Colour corrects what is written to lights that wash out (spec 066).
	Colour Profile
}

/*
DefaultSolidModes is the order a solid colour is attempted in, for a device with
no rule of its own.

Static first because a GPU that rejects it is loud about it — it errors, and
resolution moves on — while a board that renders static as "onboard LED only"
fails silently, dark headers and a success code. Preferring the mode whose
failure is visible is the safer default, and a rule inverts it where a machine
has learned otherwise.
*/
var DefaultSolidModes = []string{"static", "direct"}

/*
DefaultOffModes is how "off" is attempted.

Direct with black is the fallback for a device with no Off mode. It is also why
NeverBlank exists: on a keyboard, black is not "off", it is a dead backlight.
*/
var DefaultOffModes = []string{"off", "direct"}

// MergeRules combines every rule matching a device, in order, later winning.
func MergeRules(rules []config.DeviceRule) Rule {
	var out Rule
	for _, r := range rules {
		if len(r.SolidModes) > 0 {
			out.SolidModes = r.SolidModes
		}
		if r.NeverBlank {
			out.NeverBlank = true
		}
		if r.Brightness != nil {
			b := *r.Brightness
			out.Brightness = &b
		}
		if r.Reassert > 0 {
			out.Reassert = r.Reassert.Duration()
		}
		for name, seg := range r.Segments {
			if out.Segments == nil {
				out.Segments = map[string]config.Segment{}
			}
			out.Segments[name] = seg
		}
		if len(r.Toggles) > 0 {
			out.Toggles = append([]string(nil), r.Toggles...)
		}
		if r.FrameInterval > 0 {
			out.FrameInterval = r.FrameInterval.Duration()
		}
		if strings.TrimSpace(r.Twin) != "" {
			out.Twin = strings.TrimSpace(r.Twin)
		}
		for zone, n := range r.Lengths {
			if out.Lengths == nil {
				out.Lengths = map[string]int{}
			}
			out.Lengths[zone] = n
		}
		if r.Formerly != nil && strings.TrimSpace(r.Formerly.Name) != "" {
			former := *r.Formerly
			out.Formerly = &former
		}
		// One setting at a time, so one rule can give saturation and a
		// later one value.
		if r.Colour != nil && r.Colour.Saturation != nil {
			out.Colour.Saturation = *r.Colour.Saturation
		}
		if r.Colour != nil && r.Colour.Value != nil {
			out.Colour.Value = *r.Colour.Value
		}
		if r.Colour != nil && r.Colour.Curve != nil {
			for _, channel := range []struct {
				from *float64
				to   *float64
			}{
				{r.Colour.Curve.Red, &out.Colour.Red},
				{r.Colour.Curve.Green, &out.Colour.Green},
				{r.Colour.Curve.Blue, &out.Colour.Blue},
			} {
				if channel.from != nil {
					*channel.to = *channel.from
				}
			}
		}
	}
	return out
}

// RuleFor is MergeRules over the rules a configuration holds for this device.
func RuleFor(cfg *config.Config, device string) Rule {
	if cfg == nil {
		return Rule{}
	}
	return Renamed(MergeRules(cfg.RulesFor(device)), device)
}

/*
Renamed is a rule as it applies to the device it is for. A rule with a former
name, applied to the device under its new one, has its segments' zones renamed
through the former zones, so a segment written as {zone: "Hue 2 Channel 2"}
finds the canvas's Fan zone (spec 073). Under the former name it is unchanged.
*/
func Renamed(r Rule, device string) Rule {
	if r.Formerly == nil || strings.EqualFold(device, r.Formerly.Name) || len(r.Formerly.Zones) == 0 {
		return r
	}
	segments := make(map[string]config.Segment, len(r.Segments))
	for name, seg := range r.Segments {
		if to, ok := r.Formerly.Zones[seg.Zone]; ok {
			seg.Zone = to
		}
		segments[name] = seg
	}
	r.Segments = segments
	return r
}

// namesSolidModes reports whether the rule states its own order, as opposed to
// falling back on hotaru's default.
func (r Rule) namesSolidModes() bool { return len(r.SolidModes) > 0 }

// solidOrder is the preference order this rule asks for, or the default.
func (r Rule) solidOrder() []string {
	if len(r.SolidModes) > 0 {
		return r.SolidModes
	}
	return DefaultSolidModes
}
