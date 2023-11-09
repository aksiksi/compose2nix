package compose2nix

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"golang.org/x/exp/maps"
)

var (
	defaultStartLimitIntervalSec = int((24 * time.Hour).Seconds())
)

// Examples:
// nixose.systemd.service.RuntimeMaxSec=100
// nixose.systemd.unit.StartLimitBurst=10
var systemdLabelRegexp regexp.Regexp = *regexp.MustCompile(`nixose\.systemd\.(service|unit)\.(\w+)`)

func composeEnvironmentToMap(env types.MappingWithEquals) map[string]string {
	m := make(map[string]string)
	for k, v := range env {
		// Skip empty env variables.
		if v == nil {
			continue
		}
		m[k] = *v
	}
	return m
}

func portConfigsToPortStrings(portConfigs []types.ServicePortConfig) []string {
	var ports []string
	for _, c := range portConfigs {
		var ipAndPorts []string
		if c.HostIP != "" {
			ipAndPorts = append(ipAndPorts, c.HostIP)
		}
		if c.Published != "" {
			ipAndPorts = append(ipAndPorts, c.Published)
		}
		if c.Target != 0 {
			ipAndPorts = append(ipAndPorts, fmt.Sprintf("%d", c.Target))
		}
		port := strings.Join(ipAndPorts, ":")
		if c.Protocol != "" {
			port = fmt.Sprintf("%s/%s", port, c.Protocol)
		}
		ports = append(ports, port)
	}
	return ports
}

func parseRestartPolicyAndSystemdLabels(service *types.ServiceConfig) (*NixContainerSystemdConfig, error) {
	p := &NixContainerSystemdConfig{
		Service: make(map[string]any),
		Unit:    make(map[string]any),
	}

	// https://docs.docker.com/compose/compose-file/compose-file-v2/#restart
	switch restart := service.Restart; restart {
	case "":
		p.Service["Restart"] = "no"
	case "no", "always", "on-failure":
		p.Service["Restart"] = restart
	case "unless-stopped":
		p.Service["Restart"] = "always"
	default:
		if strings.HasPrefix(restart, "on-failure") && strings.Contains(restart, ":") {
			p.Service["Restart"] = "on-failure"
			maxAttemptsString := strings.TrimSpace(strings.Split(restart, ":")[1])
			if maxAttempts, err := strconv.ParseInt(maxAttemptsString, 10, 64); err != nil {
				return nil, fmt.Errorf("failed to parse on-failure attempts: %q: %w", maxAttemptsString, err)
			} else {
				burst := int(maxAttempts)
				p.StartLimitBurst = &burst
				// Retry limit resets once per day.
				p.StartLimitIntervalSec = &defaultStartLimitIntervalSec
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
				p.Service["Restart"] = "no"
			case "any":
				p.Service["Restart"] = "always"
			case "on-failure":
				p.Service["Restart"] = "on-failure"
			default:
				return nil, fmt.Errorf("unsupported condition: %q", condition)
			}
			if delay := restartPolicy.Delay; delay != nil {
				p.Service["RestartSec"] = delay.String()
			}
			if maxAttempts := restartPolicy.MaxAttempts; maxAttempts != nil {
				v := int(*maxAttempts)
				p.StartLimitBurst = &v
			}
			if window := restartPolicy.Window; window != nil {
				windowSecs := int(time.Duration(*window).Seconds())
				p.StartLimitIntervalSec = &windowSecs
			} else if p.StartLimitBurst != nil {
				// Retry limit resets once per day by default.
				p.StartLimitIntervalSec = &defaultStartLimitIntervalSec
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
			p.Service[key] = parseSystemdValue(value)
		case "unit":
			p.Unit[key] = parseSystemdValue(value)
		default:
			return nil, fmt.Errorf(`invalid systemd type %q - must be "service" or "unit"`, typ)
		}
		labelsToDrop = append(labelsToDrop, label)
	}
	for _, label := range labelsToDrop {
		delete(service.Labels, label)
	}

	return p, nil
}

type Generator struct {
	Project             *Project
	Runtime             ContainerRuntime
	Inputs              []string
	EnvFiles            []string
	ServiceInclude      *regexp.Regexp
	AutoStart           bool
	EnvFilesOnly        bool
	UseComposeLogDriver bool
}

func (g *Generator) Run(ctx context.Context) (*NixContainerConfig, error) {
	env, err := ReadEnvFiles(g.EnvFiles, !g.EnvFilesOnly)
	if err != nil {
		return nil, err
	}
	composeProject, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(g.Inputs),
		Environment: types.NewMapping(env),
	})
	if err != nil {
		return nil, err
	}

	containers, err := g.buildNixContainers(composeProject)
	if err != nil {
		return nil, err
	}
	networks := g.buildNixNetworks(composeProject, containers)
	volumes := g.buildNixVolumes(composeProject, containers)

	// Post-process any Compose settings that require the full state.
	g.postProcessContainers(containers)

	return &NixContainerConfig{
		Project:    g.Project,
		Runtime:    g.Runtime,
		Containers: containers,
		Networks:   networks,
		Volumes:    volumes,
	}, nil
}

