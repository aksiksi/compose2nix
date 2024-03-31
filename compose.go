package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

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
	IgnoreMissingEnvFiles  bool
	ServiceInclude         *regexp.Regexp
	AutoStart              bool
	UseComposeLogDriver    bool
	GenerateUnusedResoures bool
	CheckSystemdMounts     bool
	RemoveVolumes          bool
	NoCreateRootTarget     bool
	WriteHeader            bool
	NoWriteNixSetup        bool
	DefaultStopTimeout     time.Duration

	serviceToContainerName map[string]string
}

func (g *Generator) Run(ctx context.Context) (*NixContainerConfig, error) {
	env, err := ReadEnvFiles(g.EnvFiles, !g.EnvFilesOnly, g.IgnoreMissingEnvFiles)
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
		WriteNixSetup:    !g.NoWriteNixSetup,
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

		// Add dependencies on systemd mounts for bind mounts used by this container, if any.
		if g.CheckSystemdMounts {
			for path := range c.Volumes {
				// Check to see if this is a named volume.
				isBindMount := true
				for _, v := range volumes {
					if v.Name == path {
						isBindMount = false
						break
					}
				}
				if !isBindMount {
					continue
				}
				if !strings.HasPrefix(path, "/") {
					log.Printf("Bind mount path %q is not absolute; skipping systemd mount dependency for service %q", path, serviceName)
					continue
				}
				c.SystemdConfig.Unit.RequiresMountsFor = append(c.SystemdConfig.Unit.RequiresMountsFor, path)
			}
			slices.Sort(c.SystemdConfig.Unit.RequiresMountsFor)
		}
	}

	return nil
}

func healthCheckCommandToString(cmd []string) (string, error) {
	if len(cmd) == 0 {
		return "", fmt.Errorf("empty cmd")
	}
	switch cmd[0] {
	case "NONE":
		return "", fmt.Errorf("cmd starts with NONE")
	case "CMD-SHELL":
		return cmd[1], nil
	case "CMD":
		j, err := json.Marshal(cmd[1:])
		if err != nil {
			return "", fmt.Errorf("failed to convert %v to JSON: %w", cmd[1:], err)
		}
		return string(j), nil
	}
	panic("unreachable")
}

