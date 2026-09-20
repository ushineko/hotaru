/*
Package config reads hotaru's configuration.

Two rules shape this package, both from specs/001.

**With no configuration at all, hotaru works.** An absent file is an empty
Config, and an empty Config means every device present is in scope. Rules
narrow and correct; they never enable. Shipping one desk's device names as
defaults would leave hotaru lighting nothing on anyone else's machine, with no
clue why.

**A malformed entry costs that entry.** Load reports what is wrong with each
one it dropped and returns the rest, because a typo in the third rule should
not take the other four with it. Problems are returned, never logged: this
package has no opinion about where a message should appear.
*/
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/ushineko/fynedesygn/settings"
	_ "github.com/ushineko/fynedesygn/settings/yamlcodec" // .yml is YAML
)

// Config is the user's file: what hotaru should pay attention to, and the
// corrections that particular hardware needs.
type Config struct {
	// Scope narrows hotaru to devices whose names contain one of these,
	// matched case-insensitively. Empty means every device present.
	Scope []string `json:"scope,omitempty"`

	// Devices are corrections, matched by name. Several may match one device;
	// they apply in order, later values winning.
	Devices []DeviceRule `json:"devices,omitempty"`
}

/*
DeviceRule corrects hotaru's handling of the devices whose names contain Match.

Every field here exists because some real device misbehaves without it, and
none of them are required: a device with no rule is driven by what it reports
about itself.
*/
type DeviceRule struct {
	// Match is a case-insensitive substring of the device name. Names rather
	// than indices, because a rescanned server renumbers everything.
	Match string `json:"match"`

	// SolidModes overrides the preference order for a solid colour. The
	// default tries static before direct; a board whose addressable headers
	// only light in direct needs the reverse.
	SolidModes []string `json:"solid_modes,omitempty"`

	// NeverBlank keeps a device out of "off" while leaving it in colour
	// scenes. A keyboard that advertises no Off mode resolves off to direct
	// with black, which kills the backlight rather than dimming it.
	NeverBlank bool `json:"never_blank,omitempty"`

	// Brightness, when set, is asserted on every write to this device, for
	// hardware that otherwise keeps whatever it was last given by anything.
	Brightness *int `json:"brightness,omitempty"`

	// Reassert re-sends this device's colour on an interval. Some devices do
	// not hold what they are told: a wireless mouse restores onboard state
	// when it wakes, and nothing else puts the scene colour back.
	Reassert Duration `json:"reassert,omitempty"`

	// Segments name LED ranges: "fan-top" rather than "ring[0:11]". Naming is
	// how a person records which lights are which, once, instead of counting
	// again every time they write a scene.
	Segments map[string]Segment `json:"segments,omitempty"`
}

// Segment is a named part of a device: a whole zone, or a range within one.
type Segment struct {
	Zone string `json:"zone"`
	LEDs *LEDs  `json:"leds,omitempty"` // absent: the whole zone
}

/*
Problem is one entry hotaru could not use, and why.

Returned rather than raised so a bad rule is reported and skipped. Where names
which entry it was, in the terms the file uses.
*/
type Problem struct {
	Where string
	Why   string
}

func (p Problem) Error() string { return p.Where + ": " + p.Why }

const maxBrightness = 100

/*
Load reads the user's rules file.

A missing file is not an error: it is the supported case of a machine that has
never been configured. An unreadable or unparseable file is an error, and
nothing is written in response to either — not even moving the file aside,
which is what makes "hotaru never writes this file" true at the moment it
matters most, when a user has just mistyped their own YAML.
*/
func Load(path string) (*Config, []Problem, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the configured path
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return &Config{}, nil, nil
	case err != nil:
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(path, raw)
}

