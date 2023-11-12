package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
)

const (
	composeLabelPrefix = "compose2nix"
)

var (
	defaultStartLimitIntervalSec = int((24 * time.Hour).Seconds())

	// We purposefully do not support 0/1 for false/true.
	systemdTrue  = []string{"true", "yes", "on"}
	systemdFalse = []string{"false", "no", "off"}

	// Examples:
	// compose2nix.systemd.service.RuntimeMaxSec=100
	// compose2nix.systemd.unit.StartLimitBurst=10
	systemdLabelRegexp = regexp.MustCompile(fmt.Sprintf(`%s\.systemd\.(service|unit)\.(\w+)`, composeLabelPrefix))
)

// https://www.freedesktop.org/software/systemd/man/latest/systemd.syntax.html
func parseSystemdValue(v string) any {
	v = strings.TrimSpace(v)

	// Number
	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return int(i)
	}

	// Boolean
	switch {
	case slices.Contains(systemdTrue, v):
		return true
	case slices.Contains(systemdFalse, v):
		return false
	}

	// String
	return v
}

// TODO(aksiksi): Add support for repeated keys.
type ServiceConfig struct {
	// Map for generic options.
	Options map[string]any
}

func (s *ServiceConfig) Set(key string, value any) {
	if s.Options == nil {
		s.Options = map[string]any{}
	}
	s.Options[key] = value
}

// TODO(aksiksi): Add support for repeated keys.
type UnitConfig struct {
	After    []string
	Requires []string
	// Map for generic options.
	Options map[string]any
}

func (u *UnitConfig) Set(key string, value any) {
	if u.Options == nil {
		u.Options = map[string]any{}
	}
	switch key {
	case "After":
		u.After = append(u.After, value.(string))
	case "Requires":
		u.Requires = append(u.Requires, value.(string))
	default:
		u.Options[key] = value
	}
}

type SystemdUnit struct {
	Unit        string `json:"unit"`
	Active      string `json:"active"`
	Description string `json:"description"`
}

type SystemdProvider interface {
	FindMountForPath(path string) (string, error)
}

type SystemdCLI struct{}

func (s *SystemdCLI) FindMountForPath(path string) (string, error) {
	cmd := exec.Command("systemctl", "list-units", "--type=mount", "--output=json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(`failed to run "systemctl list-units": %w`, err)
	}
	dec := json.NewDecoder(bytes.NewReader(output))
	units := []SystemdUnit{}
	if err := dec.Decode(&units); err != nil {
		return "", fmt.Errorf("failed to decode systemctl command output: %w", err)
	}
	for _, u := range units {
		if u.Active != "active" {
			continue
		}
		// If the systemd mount path is a prefix of the given path, our path belongs to
		// this mount.
		if strings.HasPrefix(path, u.Description) {
			return u.Unit, nil
		}
	}
	return "", nil
}

func (c *NixContainerSystemdConfig) ParseRestartPolicy(service *types.ServiceConfig) error {
	// https://docs.docker.com/compose/compose-file/compose-file-v2/#restart
	switch restart := service.Restart; restart {
	case "":
		c.Service.Set("Restart", "no")
	case "no", "always", "on-failure":
		c.Service.Set("Restart", restart)
	case "unless-stopped":
		c.Service.Set("Restart", "always")
	default:
		if strings.HasPrefix(restart, "on-failure") && strings.Contains(restart, ":") {
			c.Service.Set("Restart", "on-failure")
			maxAttemptsString := strings.TrimSpace(strings.Split(restart, ":")[1])
			if maxAttempts, err := strconv.ParseInt(maxAttemptsString, 10, 64); err != nil {
				return fmt.Errorf("failed to parse on-failure attempts: %q: %w", maxAttemptsString, err)
			} else {
				burst := int(maxAttempts)
				c.StartLimitBurst = &burst
				// Retry limit resets once per day.
				c.StartLimitIntervalSec = &defaultStartLimitIntervalSec
			}
		} else {
			return fmt.Errorf("unsupported restart: %q", restart)
		}
	}

	if service.Deploy != nil {
		// The newer "deploy" config will always override the legacy "restart" config.
		// https://docs.docker.com/compose/compose-file/compose-file-v3/#restart_policy
		if restartPolicy := service.Deploy.RestartPolicy; restartPolicy != nil {
			switch condition := restartPolicy.Condition; condition {
			case "none":
				c.Service.Set("Restart", "no")
			case "any":
				c.Service.Set("Restart", "always")
			case "on-failure":
				c.Service.Set("Restart", condition)
			default:
				return fmt.Errorf("unsupported condition: %q", condition)
			}
			if delay := restartPolicy.Delay; delay != nil {
				c.Service.Set("RestartSec", delay.String())
			}
			if maxAttempts := restartPolicy.MaxAttempts; maxAttempts != nil {
				v := int(*maxAttempts)
				c.StartLimitBurst = &v
			}
			if window := restartPolicy.Window; window != nil {
				windowSecs := int(time.Duration(*window).Seconds())
				c.StartLimitIntervalSec = &windowSecs
			} else if c.StartLimitBurst != nil {
				// Retry limit resets once per day by default.
				c.StartLimitIntervalSec = &defaultStartLimitIntervalSec
			}
		}
	}

	return nil
}

func (c *NixContainerSystemdConfig) ParseSystemdLabels(service *types.ServiceConfig) error {
	var labelsToDrop []string
	for label, value := range service.Labels {
		if !strings.HasPrefix(label, composeLabelPrefix) {
			continue
		}
		m := systemdLabelRegexp.FindStringSubmatch(label)
		if len(m) == 0 {
			return fmt.Errorf("invalid nixose label specified for service %q: %q", service.Name, label)
		}
		typ, key := m[1], m[2]
		switch typ {
		case "service":
			c.Service.Set(key, parseSystemdValue(value))
		case "unit":
			c.Unit.Set(key, parseSystemdValue(value))
		default:
			return fmt.Errorf(`invalid systemd type %q - must be "service" or "unit"`, typ)
		}
		labelsToDrop = append(labelsToDrop, label)
	}
	for _, label := range labelsToDrop {
		delete(service.Labels, label)
	}
	return nil
}