func (g *Generator) postProcessContainers(containers []*NixContainer) {
	serviceToContainer := make(map[string]*NixContainer)
	for _, c := range containers {
		serviceToContainer[c.service.Name] = c
	}

	for _, c := range containers {
		if networkMode := c.service.NetworkMode; strings.HasPrefix(networkMode, "service:") {
			targetService := strings.Split(networkMode, ":")[1]
			targetContainerName := serviceToContainer[targetService].Name
			c.ExtraOptions = append(c.ExtraOptions, "--network=container:"+targetContainerName)
			c.DependsOn = append(c.DependsOn, targetContainerName)
		}

		dependsOn := c.service.GetDependencies()
		for i, service := range dependsOn {
			dependsOn[i] = serviceToContainer[service].Name
		}
		c.DependsOn = dependsOn

		// Drop the reference to the service at the end of post-processing. This allows GC to
		// kick in and free the service allocation.
		c.service = nil

		// Sort slices now that we're done with the container.
		slices.Sort(c.DependsOn)
		slices.Sort(c.ExtraOptions)
	}
}

func (g *Generator) buildNixContainer(service types.ServiceConfig) (*NixContainer, error) {
	var name string
	if service.ContainerName != "" {
		name = service.ContainerName
	} else {
		name = g.Project.With(service.Name)
	}

	systemdConfig, err := parseRestartPolicyAndSystemdLabels(&service)
	if err != nil {
		return nil, err
	}

	c := &NixContainer{
		Runtime:       g.Runtime,
		Name:          name,
		Image:         service.Image,
		Labels:        service.Labels,
		Ports:         portConfigsToPortStrings(service.Ports),
		User:          service.User,
		Volumes:       make(map[string]string),
		Networks:      maps.Keys(service.Networks),
		SystemdConfig: systemdConfig,
		AutoStart:     g.AutoStart,
		LogDriver:     "journald", // This is the NixOS default
		service:       &service,
	}
	slices.Sort(c.Networks)

	if !g.EnvFilesOnly {
		c.Environment = composeEnvironmentToMap(service.Environment)
	} else {
		c.EnvFiles = g.EnvFiles
	}

	for _, v := range service.Volumes {
		c.Volumes[v.Source] = v.String()
	}

	for _, name := range c.Networks {
		networkName := g.Project.With(name)
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--network=%s", networkName))
	}

	// https://docs.docker.com/compose/compose-file/compose-file-v3/#network_mode
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-mode-net
	switch networkMode := strings.TrimSpace(service.NetworkMode); {
	case networkMode == "host":
		c.ExtraOptions = append(c.ExtraOptions, "--network=host")
	case strings.HasPrefix(networkMode, "container:"):
		// container:[name] mode is supported by both Docker and Podman.
		c.ExtraOptions = append(c.ExtraOptions, networkMode)
		containerName := strings.TrimSpace(strings.Split(networkMode, ":")[1])
		c.DependsOn = append(c.DependsOn, containerName)
	}

	// Allow other containers to use service name as an alias.
	c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--network-alias=%s", service.Name))

	for _, ip := range service.DNS {
		c.ExtraOptions = append(c.ExtraOptions, "--dns="+ip)
	}

	// https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities
	if service.Privileged {
		c.ExtraOptions = append(c.ExtraOptions, "--privileged")
	}
	for _, cap := range service.CapAdd {
		c.ExtraOptions = append(c.ExtraOptions, "--cap-add="+cap)
	}
	for _, cap := range service.CapDrop {
		c.ExtraOptions = append(c.ExtraOptions, "--cap-drop="+cap)
	}
	for _, device := range service.Devices {
		c.ExtraOptions = append(c.ExtraOptions, "--device="+device)
	}

	// Compose defaults to "json-file", so we'll treat _any_ "json-file" setting as a default.
	// Users can override this behavior via CLI.
	//
	// https://docs.docker.com/config/containers/logging/configure/
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#log-driver-driver
	if service.LogDriver != "" {
		if service.LogDriver != "json-file" || g.UseComposeLogDriver {
			c.LogDriver = service.LogDriver
		}
		// Log options are always passed through.
		c.ExtraOptions = append(c.ExtraOptions, mapToRepeatedKeyValFlag("--log-opt", service.LogOpt)...)
	}
	// New logging setting always overrides the legacy setting.
	// https://docs.docker.com/compose/compose-file/compose-file-v3/#logging
	if logging := service.Logging; logging != nil {
		if logging.Driver != "json-file" || g.UseComposeLogDriver {
			c.LogDriver = logging.Driver
		}
		// Log options are always passed through.
		c.ExtraOptions = append(c.ExtraOptions, mapToRepeatedKeyValFlag("--log-opt", logging.Options)...)
	}

	return c, nil
}