// Parse decodes a rules file that has already been read.
func Parse(path string, raw []byte) (*Config, []Problem, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return &Config{}, nil, nil
	}

	codec, err := settings.CodecFor(path)
	if err != nil {
		return nil, nil, fmt.Errorf("choose a format for %s: %w", path, err)
	}

	// One section per top-level key, decoded separately, so a broken section
	// names itself instead of failing the document.
	var doc map[string]json.RawMessage
	if err := codec.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	cfg := &Config{}
	var problems []Problem

	if section, ok := doc["scope"]; ok {
		if err := json.Unmarshal(section, &cfg.Scope); err != nil {
			problems = append(problems, Problem{"scope", "not a list of names: " + err.Error()})
			cfg.Scope = nil
		}
	}

	if section, ok := doc["devices"]; ok {
		var entries []json.RawMessage
		if err := json.Unmarshal(section, &entries); err != nil {
			problems = append(problems, Problem{"devices", "not a list: " + err.Error()})
		}
		for i, entry := range entries {
			rule, ps := decodeRule(i, entry)
			problems = append(problems, ps...)
			if rule != nil {
				cfg.Devices = append(cfg.Devices, *rule)
			}
		}
	}

	for key := range doc {
		if key != "scope" && key != "devices" {
			problems = append(problems, Problem{key, "unknown section, ignored"})
		}
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].Where < problems[j].Where })

	return cfg, problems, nil
}

func decodeRule(i int, raw json.RawMessage) (*DeviceRule, []Problem) {
	where := fmt.Sprintf("devices[%d]", i)

	var rule DeviceRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return nil, []Problem{{where, "does not decode: " + err.Error()}}
	}
	if strings.TrimSpace(rule.Match) == "" {
		return nil, []Problem{{where, "no match: a rule needs a device name to match on"}}
	}

	where = fmt.Sprintf("devices[%d] (%s)", i, rule.Match)
	var problems []Problem

	if rule.Brightness != nil && (*rule.Brightness < 0 || *rule.Brightness > maxBrightness) {
		problems = append(problems,
			Problem{where, fmt.Sprintf("brightness %d is outside 0-%d, ignored", *rule.Brightness, maxBrightness)})
		rule.Brightness = nil
	}
	if rule.Reassert < 0 {
		problems = append(problems, Problem{where, "reassert is negative, ignored"})
		rule.Reassert = 0
	}

	for name, segment := range rule.Segments {
		if why := segment.problem(); why != "" {
			problems = append(problems, Problem{where + " segment " + name, why})
			delete(rule.Segments, name)
		}
	}
	if len(rule.Segments) == 0 {
		rule.Segments = nil
	}
	return &rule, problems
}

func (s Segment) problem() string {
	if strings.TrimSpace(s.Zone) == "" {
		return "no zone: a segment names the zone its LEDs are in"
	}
	if s.LEDs == nil {
		return ""
	}
	if s.LEDs.First < 0 {
		return fmt.Sprintf("leds start at %d, which is before the first LED", s.LEDs.First)
	}
	if s.LEDs.Last < s.LEDs.First {
		return fmt.Sprintf("leds %s ends before it starts", s.LEDs)
	}
	return ""
}

/*
InScope reports whether a device is one hotaru drives.

Empty scope is every device. That default is the difference between a program
that lights a stranger's machine and one that silently does nothing on it.
*/
func (c *Config) InScope(device string) bool {
	if len(c.Scope) == 0 {
		return true
	}
	name := strings.ToLower(device)
	for _, want := range c.Scope {
		if want = strings.ToLower(strings.TrimSpace(want)); want != "" && strings.Contains(name, want) {
			return true
		}
	}
	return false
}

// RulesFor is every rule matching a device name, in file order. Several may
// match; a caller applies them in turn, later values winning.
func (c *Config) RulesFor(device string) []DeviceRule {
	name := strings.ToLower(device)
	var out []DeviceRule
	for _, rule := range c.Devices {
		if strings.Contains(name, strings.ToLower(rule.Match)) {
			out = append(out, rule)
		}
	}
	return out
}
