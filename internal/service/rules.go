package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

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

	if err := writeRules(path, config.Render(cfg)); err != nil {
		return "", err
	}
	if _, err := s.Reload(); err != nil {
		return "", err
	}
	if leader != nil {
		s.followNow(ctx, leader, target)
	}
	return rule.Match, nil
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
