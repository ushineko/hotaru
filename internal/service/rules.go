package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
RuleChange is a change to one device's rule, made from the window or the CLI
(spec 077 R3.2, R3.9). A nil field is left as it is.
*/
type RuleChange struct {
	// Follow names the device to follow, by any part of its name; empty
	// stops following.
	Follow *string
	// Pace sets the device's pace for each effect named; zero removes it.
	Pace map[string]float64
	// Lengths gives each named zone whose length the device cannot count a
	// length, in use at once; zero removes it (spec 078 R1).
	Lengths map[string]int
	// Shapes says what each named zone holds, "ring" (fans) or "line" (a
	// strip); empty removes it (spec 078 R2).
	Shapes map[string]string
	// Segments names parts of the device; nil removes the name (spec 078
	// R3).
	Segments map[string]*config.Segment
	// Fans divides each named zone into that many equal rings, each named:
	// a name already on the same lights is kept, else the zone's and the
	// fan's number; other names in the zone that divide it otherwise go,
	// a toggle stays (spec 078 R2.1).
	Fans map[string]int
}

// segmentName is a name the rules take: lowercase words and dashes.
var segmentName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// BadRule is a change the rules would not take, and why.
type BadRule struct{ Why string }

func (e *BadRule) Error() string { return e.Why }

/*
SetRule changes one device's rule in the rules file, and is in use at once.

The service writes the file, as it writes the scenes: the window holds no
file, and one route serves the window and the CLI. The rule changed is the
last one that matches the device, as the service reads them, or a new one.
The file before is kept beside it as .bak, and comments in it are not kept,
as with every write of the rules. A device given a leader takes the leader's
recorded effect now, in step with it, so it is seen following at once.
*/
func (s *Service) SetRule(ctx context.Context, device string, change RuleChange) (string, error) {
	path := s.RulesPath()
	if path == "" {
		return "", errors.New("the service has no rules file to write")
	}
	_, client := s.current()
	found, err := client.Devices(ctx)
	if err != nil {
		return "", err
	}
	target := leaderOf(found, device, "")
	if target == nil {
		return "", &NoDevice{Name: device}
	}

	cfg, _, err := config.Load(path)
	if err != nil {
		return "", err
	}
	rule := ruleToChange(cfg, target.Name)
	var leader *devices.Device
	if change.Follow != nil {
		name := strings.TrimSpace(*change.Follow)
		if name != "" {
			if leader = leaderOf(found, name, target.Name); leader == nil {
				return "", &BadRule{Why: fmt.Sprintf("no other device called %q to follow", name)}
			}
		}
		rule.Follow = name
	}
	for effect, pace := range change.Pace {
		for named := range rule.Pace {
			if strings.EqualFold(named, effect) {
				delete(rule.Pace, named)
			}
		}
		switch {
		case pace == 0:
		case pace < 0 || pace > 10:
			return "", &BadRule{Why: fmt.Sprintf("a pace above 0 and at most 10, not %g", pace)}
		default:
			if rule.Pace == nil {
				rule.Pace = map[string]float64{}
			}
			rule.Pace[effect] = pace
		}
	}
	if len(rule.Pace) == 0 {
		rule.Pace = nil
	}
	if err := divide(rule, target, change.Fans); err != nil {
		return "", err
	}
	if err := changeZones(rule, target, change); err != nil {
		return "", err
	}

	if err := writeRules(path, config.Render(cfg)); err != nil {
		return "", err
	}
	if _, err := s.Reload(); err != nil {
		return "", err
	}
	for zone, n := range change.Lengths {
		if n > 0 {
			// In use at once, as a restart would give it (spec 078 R1.2).
			_ = s.SetLength(ctx, target.Name, zone, n)
		}
	}
	if leader != nil {
		s.followNow(ctx, leader, target)
	}
	return rule.Match, nil
}

/*
changeZones applies a change's lengths, shapes and names to a rule, refusing
what the device would not take: a length for a zone it counts itself, a shape
for one whose parts it counts (the Kraken's fans), a zone it does not have,
or a name the rules do not take.
*/
func changeZones(rule *config.DeviceRule, device *devices.Device, change RuleChange) error {
	zoneOf := func(name string) (devices.Zone, error) {
		zone, ok := device.Zone(name)
		if !ok {
			return devices.Zone{}, &BadRule{Why: fmt.Sprintf("%s has no zone called %q", device.Name, name)}
		}
		return zone, nil
	}
	for name, n := range change.Lengths {
		zone, err := zoneOf(name)
		if err != nil {
			return err
		}
		if zone.Max == 0 {
			return &BadRule{Why: fmt.Sprintf("%s counts the lights on %s itself", device.Name, zone.Name)}
		}
		if n > zone.Max {
			return &BadRule{Why: fmt.Sprintf("%s takes at most %d lights, not %d", zone.Name, zone.Max, n)}
		}
		delete(rule.Lengths, zone.Name)
		if n > 0 {
			if rule.Lengths == nil {
				rule.Lengths = map[string]int{}
			}
			rule.Lengths[zone.Name] = n
		}
	}
	for name, shape := range change.Shapes {
		zone, err := zoneOf(name)
		if err != nil {
			return err
		}
		if len(zone.Parts) > 0 {
			return &BadRule{Why: fmt.Sprintf("%s counts what is on %s itself: %s", device.Name, zone.Name, partsSaid(zone.Parts))}
		}
		if shape != "" && shape != string(devices.ShapeRing) && shape != string(devices.ShapeLine) {
			return &BadRule{Why: fmt.Sprintf("a zone holds a ring or a line, not %q", shape)}
		}
		delete(rule.Shapes, zone.Name)
		if shape != "" {
			if rule.Shapes == nil {
				rule.Shapes = map[string]string{}
			}
			rule.Shapes[zone.Name] = shape
		}
	}
	if len(rule.Shapes) == 0 {
		rule.Shapes = nil
	}
	for name, segment := range change.Segments {
		if !segmentName.MatchString(name) {
			return &BadRule{Why: fmt.Sprintf("%q: a name is lowercase words and dashes", name)}
		}
		if segment == nil {
			delete(rule.Segments, name)
			continue
		}
		zone, err := zoneOf(segment.Zone)
		if err != nil {
			return err
		}
		named := config.Segment{Zone: zone.Name, LEDs: segment.LEDs}
		if named.LEDs != nil && (named.LEDs.First < 0 || named.LEDs.Last < named.LEDs.First) {
			return &BadRule{Why: fmt.Sprintf("%s: lights %d to %d", name, named.LEDs.First, named.LEDs.Last)}
		}
		if rule.Segments == nil {
			rule.Segments = map[string]config.Segment{}
		}
		rule.Segments[name] = named
	}
	return nil
}

