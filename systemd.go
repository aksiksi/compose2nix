package compose2nix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
)

// https://www.freedesktop.org/software/systemd/man/latest/systemd.syntax.html
func parseSystemdValue(v string) any {
	v = strings.TrimSpace(v)

	switch {
	case slices.Contains(systemdTrue, v):
		return true
	case slices.Contains(systemdFalse, v):
		return false
	case strings.Contains(v, ","):
		// Is this how lists are defined?
		s := strings.Split(v, ",")
		for i := range s {
			s[i] = strings.TrimSpace(s[i])
		}
		return s
	}

	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return int(i)
	}

	return v
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

func parseRestartPolicyAndSystemdLabels(service *types.ServiceConfig) (*NixContainerSystemdConfig, error) {
	c := NewNixContainerSystemdConfig()

	// https://docs.docker.com/compose/compose-file/compose-file-v2/#restart
	switch restart := service.Restart; restart {
	case "":
		c.Service["Restart"] = "no"
	case "no", "always", "on-failure":
		c.Service["Restart"] = restart
	case "unless-stopped":
		c.Service["Restart"] = "always"
	default:
		if strings.HasPrefix(restart, "on-failure") && strings.Contains(restart, ":") {
			c.Service["Restart"] = "on-failure"
			maxAttemptsString := strings.TrimSpace(strings.Split(restart, ":")[1])
			if maxAttempts, err := strconv.ParseInt(maxAttemptsString, 10, 64); err != nil {
				return nil, fmt.Errorf("failed to parse on-failure attempts: %q: %w", maxAttemptsString, err)
			} else {
				burst := int(maxAttempts)
				c.StartLimitBurst = &burst
				// Retry limit resets once per day.
				c.StartLimitIntervalSec = &defaultStartLimitIntervalSec
			}
		} else {
			return nil, fmt.Errorf("unsupported restart: %q", restart)
		}
	}

	if service.Deploy != nil {
		// The newer "deploy" config will always override the legacy "restart" config.
		// https://docs.docker.com/compose/compose-file/compose-file-v3/#restart_policy
		if restartPolicy := service.Deploy.RestartPolicy; restartPolicy != nil {
			switch condition := restartPolicy.Condition; condition {
			case "none":
				c.Service["Restart"] = "no"
			case "any":
				c.Service["Restart"] = "always"
			case "on-failure":
				c.Service["Restart"] = "on-failure"
			default:
				return nil, fmt.Errorf("unsupported condition: %q", condition)
			}
			if delay := restartPolicy.Delay; delay != nil {
				c.Service["RestartSec"] = delay.String()
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

	// Custom values provided via labels will override any explicit restart settings.
	var labelsToDrop []string
	for label, value := range service.Labels {
		if !strings.HasPrefix(label, "nixose.") {
			continue
		}
		m := systemdLabelRegexp.FindStringSubmatch(label)
		if len(m) == 0 {
			return nil, fmt.Errorf("invalid nixose label specified for service %q: %q", service.Name, label)
		}
		typ, key := m[1], m[2]
		switch typ {
		case "service":
			c.Service[key] = parseSystemdValue(value)
		case "unit":
			switch key {
			case "Requires":
				switch v := parseSystemdValue(value).(type) {
				case string:
					c.Requires = append(c.Requires, v)
				case []string:
					c.Requires = append(c.Requires, v...)
				}
			default:
				c.Unit[key] = parseSystemdValue(value)
			}
		default:
			return nil, fmt.Errorf(`invalid systemd type %q - must be "service" or "unit"`, typ)
		}
		labelsToDrop = append(labelsToDrop, label)
	}
	for _, label := range labelsToDrop {
		delete(service.Labels, label)
	}

	return c, nil
}
