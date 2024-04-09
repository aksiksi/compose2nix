package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
)

const (
	composeLabelPrefix = "compose2nix"

	// https://www.freedesktop.org/software/systemd/man/latest/systemd-system.conf.html#DefaultTimeoutStartSec=
	defaultSystemdStopTimeout = 90 * time.Second
)

var (
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
	// Remove all quotes from the string.
	v = strings.ReplaceAll(v, `"`, "")
	v = strings.ReplaceAll(v, `'`, "")

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
	After             []string
	Requires          []string
	PartOf            []string
	UpheldBy          []string
	WantedBy          []string
	RequiresMountsFor []string
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
	case "PartOf":
		u.PartOf = append(u.PartOf, value.(string))
	case "UpheldBy":
		u.UpheldBy = append(u.UpheldBy, value.(string))
	case "WantedBy":
		u.WantedBy = append(u.WantedBy, value.(string))
	case "RequiresMountsFor":
		u.RequiresMountsFor = append(u.RequiresMountsFor, value.(string))
	default:
		u.Options[key] = value
	}
}

func (c *NixContainerSystemdConfig) ParseRestartPolicy(service *types.ServiceConfig, runtime ContainerRuntime) error {
	indefiniteRestart := false

	// https://docs.docker.com/compose/compose-file/compose-file-v2/#restart
	switch restart := service.Restart; restart {
	case "", "no":
		// Need to use string literal here to avoid parsing as a boolean.
		c.Service.Set("Restart", `"no"`)
	case "always", "on-failure":
		// Both of these match the systemd restart options.
		c.Service.Set("Restart", restart)
		indefiniteRestart = true
	case "unless-stopped":
		// We don't have an equivalent in systemd. Podman does the same thing.
		c.Service.Set("Restart", "always")
		indefiniteRestart = true
	default:
		if strings.HasPrefix(restart, "on-failure:") && len(strings.Split(restart, ":")) == 2 {
			c.Service.Set("Restart", "on-failure")
			maxAttemptsString := strings.TrimSpace(strings.Split(restart, ":")[1])
			maxAttempts, err := strconv.ParseInt(maxAttemptsString, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse on-failure attempts: %q: %w", maxAttemptsString, err)
			}
			burst := int(maxAttempts)
			c.StartLimitBurst = &burst
			c.Unit.Set("StartLimitIntervalSec", "infinity")
		} else {
			return fmt.Errorf("unsupported restart: %q", restart)
		}
	}

	// The newer "deploy" config will always override the legacy "restart" config.
	// https://docs.docker.com/compose/compose-file/compose-file-v3/#restart_policy
	if deploy := service.Deploy; deploy != nil && deploy.RestartPolicy != nil {
		switch condition := deploy.RestartPolicy.Condition; condition {
		case "none":
			// Need to use string literal here to avoid parsing as a boolean.
			c.Service.Set("Restart", `"no"`)
		case "", "any":
			// If unset, defaults to "any".
			c.Service.Set("Restart", "always")
			indefiniteRestart = true
		case "on-failure":
			c.Service.Set("Restart", "on-failure")
		default:
			return fmt.Errorf("unsupported condition: %q", condition)
		}
		if delay := deploy.RestartPolicy.Delay; delay != nil {
			c.Service.Set("RestartSec", delay.String())
		} else {
			c.Service.Set("RestartSec", 0)
		}
		if maxAttempts := deploy.RestartPolicy.MaxAttempts; maxAttempts != nil {
			v := int(*maxAttempts)
			c.StartLimitBurst = &v
		}
		if window := deploy.RestartPolicy.Window; window != nil {
			// TODO(aksiksi): Investigate if StartLimitIntervalSec lines up with Compose's "window".
			windowSecs := int(time.Duration(*window).Seconds())
			c.Unit.Set("StartLimitIntervalSec", windowSecs)
		} else if c.StartLimitBurst != nil {
			c.Unit.Set("StartLimitIntervalSec", "infinity")
		}
	}

	if indefiniteRestart && runtime == ContainerRuntimeDocker {
		// This simulates the default behavior of Docker. Basically, Docker will restart
		// the container with a sleep period of 100ms. This sleep period is doubled until a
		// maximum of 1 minute.
		// See: https://docs.docker.com/reference/cli/docker/container/run/#restart
		c.Service.Set("RestartSec", "100ms")
		c.Service.Set("RestartSteps", 9) // 2^(9 attempts) = 512 (* 100ms) ~= 1 minute
		c.Service.Set("RestartMaxDelaySec", "1m")
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
			return fmt.Errorf("invalid compose2nix label specified for service %q: %q", service.Name, label)
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