func (g *Generator) buildNixContainer(service types.ServiceConfig) (*NixContainer, error) {
	name := g.serviceToContainerName[service.Name]

	c := &NixContainer{
		Runtime:       g.Runtime,
		Name:          name,
		Image:         service.Image,
		Labels:        service.Labels,
		Ports:         portConfigsToPortStrings(service.Ports),
		User:          service.User,
		Volumes:       make(map[string]string),
		SystemdConfig: NewNixContainerSystemdConfig(),
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

	if !service.Command.IsZero() {
		c.Command = service.Command
	}

	// Figure out explicit dependencies for this container.
	//
	// TODO(aksiksi): Support the long syntax.
	// https://docs.docker.com/compose/compose-file/05-services/#long-syntax-1
	for _, s := range service.GetDependencies() {
		targetContainerName, ok := g.serviceToContainerName[s]
		if !ok {
			return nil, fmt.Errorf("service %q depends on non-existent service %q", service.Name, s)
		}
		if !slices.Contains(c.DependsOn, targetContainerName) {
			c.DependsOn = append(c.DependsOn, targetContainerName)
		}
	}

	// If the container is connected to a network, it's counted as being in a bridge network.
	// We need to know this to be able to determine if we can configure a network alias.
	//
	// NOTE(aksiksi): Is this even correct?
	inBridgeNetwork := len(service.Networks) > 0

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

	var firstNetworkName string
	for name, net := range service.Networks {
		if firstNetworkName == "" {
			firstNetworkName = name
		}

		networkName := g.Project.With(name)
		c.Networks = append(c.Networks, networkName)

		networkFlag := fmt.Sprintf("--network=%s", networkName)

		// If we don't have any additional config set on this network, stop here.
		if net == nil {
			c.ExtraOptions = append(c.ExtraOptions, networkFlag)
			continue
		}

		switch g.Runtime {
		case ContainerRuntimeDocker:
			// Aliases are scoped to all networks - I think?
			for _, alias := range net.Aliases {
				c.ExtraOptions = append(c.ExtraOptions, "--network-alias="+alias)
			}
		case ContainerRuntimePodman:
			// Aliases are scoped to the current network.
			// https://docs.podman.io/en/latest/markdown/podman-run.1.html#network-mode-net
			var networkOpts []string
			for _, alias := range net.Aliases {
				networkOpts = append(networkOpts, "alias="+alias)
			}

			// Below, we fallback to using --ip/--ip6 if a single network is
			// specified. This aligns with Docker behavior.
			if len(service.Networks) > 1 {
				if net.Ipv4Address != "" {
					networkOpts = append(networkOpts, "ip="+net.Ipv4Address)
				}
				if net.Ipv6Address != "" {
					networkOpts = append(networkOpts, "ip="+net.Ipv6Address)
				}
			}

			if len(networkOpts) > 0 {
				networkFlag += fmt.Sprintf(":%s", strings.Join(networkOpts, ","))
			}
		}

		c.ExtraOptions = append(c.ExtraOptions, networkFlag)
	}

	// NOTE(aksiksi): Docker might actually support network-scoped IPs.
	//
	// But, we need to think about this carefully because the flags seem to be
	// order-depdendent. That is, the order of --network and --ip/--ip6 needs
	// to be maintained. Since we sort the ExtraOptions array, the ordering
	// would break without some changes.
	if net := service.Networks[firstNetworkName]; len(service.Networks) == 1 && net != nil {
		if net.Ipv4Address != "" {
			c.ExtraOptions = append(c.ExtraOptions, "--ip="+net.Ipv4Address)
		}
		if net.Ipv6Address != "" {
			c.ExtraOptions = append(c.ExtraOptions, "--ip6="+net.Ipv6Address)
		}
	}

	if service.MacAddress != "" {
		c.ExtraOptions = append(c.ExtraOptions, "--mac-address="+service.MacAddress)
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
	if service.Runtime != "" {
		c.ExtraOptions = append(c.ExtraOptions, "--runtime="+service.Runtime)
	}
	for _, opt := range service.SecurityOpt {
		c.ExtraOptions = append(c.ExtraOptions, "--security-opt="+opt)
	}

	// https://docs.docker.com/compose/compose-file/05-services/#extra_hosts
	// https://docs.docker.com/engine/reference/commandline/run/#add-host
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#add-host-host-ip
	for hostname, ip := range service.ExtraHosts {
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--add-host=%s:%s", hostname, ip))
	}

	if service.Hostname != "" {
		c.ExtraOptions = append(c.ExtraOptions, "--hostname="+service.Hostname)
	}

	// https://docs.docker.com/compose/compose-file/05-services/#sysctls
	// https://docs.docker.com/engine/reference/commandline/run/#sysctl
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#sysctl-name-value
	for name, value := range service.Sysctls {
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--sysctl=%s=%s", name, value))
	}

	if service.ShmSize != 0 {
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--shm-size=%d", service.ShmSize))
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

	// Health check.
	// https://docs.docker.com/compose/compose-file/05-services/#healthcheck
	if healthCheck := service.HealthCheck; healthCheck != nil {
		// Figure out if the Dockerfile health check is disabled.
		disable := healthCheck.Disable || (len(healthCheck.Test) > 0 && healthCheck.Test[0] == "NONE")
		if disable {
			c.ExtraOptions = append(c.ExtraOptions, "--no-healthcheck")
		} else {
			if len(healthCheck.Test) > 0 {
				cmd, err := healthCheckCommandToString(healthCheck.Test)
				if err != nil {
					return nil, fmt.Errorf("failed to convert healthcheck command: %w", err)
				}

				// We need to escape double-quotes for Nix.
				//
				// We also need to escape the special "${" sequence as it is possible that this is
				// passed in to evaluate a Bash env variable as part of the command.
				//
				// See: https://nixos.org/manual/nix/stable/language/values
				cmd = strings.ReplaceAll(cmd, `"`, `\"`)
				cmd = strings.ReplaceAll(cmd, "${", `\${`)

				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-cmd='%s'", cmd))
			}
			if timeout := healthCheck.Timeout; timeout != nil {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-timeout=%v", *timeout))
			}
			if interval := healthCheck.Interval; interval != nil {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-interval=%v", *interval))
			}
			if retries := healthCheck.Retries; retries != nil {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-retries=%d", *retries))
			}
			if startPeriod := healthCheck.StartPeriod; startPeriod != nil {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-start-period=%v", *startPeriod))
			}
			// Not supported by Docker.
			if startInterval := healthCheck.StartInterval; startInterval != nil && g.Runtime == ContainerRuntimePodman {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-start-interval=%v", *startInterval))
			}
		}
	}

	// Deploy resources configuration.
	// https://docs.docker.com/compose/compose-file/deploy/#resources
	if deploy := service.Deploy; deploy != nil {
		if limits := deploy.Resources.Limits; limits != nil {
			if limits.MemoryBytes != 0 {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--memory=%db", limits.MemoryBytes))
			}
			// Name is misleading - this actually is the exact number passed in with "cpus".
			if limits.NanoCPUs != "" {
				c.ExtraOptions = append(c.ExtraOptions, "--cpu-quota="+limits.NanoCPUs)
			}
		}
		if reservations := deploy.Resources.Reservations; reservations != nil {
			if reservations.MemoryBytes != 0 {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--memory-reservation=%db", reservations.MemoryBytes))
			}
			// Name is misleading - this actually is the exact number passed in with "cpus".
			if reservations.NanoCPUs != "" {
				c.ExtraOptions = append(c.ExtraOptions, "--cpus="+reservations.NanoCPUs)
			}
		}
	}

	// Restart policy.
	if err := c.SystemdConfig.ParseRestartPolicy(&service, g.Runtime); err != nil {
		return nil, err
	}

	// Override systemd stop timeout to match Docker/Podman default of 10 seconds.
	// https://docs.podman.io/en/latest/markdown/podman-stop.1.html
	//
	// Users can always override this by setting per-service Compose labels, or by passing in a CLI
	// flag.
	if g.DefaultStopTimeout == 0 {
		g.DefaultStopTimeout = defaultSystemdStopTimeout
	}
	if g.DefaultStopTimeout != defaultSystemdStopTimeout {
		// We only set a timeout if it's not the same as the systemd default.
		c.SystemdConfig.Service.Set("TimeoutStopSec", int(g.DefaultStopTimeout.Seconds()))
	}

	// Sort slices now that we're done processing the container.
	slices.Sort(c.DependsOn)
	slices.Sort(c.ExtraOptions)
	slices.Sort(c.Networks)

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

	// Set UpheldBy for this service's dependencies. This ensures that, when the dependency comes up,
	// this container will also be started - and continuously restarted with backoff - until it comes up.
	// See: https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html#Upholds=
	//
	// Why do we need to do this? Because, by default, systemd does not attempt to start failed dependent units
	// when the parent (dependency) comes up. See: https://github.com/systemd/systemd/issues/1312.
	//
	// Note 2: Upholds is supported in version 249+. NixOS 23.05 uses 253.6, so this should always be supported.
	// See: https://github.com/NixOS/nixpkgs/blob/nixos-23.05/pkgs/os-specific/linux/systemd/default.nix#L148.
	for _, containerName := range c.DependsOn {
		c.SystemdConfig.Unit.UpheldBy = append(c.SystemdConfig.Unit.UpheldBy, fmt.Sprintf("%s-%s.service", g.Runtime, containerName))
	}

	// systemd configs provided via labels always override everything else.
	if err := c.SystemdConfig.ParseSystemdLabels(&service); err != nil {
		return nil, err
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
			Runtime:    g.Runtime,
			Name:       g.Project.With(name),
			Driver:     network.Driver,
			DriverOpts: network.DriverOpts,
			Labels:     network.Labels,
		}

		// IPAM configuration.
		// https://docs.docker.com/compose/compose-file/06-networks/#ipam
		// https://docs.docker.com/reference/cli/docker/network/create/
		// https://docs.podman.io/en/latest/markdown/podman-network-create.1.html
		if network.Ipam.Driver != "" {
			n.IpamDriver = network.Ipam.Driver
		}
		for _, ipamConfig := range network.Ipam.Config {
			cfg := IpamConfig{
				Subnet:  ipamConfig.Subnet,
				IPRange: ipamConfig.IPRange,
				Gateway: ipamConfig.Gateway,
			}
			if g.Runtime == ContainerRuntimeDocker {
				for k, v := range ipamConfig.AuxiliaryAddresses {
					cfg.AuxAddresses = append(cfg.AuxAddresses, fmt.Sprintf("%s=%s", k, v))
				}
				slices.Sort(cfg.AuxAddresses)
			}
			n.IpamConfigs = append(n.IpamConfigs, cfg)
		}

		// Keep track of all container services that are in this network.
		// If a network is unused, we don't need to generate it.
		used := false
		for _, c := range containers {
			if slices.Contains(c.Networks, n.Name) {
				used = true
				break
			}
		}
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

	if g.CheckSystemdMounts {
		for name, volume := range volumes {
			path := volume.Path()
			if !strings.HasPrefix(path, "/") {
				log.Printf("Volume path %q is not absolute; skipping systemd mount dependency for volume %q", path, name)
				continue
			}
			volume.RequiresMountsFor = append(volume.RequiresMountsFor, path)
		}
	}

	return volumes
}
