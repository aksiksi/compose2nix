package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

func composeEnvironmentToMap(env types.MappingWithEquals) map[string]string {
	m := map[string]string{}
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
	Project                *Project
	Runtime                ContainerRuntime
	Inputs                 []string
	EnvFiles               []string
	IncludeEnvFiles        bool
	EnvFilesOnly           bool
	ServiceInclude         *regexp.Regexp
	AutoStart              bool
	UseComposeLogDriver    bool
	GenerateUnusedResoures bool
	SystemdProvider        SystemdProvider
	CheckSystemdMounts     bool
	RemoveVolumes          bool
	NoCreateRootTarget     bool
	WriteHeader            bool

	serviceToContainerName map[string]string
}

func (g *Generator) Run(ctx context.Context) (*NixContainerConfig, error) {
	env, err := ReadEnvFiles(g.EnvFiles, !g.EnvFilesOnly)
	if err != nil {
		return nil, err
	}

	var opts []func(*loader.Options)
	if g.Project != nil {
		opts = append(opts, func(o *loader.Options) {
			o.SetProjectName(g.Project.Name, true)
		})
	}

	// Workaround for https://github.com/compose-spec/compose-go/issues/489.
	var configFiles []types.ConfigFile
	for _, input := range g.Inputs {
		content, err := os.ReadFile(input)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %q: %w", input, err)
		}
		configFiles = append(configFiles, types.ConfigFile{
			Filename: input,
			Content:  content,
		})
	}

	composeProject, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: configFiles,
		Environment: types.NewMapping(env),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Compose project: %w", err)
	}

	if composeProject.Name != "" {
		// Always override the project we have set. It could have been normalized or, more commonly,
		// pulled from one of the Compose files' top-level "name" setting.
		g.Project = NewProject(composeProject.Name)
	}

	// Construct a map of service to container name.
	g.serviceToContainerName = map[string]string{}
	for _, service := range composeProject.Services {
		var name string
		if service.ContainerName != "" {
			name = service.ContainerName
		} else {
			name = g.Project.With(service.Name)
		}
		g.serviceToContainerName[service.Name] = name
	}

	containers, err := g.buildNixContainers(composeProject)
	if err != nil {
		return nil, err
	}
	networks := g.buildNixNetworks(composeProject, containers)
	volumes := g.buildNixVolumes(composeProject, containers)

	// Post-process any Compose settings that require the full state.
	g.postProcessContainers(containers, volumes)

	var version string
	if g.WriteHeader {
		version = appVersion
	}

	return &NixContainerConfig{
		Version:          version,
		Project:          g.Project,
		Runtime:          g.Runtime,
		Containers:       containers,
		Networks:         networks,
		Volumes:          volumes,
		CreateRootTarget: !g.NoCreateRootTarget,
		AutoStart:        g.AutoStart,
	}, nil
}

func (g *Generator) postProcessContainers(containers []*NixContainer, volumes []*NixVolume) error {
	for _, c := range containers {
		var serviceName string
		for s, containerName := range g.serviceToContainerName {
			if containerName == c.Name {
				serviceName = s
				break
			}
		}

		// Add systemd dependencies on volume(s).
		for _, v := range volumes {
			if _, ok := c.Volumes[v.Name]; !ok {
				continue
			}
			c.SystemdConfig.Unit.After = append(c.SystemdConfig.Unit.After, g.volumeNameToService(v.Name))
			c.SystemdConfig.Unit.Requires = append(c.SystemdConfig.Unit.Requires, g.volumeNameToService(v.Name))
		}

		// Add dependencies on systemd mounts for volumes used by this container, if any.
		if g.CheckSystemdMounts {
			if g.SystemdProvider == nil {
				return fmt.Errorf("no systemd provider specified")
			}
			for name := range c.Volumes {
				var path string
				// Check to see if this is a named volume.
				for _, v := range volumes {
					if v.Name == name {
						path = v.Path()
						break
					}
				}
				if path == "" {
					// This is a bind mount.
					path = name
				}
				if !strings.HasPrefix(path, "/") {
					log.Printf("Volume path %q is not absolute; skipping systemd mount dependency for service %q", path, serviceName)
					continue
				}
				unit, err := g.SystemdProvider.FindMountForPath(path)
				if err != nil {
					return err
				}
				if unit == "" {
					// No unit exists for this path.
					continue
				} else if !slices.Contains(c.SystemdConfig.Unit.Requires, unit) {
					c.SystemdConfig.Unit.After = append(c.SystemdConfig.Unit.After, unit)
					c.SystemdConfig.Unit.Requires = append(c.SystemdConfig.Unit.Requires, unit)
				}
			}
		}
	}

	return nil
}