/*
divide gives each zone in fans its rings and their names. A zone whose parts
the device counts, or whose length does not divide, is refused, with the
lengths that would.
*/
func divide(rule *config.DeviceRule, device *devices.Device, fans map[string]int) error {
	for name, n := range fans {
		zone, ok := device.Zone(name)
		if !ok {
			return &BadRule{Why: fmt.Sprintf("%s has no zone called %q", device.Name, name)}
		}
		if len(zone.Parts) > 0 {
			return &BadRule{Why: fmt.Sprintf("%s counts what is on %s itself: %s", device.Name, zone.Name, partsSaid(zone.Parts))}
		}
		if n < 1 || zone.Count%n != 0 {
			step := max(n, 1)
			low := zone.Count / step * step
			return &BadRule{Why: fmt.Sprintf("%d lights do not divide into %d fans; %d or %d do", zone.Count, n, low, low+step)}
		}
		size := zone.Count / n
		names := make([]string, n)
		for i := range n {
			for named, seg := range rule.Segments {
				if strings.EqualFold(seg.Zone, zone.Name) && seg.LEDs != nil && seg.LEDs.First == i*size && seg.LEDs.Last == (i+1)*size-1 {
					names[i] = named
				}
			}
			if names[i] == "" {
				names[i] = fmt.Sprintf("%s-fan-%d", zoneSlug(zone.Name), i+1)
			}
		}
		for named, seg := range rule.Segments {
			if strings.EqualFold(seg.Zone, zone.Name) && !slices.Contains(names, named) && !slices.Contains(rule.Toggles, named) {
				delete(rule.Segments, named)
			}
		}
		if rule.Segments == nil {
			rule.Segments = map[string]config.Segment{}
		}
		for i, named := range names {
			rule.Segments[named] = config.Segment{Zone: zone.Name, LEDs: &config.LEDs{First: i * size, Last: (i+1)*size - 1}}
		}
		if rule.Shapes == nil {
			rule.Shapes = map[string]string{}
		}
		rule.Shapes[zone.Name] = string(devices.ShapeRing)
	}
	return nil
}

// zoneSlug is a zone's name as the start of a part's: "Addressable RGB
// Header 2" is "header-2".
func zoneSlug(zone string) string {
	lowered := strings.TrimPrefix(strings.ToLower(zone), "addressable rgb ")
	var out strings.Builder
	dash := false
	for _, r := range lowered {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
			dash = false
		} else if !dash && out.Len() > 0 {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(out.String(), "-")
}

// partsSaid is a zone's parts in a few words: "three of 8".
func partsSaid(parts []int) string {
	return fmt.Sprintf("%d parts, %v lights", len(parts), parts)
}

// ruleToChange is the last rule matching the device, as the service reads
// them, or a new one for it by its full name.
func ruleToChange(cfg *config.Config, device string) *config.DeviceRule {
	lowered := strings.ToLower(device)
	for i := len(cfg.Devices) - 1; i >= 0; i-- {
		match := strings.ToLower(cfg.Devices[i].Match)
		if match != "" && strings.Contains(lowered, match) {
			return &cfg.Devices[i]
		}
	}
	cfg.Devices = append(cfg.Devices, config.DeviceRule{Match: lowered})
	return &cfg.Devices[len(cfg.Devices)-1]
}

// writeRules replaces the rules file whole, keeping the one before as .bak.
func writeRules(path, rules string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("make the rules' directory: %w", err)
	}
	if before, err := os.ReadFile(path); err == nil { //nolint:gosec // the configured path
		if err := os.WriteFile(path+".bak", before, 0o600); err != nil { //nolint:gosec // the configured path, set by the daemon, never from a request
			return fmt.Errorf("keep the rules before: %w", err)
		}
	}
	next := path + ".new"
	if err := os.WriteFile(next, []byte(rules), 0o600); err != nil {
		return fmt.Errorf("write the rules: %w", err)
	}
	if err := os.Rename(next, path); err != nil {
		return fmt.Errorf("write the rules: %w", err)
	}
	return nil
}

/*
followNow gives a new follower its leader's recorded effect, with the leader,
so the two start in step. A leader recorded with no effect, or with none,
leaves the follower as it is until the leader is given one.
*/
func (s *Service) followNow(ctx context.Context, leader, follower *devices.Device) {
	kept, ok := s.Desired().Devices[leader.Name]
	if !ok || kept.Mode == "" || kept.Mode == canvasStill {
		return
	}
	effect := scenes.Effect{Mode: kept.Mode, Speed: kept.Speed}
	effect.SetPalette(kept.Named())
	if !effect.Named() {
		return
	}
	_, _ = s.Apply(ctx, Request{
		Devices: []string{leader.Name, follower.Name},
		Effects: map[string]scenes.Effect{leader.Name: effect},
	})
}
