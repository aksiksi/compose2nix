package nixose

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

type Generator struct {
	Project      *Project
	Runtime      ContainerRuntime
	Paths        []string
	EnvFiles     []string
	AutoStart    bool
	EnvFilesOnly bool

	composeProject *types.Project
}

func (g *Generator) Run(ctx context.Context) (*NixContainerConfig, error) {
	env, err := ReadEnvFiles(g.EnvFiles, !g.EnvFilesOnly)
	if err != nil {
		return nil, err
	}
	composeProject, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(g.Paths),
		Environment: types.NewMapping(env),
	})
	if err != nil {
		return nil, err
	}
	g.composeProject = composeProject

	containers := g.buildNixContainers()
	networks := g.buildNixNetworks(containers)
	volumes := g.buildNixVolumes(containers)

	return &NixContainerConfig{
		Project:    g.Project,
		Runtime:    g.Runtime,
		Containers: containers,
		Networks:   networks,
		Volumes:    volumes,
	}, nil
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
				v := int(maxAttempts)
				p.StartLimitBurst = &v
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

func (g *Generator) buildNixContainer(service types.ServiceConfig) NixContainer {
	dependsOn := service.GetDependencies()
	if g.Project != nil {
		for i := range dependsOn {
			dependsOn[i] = g.Project.With(dependsOn[i])
		}
	}

	var name string
	if service.ContainerName != "" {
		name = service.ContainerName
	} else {
		// TODO(aksiksi): We should try to use the same convention as Docker Compose
		// when container_name is not set.
		// See: https://github.com/docker/compose/issues/6316
		name = service.Name
	}

	systemdConfig, err := parseRestartPolicyAndSystemdLabels(&service)
	if err != nil {
		// TODO(aksiksi): Return error here instead of panicing.
		panic(err)
	}

	c := NixContainer{
		Project:       g.Project,
		Runtime:       g.Runtime,
		Name:          name,
		Image:         service.Image,
		Labels:        service.Labels,
		Ports:         portConfigsToPortStrings(service.Ports),
		User:          service.User,
		Volumes:       make(map[string]string),
		Networks:      maps.Keys(service.Networks),
		SystemdConfig: systemdConfig,
		DependsOn:     dependsOn,
		AutoStart:     g.AutoStart,
	}
	slices.Sort(c.Networks)

	if !g.EnvFilesOnly {
		c.Environment = composeEnvironmentToMap(service.Environment)
	} else {
		c.EnvFiles = g.EnvFiles
	}

	for _, name := range c.Networks {
		networkName := g.Project.With(name)
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--network=%s", networkName))
		// Allow other containers to use bare container name as an alias even when a project is set.
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--network-alias=%s", service.Name))
	}

	// TODO(aksiksi): Handle the service's "network_mode"
	// We can only parse the network mode at this point if it points to host or container.
	// If it points to a service, we'll need to do a scan when we've finished parsing all
	// containers.
	// Compose: https://docs.docker.com/compose/compose-file/compose-file-v3/#network_mode
	// Podman: https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-mode-net

	for _, v := range service.Volumes {
		c.Volumes[v.Source] = v.String()
	}

	return c
}

func (g *Generator) buildNixContainers() []NixContainer {
	var containers []NixContainer
	for _, s := range g.composeProject.Services {
		containers = append(containers, g.buildNixContainer(s))
	}
	slices.SortFunc(containers, func(c1, c2 NixContainer) int {
		return cmp.Compare(c1.Name, c2.Name)
	})
	return containers
}

func (g *Generator) buildNixNetworks(containers []NixContainer) []NixNetwork {
	var networks []NixNetwork
	for name, network := range g.composeProject.Networks {
		n := NixNetwork{
			Project: g.Project,
			Runtime: g.Runtime,
			Name:    name,
			Labels:  network.Labels,
		}
		// Keep track of all containers that are in this network.
		for _, c := range containers {
			if slices.Contains(c.Networks, name) {
				n.Containers = append(n.Containers, fmt.Sprintf("%s-%s", g.Runtime, g.Project.With(c.Name)))
			}
		}
		networks = append(networks, n)
	}
	slices.SortFunc(networks, func(n1, n2 NixNetwork) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return networks
}

func (g *Generator) buildNixVolumes(containers []NixContainer) []NixVolume {
	var volumes []NixVolume
	for name, volume := range g.composeProject.Volumes {
		v := NixVolume{
			Project:    g.Project,
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

		// Keep track of all containers that use this named volume.
		for _, c := range containers {
			if _, ok := c.Volumes[name]; ok {
				v.Containers = append(v.Containers, fmt.Sprintf("%s-%s", g.Runtime, g.Project.With(c.Name)))
			}
		}
		volumes = append(volumes, v)
	}
	slices.SortFunc(volumes, func(n1, n2 NixVolume) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return volumes
}