func (g *Generator) buildNixContainer(service types.ServiceConfig) (*NixContainer, error) {
	name := g.serviceToContainerName[service.Name]

	systemdConfig := NewNixContainerSystemdConfig()
	if err := systemdConfig.ParseRestartPolicy(&service); err != nil {
		return nil, err
	}
	// Restart configs provided via labels will always override Compose restart settings.
	if err := systemdConfig.ParseSystemdLabels(&service); err != nil {
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
		SystemdConfig: systemdConfig,
		LogDriver:     "journald", // This is the NixOS default
	}

	if g.IncludeEnvFiles {
		c.EnvFiles = g.EnvFiles
	}
	if !g.EnvFilesOnly {
		c.Environment = composeEnvironmentToMap(service.Environment)
	}

	for _, v := range service.Volumes {
		c.Volumes[v.Source] = v.String()
	}

	// Figure out explicit dependencies for this container.
	for _, s := range service.GetDependencies() {
		targetContainerName, ok := g.serviceToContainerName[s]
		if !ok {
			return nil, fmt.Errorf("service %q depends on non-existent service %q", service.Name, s)
		}
		if !slices.Contains(c.DependsOn, targetContainerName) {
			c.DependsOn = append(c.DependsOn, targetContainerName)
		}
	}

	// https://docs.docker.com/network/#ip-address-and-hostname
	if g.Runtime == ContainerRuntimeDocker && len(service.Networks) > 1 {
		return nil, fmt.Errorf("only a single network is supported for each Docker service")
	}

	// If the container is connected to a network, it's counted as being in a bridge network.
	// We need to know this to be able to determine if we can configure a network alias.
	inBridgeNetwork := len(service.Networks) > 0

	for name, net := range service.Networks {
		networkName := g.Project.With(name)
		c.Networks = append(c.Networks, networkName)

		// Network-scoped aliases.
		var aliases []string
		if net != nil {
			aliases = append(aliases, net.Aliases...)
		}

		networkFlag := fmt.Sprintf("--network=%s", networkName)
		switch g.Runtime {
		case ContainerRuntimeDocker:
			// Aliases are scoped to this (single) network.
			for _, alias := range aliases {
				c.ExtraOptions = append(c.ExtraOptions, "--network-alias="+alias)
			}
		case ContainerRuntimePodman:
			// Aliases are scoped to the current network.
			// https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-mode-net
			var networkOpts []string
			for _, alias := range aliases {
				networkOpts = append(networkOpts, "alias="+alias)
			}
			if len(networkOpts) > 0 {
				networkFlag += fmt.Sprintf(":%s", strings.Join(networkOpts, ","))
			}
		}

		c.ExtraOptions = append(c.ExtraOptions, networkFlag)
	}

	// https://docs.docker.com/compose/compose-file/05-services/#network_mode
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-mode-net
	if networkMode := strings.TrimSpace(service.NetworkMode); networkMode != "" {
		switch {
		case networkMode == "none":
			c.ExtraOptions = append(c.ExtraOptions, "--network=none")
		case networkMode == "host":
			c.ExtraOptions = append(c.ExtraOptions, "--network=host")
		// https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-mode-net
		case strings.HasPrefix(networkMode, "bridge") && g.Runtime == ContainerRuntimePodman:
			// TODO(aksiksi): Can we even do anything for Docker?
			c.ExtraOptions = append(c.ExtraOptions, "--network="+networkMode)
			inBridgeNetwork = true
		case strings.HasPrefix(networkMode, "service:"):
			// Convert the Compose "service" network mode to a "container" network mode.
			targetService := strings.Split(networkMode, ":")[1]
			targetContainerName, ok := g.serviceToContainerName[targetService]
			if !ok {
				return nil, fmt.Errorf("network_mode for service %q refers to a non-existent service %q", service.Name, targetService)
			}
			c.ExtraOptions = append(c.ExtraOptions, "--network=container:"+targetContainerName)
			if !slices.Contains(c.DependsOn, targetContainerName) {
				c.DependsOn = append(c.DependsOn, targetContainerName)
			}
		case strings.HasPrefix(networkMode, "container:"):
			// container:[name] mode is supported by both Docker and Podman.
			// This container could be external, so we can't fail if it doesn't exist in this Compose
			// project.
			targetContainerName := strings.TrimSpace(strings.Split(networkMode, ":")[1])
			c.ExtraOptions = append(c.ExtraOptions, "--network=container:"+targetContainerName)
			if !slices.Contains(c.DependsOn, targetContainerName) {
				c.DependsOn = append(c.DependsOn, targetContainerName)
			}
		default:
			return nil, fmt.Errorf("unsupported network_mode: %s", networkMode)
		}
	}

	if inBridgeNetwork {
		// Allow other containers to use service name as an alias.
		//
		// In the case of Podman, this alias applies to all networks the container is a part of.
		// Network-scoped aliases are handled below.
		//
		// See: https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-alias-alias
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--network-alias=%s", service.Name))
	}

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

	// https://docs.docker.com/compose/compose-file/05-services/#extra_hosts
	// https://docs.docker.com/engine/reference/commandline/run/#add-host
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#add-host-host-ip
	for hostname, ip := range service.ExtraHosts {
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--add-host=%s:%s", hostname, ip))
	}

	// https://docs.docker.com/compose/compose-file/05-services/#sysctls
	// https://docs.docker.com/engine/reference/commandline/run/#sysctl
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#sysctl-name-value
	for name, value := range service.Sysctls {
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--sysctl=%s=%s", name, value))
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

	// Add systemd dependencies on network(s).
	for _, networkName := range c.Networks {
		c.SystemdConfig.Unit.After = append(c.SystemdConfig.Unit.After, g.networkNameToService(networkName))
		c.SystemdConfig.Unit.Requires = append(c.SystemdConfig.Unit.Requires, g.networkNameToService(networkName))
	}
	// Add systemd dependency on root target.
	if !g.NoCreateRootTarget {
		c.SystemdConfig.Unit.PartOf = append(c.SystemdConfig.Unit.PartOf, fmt.Sprintf("%s.target", rootTarget(g.Runtime, g.Project)))
		c.SystemdConfig.Unit.WantedBy = append(c.SystemdConfig.Unit.WantedBy, fmt.Sprintf("%s.target", rootTarget(g.Runtime, g.Project)))
	}

	// Sort slices now that we're done processing the container.
	slices.Sort(c.DependsOn)
	slices.Sort(c.ExtraOptions)
	slices.Sort(c.Networks)

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

func (g *Generator) networkNameToService(name string) string {
	return fmt.Sprintf("%s-network-%s.service", g.Runtime, name)
}

func (g *Generator) volumeNameToService(name string) string {
	return fmt.Sprintf("%s-volume-%s.service", g.Runtime, name)
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
		used := false
		for _, c := range containers {
			if slices.Contains(c.Networks, n.Name) {
				used = true
				break
			}
		}
		// If a network is unused, we don't need to generate it.
		if !used && !g.GenerateUnusedResoures {
			continue
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
			Runtime: g.Runtime,
			// Volume name is not project-scoped to match Compose semantics.
			Name:         name,
			Driver:       volume.Driver,
			DriverOpts:   volume.DriverOpts,
			Labels:       volume.Labels,
			RemoveOnStop: g.RemoveVolumes,
		}

		// FIXME(aksiksi): Podman does not properly handle NFS if the volume
		// is a regular mount. So, we can just "patch" each container's volume
		// mapping to use a direct bind mount instead of a volume and then skip
		// creation of the volume entirely.
		if g.Runtime == ContainerRuntimePodman && v.Driver == "" {
			bindPath := v.DriverOpts["device"]
			if bindPath == "" {
				log.Printf("Volume %q has no device set; skipping", name)
				continue
			}
			for _, c := range containers {
				if volumeString, ok := c.Volumes[name]; ok {
					volumeString = strings.TrimPrefix(volumeString, name)
					c.Volumes[bindPath] = bindPath + volumeString
					delete(c.Volumes, name)
				}
			}
			continue
		}

		// If a volume is unused, we don't need to generate it.
		used := false
		for _, c := range containers {
			if _, ok := c.Volumes[name]; ok {
				used = true
				break
			}
		}
		if !used && !g.GenerateUnusedResoures {
			continue
		}
		volumes = append(volumes, v)
	}
	slices.SortFunc(volumes, func(n1, n2 *NixVolume) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return volumes
}
