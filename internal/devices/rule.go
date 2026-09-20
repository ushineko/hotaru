package devices

import (
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
	}
	return out
}

// RuleFor is MergeRules over the rules a configuration holds for this device.
func RuleFor(cfg *config.Config, device string) Rule {
	if cfg == nil {
		return Rule{}
	}
	return MergeRules(cfg.RulesFor(device))
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