func (g *Generator) buildNixContainers(composeProject *types.Project) ([]*NixContainer, error) {
	var containers []*NixContainer
	for _, s := range composeProject.Services {
		if g.ServiceInclude != nil && !g.ServiceInclude.MatchString(s.Name) {
			log.Printf("Skipping service %q due to include regex %q", s.Name, g.ServiceInclude.String())
			continue
		}
		c, err := g.buildNixContainer(s)
		if err != nil {
			return nil, fmt.Errorf("failed to build container for service %q: %w", s.Name, err)
		}
		containers = append(containers, c)
	}
	slices.SortFunc(containers, func(c1, c2 *NixContainer) int {
		return cmp.Compare(c1.Name, c2.Name)
	})
	return containers, nil
}

func (g *Generator) containerNameToService(name string) string {
	return fmt.Sprintf("%s-%s.service", g.Runtime, name)
}

func (g *Generator) buildNixNetworks(composeProject *types.Project, containers []*NixContainer) []*NixNetwork {
	var networks []*NixNetwork
	for name, network := range composeProject.Networks {
		n := &NixNetwork{
			Runtime: g.Runtime,
			Name:    g.Project.With(name),
			Labels:  network.Labels,
		}
		// Keep track of all container services that are in this network.
		for _, c := range containers {
			if slices.Contains(c.Networks, name) {
				n.Containers = append(n.Containers, g.containerNameToService(c.Name))
			}
		}
		networks = append(networks, n)
	}
	slices.SortFunc(networks, func(n1, n2 *NixNetwork) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return networks
}

func (g *Generator) buildNixVolumes(composeProject *types.Project, containers []*NixContainer) []*NixVolume {
	var volumes []*NixVolume
	for name, volume := range composeProject.Volumes {
		v := &NixVolume{
			Runtime:    g.Runtime,
			Name:       name,
			Driver:     volume.Driver,
			DriverOpts: volume.DriverOpts,
		}

		// FIXME(aksiksi): Podman does not properly handle NFS if the volume
		// is a regular mount. So, we can just "patch" each container's volume
		// mapping to use a direct bind mount instead of a volume and then skip
		// creation of the volume entirely.
		if g.Runtime == ContainerRuntimePodman && v.Driver == "" {
			bindPath := v.DriverOpts["device"]
			if bindPath == "" {
				log.Fatalf("Volume %q has no device set", name)
			}
			for _, c := range containers {
				if volumeString, ok := c.Volumes[name]; ok {
					volumeString = strings.TrimPrefix(volumeString, name)
					c.Volumes[name] = bindPath + volumeString
				}
			}
			continue
		}

		// Keep track of all container services that use this named volume.
		for _, c := range containers {
			if _, ok := c.Volumes[name]; ok {
				v.Containers = append(v.Containers, g.containerNameToService(c.Name))
			}
		}
		volumes = append(volumes, v)
	}
	slices.SortFunc(volumes, func(n1, n2 *NixVolume) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return volumes
}
