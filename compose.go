package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

const (
	composeLabelPrefix = "compose2nix"
)

func parseNixContainerLabels(c *NixContainer, sopsConfig *SopsConfig) error {
	for label, v := range c.Labels {
		if !strings.HasPrefix(label, composeLabelPrefix) {
			continue
		}
		switch {
		case label == "compose2nix.settings.autoStart":
			if v == "true" {
				c.AutoStart = true
			} else if v == "false" {
				c.AutoStart = false
			} else {
				return fmt.Errorf("compose2nix.settings.autoStart must be: true or false")
			}
		case label == "compose2nix.settings.sops.secrets":
			if sopsConfig == nil {
				return fmt.Errorf("compose2nix.settings.sops.secrets defined, but not sops config specified")
			}
			for _, secret := range strings.Split(v, ",") {
				secret = strings.TrimSpace(secret)
				if secret == "" {
					continue
				}
				if !sopsConfig.HasSecret(secret) {
					return fmt.Errorf("sops secret %q not found in sops config file %q", secret, sopsConfig.FilePath)
				}
				c.SopsSecrets = append(c.SopsSecrets, secret)
			}
		case strings.HasPrefix(label, "compose2nix.systemd."):
			// This will be handled later.
			continue
		default:
			return fmt.Errorf("invalid compose2nix container label: %q", label)
		}
	}
	return nil
}

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

// Dummy interface that allows patching os.Getwd() in tests.
type getWorkingDir interface {
	GetWd() (string, error)
}

func (g *Generator) GetRootPath() (string, error) {
	if g.RootPath != "" {
		return g.RootPath, nil
	}
	cwd, err := g.GetWorkingDir.GetWd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	return cwd, nil
}

type Generator struct {
	Project                 *Project
	Runtime                 ContainerRuntime
	Inputs                  []string
	EnvFiles                []string
	RootPath                string
	IncludeEnvFiles         bool
	EnvFilesOnly            bool
	IgnoreMissingEnvFiles   bool
	ServiceInclude          *regexp.Regexp
	AutoStart               bool
	UseComposeLogDriver     bool
	GenerateUnusedResources bool
	CheckSystemdMounts      bool
	CheckBindMounts         bool
	UseUpheldBy             bool
	RemoveVolumes           bool
	NoCreateRootTarget      bool
	AutoFormat              bool
	WriteHeader             bool
	NoWriteNixSetup         bool
	DefaultStopTimeout      time.Duration
	IncludeBuild            bool
	GetWorkingDir           getWorkingDir
	OptionPrefix            string
	EnableOption            bool
	SopsConfig              *SopsConfig
	WarningsAsErrors        bool

	serviceToContainerName map[string]string
	rootPath               string
}

func (g *Generator) Run(ctx context.Context) (*NixContainerConfig, error) {
	rootPath, err := g.GetRootPath()
	if err != nil {
		return nil, err
	}
	g.rootPath = rootPath

	// Transform env files into absolute paths. This ensures that we can compare
	// them to Compose env files when building Nix containers.
	for i, p := range g.EnvFiles {
		if !path.IsAbs(p) {
			g.EnvFiles[i] = path.Join(rootPath, p)
		}
	}

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
	composeProject, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(g.Inputs),
		Environment: types.NewMapping(env),
		WorkingDir:  rootPath,
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

	networks, networkMap := g.buildNixNetworks(composeProject)
	volumes, volumeMap := g.buildNixVolumes(composeProject)
	containers, builds, err := g.buildNixContainers(composeProject, networkMap, volumeMap)
	if err != nil {
		return nil, err
	}

	// Post-process any Compose settings that require the full state.
	networks, volumes = g.postProcess(containers, networks, volumes)

	var version string
	if g.WriteHeader {
		version = appVersion
	}

	var option string = ""
	if g.OptionPrefix == "" {
		option = g.Project.Name
	} else {
		option = fmt.Sprintf("%s.%s", g.OptionPrefix, g.Project.Name)
	}

	return &NixContainerConfig{
		Version:          version,
		Project:          g.Project,
		Runtime:          g.Runtime,
		Containers:       containers,
		Builds:           builds,
		Networks:         networks,
		Volumes:          volumes,
		CreateRootTarget: !g.NoCreateRootTarget,
		AutoStart:        g.AutoStart,
		WriteNixSetup:    !g.NoWriteNixSetup,
		AutoFormat:       g.AutoFormat,
		IncludeBuild:     g.IncludeBuild,
		Option:           option,
		EnableOption:     g.EnableOption,
		SopsConfig:       g.SopsConfig,
	}, nil
}

func (g *Generator) postProcess(containers []*NixContainer, networks []*NixNetwork, volumes []*NixVolume) ([]*NixNetwork, []*NixVolume) {
	// Drop any networks that are unused or external.
	networks = slices.DeleteFunc(networks, func(n *NixNetwork) bool {
		used := false
		for _, c := range containers {
			if slices.Contains(c.Networks, n.Name) {
				used = true
				break
			}
		}
		return (!used && !g.GenerateUnusedResources) || n.External
	})

	// Drop any volumes that are unused or external.
	volumes = slices.DeleteFunc(volumes, func(v *NixVolume) bool {
		used := false
		for _, c := range containers {
			if _, ok := c.Volumes[v.Name]; ok {
				used = true
				break
			}
		}
		return (!used && !g.GenerateUnusedResources) || v.External
	})
	return networks, volumes
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
		return sliceToStringArray(cmd[1:]), nil
	}
	panic("unreachable")
}

// Health check.
// https://docs.docker.com/compose/compose-file/05-services/#healthcheck
func parseHealthCheck(c *NixContainer, service types.ServiceConfig, runtime ContainerRuntime) error {
	if healthCheck := service.HealthCheck; healthCheck != nil {
		// Figure out if the Dockerfile health check is disabled.
		disable := healthCheck.Disable || (len(healthCheck.Test) > 0 && healthCheck.Test[0] == "NONE")
		if disable {
			c.ExtraOptions = append(c.ExtraOptions, "--no-healthcheck")
		} else {
			if len(healthCheck.Test) > 0 {
				cmd, err := healthCheckCommandToString(healthCheck.Test)
				if err != nil {
					return fmt.Errorf("failed to convert healthcheck command: %w", err)
				}
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--health-cmd=%s", cmd))
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
			if startInterval := healthCheck.StartInterval; startInterval != nil {
				flagName := "health-start-interval"
				if runtime == ContainerRuntimePodman {
					// https://docs.podman.io/en/latest/markdown/podman-run.1.html#health-startup-interval-interval
					flagName = "health-startup-interval"
				}
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--%s=%v", flagName, *startInterval))
			}
		}
	}
	return nil
}

func (g *Generator) handleVolumesForService(service types.ServiceConfig, volumeMap map[string]*NixVolume, c *NixContainer) error {
	for _, v := range service.Volumes {
		if volume, ok := volumeMap[v.Source]; ok {

			// compose-go does not differentiate between short and long syntax, so we'll use long-only fields to try to tell the difference.

			hasLongSyntaxSubpath := v.Volume.Subpath != ""

			hasNoCopyRequested := v.Volume.NoCopy
			noCopySupported := c.Runtime == ContainerRuntimeDocker
			hasNoCopyEffective := hasNoCopyRequested && noCopySupported

			if hasNoCopyRequested && !noCopySupported {
				if err := g.checkOrWarn(
					"service %q: 'nocopy' is not supported for %s runtime and will be ignored",
					service.Name, c.Runtime,
				); err != nil {
					return err
				}
			}

			needsMountOptions := hasLongSyntaxSubpath || hasNoCopyEffective

			if needsMountOptions {
				// Handle long syntax by passing --mount flag to the backend
				// Docker: https://docs.docker.com/reference/cli/docker/container/run/#mount
				// Podman: https://docs.podman.io/en/latest/markdown/podman-run.1.html#mount-type-type-type-specific-option

				mount := fmt.Sprintf("type=%s,source=%s,target=%s", v.Type, volume.Name, v.Target)

				if hasLongSyntaxSubpath {
					mount += fmt.Sprintf(",volume-subpath=%s", v.Volume.Subpath)
				}

				if hasNoCopyEffective {
					mount += ",volume-nocopy"
				}

				if v.ReadOnly {
					mount += ",readonly"
				}

				c.ExtraOptions = append(c.ExtraOptions, "--mount="+mount)

				// Used to force generation of volume creation.
				// Empty strings are filtered from the volumes attribute.
				c.Volumes[volume.Name] = ""

			} else {
				// Replace the Compose volume name with the actual Docker volume
				// name (i.e., potentially prefixed with project).
				//
				// This is what we'll use to refer to the volume in the generated
				// container config.

				volumeParts := strings.Split(v.String(), ":")
				volumeParts[0] = volume.Name
				c.Volumes[volume.Name] = strings.Join(volumeParts, ":")
			}

			if !volume.External {
				// Add systemd dependencies on volume(s).
				c.SystemdConfig.Unit.After = append(c.SystemdConfig.Unit.After, g.volumeNameToService(volume.Name))
				c.SystemdConfig.Unit.Requires = append(c.SystemdConfig.Unit.Requires, g.volumeNameToService(volume.Name))
				if g.UseUpheldBy {
					c.SystemdConfig.Unit.UpheldBy = append(c.SystemdConfig.Unit.UpheldBy, g.volumeNameToService(volume.Name))
				}
			}

		} else {
			// This is a bind mount.
			sourcePath := v.Source

			if g.CheckBindMounts {
				if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
					return fmt.Errorf("service %q: bind mount source path %q does not exist", service.Name, sourcePath)
				}
			}

			// Replace the source path in the volume string.
			volumeString := strings.Split(v.String(), ":")
			volumeString[0] = sourcePath
			c.Volumes[sourcePath] = strings.Join(volumeString, ":")

			if g.CheckSystemdMounts {
				c.SystemdConfig.Unit.RequiresMountsFor = append(c.SystemdConfig.Unit.RequiresMountsFor, sourcePath)
			}
		}
	}

	return nil
}

func (g *Generator) checkOrWarn(format string, args ...any) error {
	if g.WarningsAsErrors {
		return fmt.Errorf(format, args...)
	}

	log.Printf("warning: "+format, args...)
	return nil
}

func (g *Generator) buildNixContainer(service types.ServiceConfig, networkMap map[string]*NixNetwork, volumeMap map[string]*NixVolume) (*NixContainer, error) {
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
		AutoStart:     g.AutoStart,
	}

	if err := parseNixContainerLabels(c, g.SopsConfig); err != nil {
		return nil, err
	}

	if g.IncludeEnvFiles || g.EnvFilesOnly {
		// Env files provided via CLI.
		c.EnvFiles = append(c.EnvFiles, g.EnvFiles...)

		// Env files set on the Compose service.
		for _, e := range service.EnvFiles {
			// It's possible that an env file that was passed in via CLI is
			// _also_ present in the Compose service (as an env_file).
			if !slices.Contains(c.EnvFiles, e.Path) {
				c.EnvFiles = append(c.EnvFiles, e.Path)
			}
		}
	}
	if !g.EnvFilesOnly {
		c.Environment = composeEnvironmentToMap(service.Environment)
	}

	if err := g.handleVolumesForService(service, volumeMap, c); err != nil {
		return nil, err
	}

	if !service.Command.IsZero() {
		c.Command = service.Command
	}
	if entrypoint := service.Entrypoint; !entrypoint.IsZero() {
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--entrypoint=%s", sliceToStringArray(entrypoint)))
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
			// TODO(aksiksi): Should we even be doing this?
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

		networkName := networkMap[name].Name
		c.Networks = append(c.Networks, networkName)

		networkFlag := fmt.Sprintf("--network=%s", networkName)

		if !networkMap[name].External {
			// Add systemd dependencies on network.
			c.SystemdConfig.Unit.After = append(c.SystemdConfig.Unit.After, g.networkNameToService(networkName))
			c.SystemdConfig.Unit.Requires = append(c.SystemdConfig.Unit.Requires, g.networkNameToService(networkName))
			if g.UseUpheldBy {
				c.SystemdConfig.Unit.UpheldBy = append(c.SystemdConfig.Unit.UpheldBy, g.networkNameToService(networkName))
			}
		}

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
	for _, d := range service.Devices {
		device := d.Source

		// Special case: Nvidia CDI devices use short syntax.
		if strings.HasPrefix(device, "nvidia.com") {
			c.ExtraOptions = append(c.ExtraOptions, "--device="+device)
			continue
		}

		// Otherwise, we default to using the long syntax.
		// https://docs.docker.com/reference/cli/docker/container/run/#device
		if d.Target != "" {
			device += fmt.Sprintf(":%s", d.Target)
		}
		if d.Permissions != "" {
			device += fmt.Sprintf(":%s", d.Permissions)
		}
		c.ExtraOptions = append(c.ExtraOptions, "--device="+device)
	}
	if service.Runtime != "" {
		c.ExtraOptions = append(c.ExtraOptions, "--runtime="+service.Runtime)
	}
	for _, opt := range service.SecurityOpt {
		c.ExtraOptions = append(c.ExtraOptions, "--security-opt="+opt)
	}

	// https://docs.docker.com/compose/compose-file/05-services/#extra_hosts
	// https://github.com/compose-spec/compose-spec/blob/master/spec.md#extra_hosts
	// https://docs.docker.com/engine/reference/commandline/run/#add-host
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#add-host-host-ip
	for hostname, ips := range service.ExtraHosts {
		// We can get upto two IPs per hostname: one v4 and one v6.
		// See: https://github.com/compose-spec/compose-go/pull/563
		for _, ip := range ips {
			c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--add-host=%s:%s", hostname, ip))
		}
	}

	if service.Hostname != "" {
		c.ExtraOptions = append(c.ExtraOptions, "--hostname="+service.Hostname)
	}

	// https://docs.docker.com/compose/compose-file/05-services/#group_add
	// https://docs.docker.com/engine/reference/commandline/run/#group-add
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#group-add-group
	for _, group := range service.GroupAdd {
		c.ExtraOptions = append(c.ExtraOptions, "--group-add="+group)
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
			c.ExtraOptions = append(c.ExtraOptions, mapToRepeatedKeyValFlag("--log-opt", service.LogOpt)...)
		}
	}
	// New logging setting always overrides the legacy setting.
	// https://docs.docker.com/compose/compose-file/compose-file-v3/#logging
	if logging := service.Logging; logging != nil {
		if logging.Driver != "json-file" || g.UseComposeLogDriver {
			c.LogDriver = logging.Driver
			c.ExtraOptions = append(c.ExtraOptions, mapToRepeatedKeyValFlag("--log-opt", logging.Options)...)
		}
	}

	if err := parseHealthCheck(c, service, g.Runtime); err != nil {
		return nil, err
	}

	// Deploy resources configuration.
	// https://docs.docker.com/compose/compose-file/deploy/#resources
	if deploy := service.Deploy; deploy != nil {
		if limits := deploy.Resources.Limits; limits != nil {
			if limits.MemoryBytes != 0 {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--memory=%db", limits.MemoryBytes))
			}
			// Name is misleading - this actually is the exact number passed in with "cpus".
			if limits.NanoCPUs != 0 {
				c.ExtraOptions = append(c.ExtraOptions, "--cpus="+strconv.FormatFloat(float64(limits.NanoCPUs), 'f', -1, 32))
			}
		}
		if reservations := deploy.Resources.Reservations; reservations != nil {
			if reservations.MemoryBytes != 0 {
				c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--memory-reservation=%db", reservations.MemoryBytes))
			}

			// CPU reservation is a Docker Swarm option.

			// CDI GPU support.
			for _, device := range reservations.Devices {
				driver := strings.ToLower(device.Driver)
				if driver != "cdi" && driver != "nvidia" {
					continue
				}
				if driver == "nvidia" {
					// Pass in all GPUs in CDI format.
					//
					// TODO(aksiksi): Maybe we can do something better here?
					c.ExtraOptions = append(c.ExtraOptions, "--device=nvidia.com/gpu=all")
					if err := g.checkOrWarn("\"driver: nvidia\" is implicitly converted to CDI that matches all GPUs"); err != nil {
						return nil, err
					}
					continue
				}
				for _, deviceID := range device.IDs {
					c.ExtraOptions = append(c.ExtraOptions, "--device="+deviceID)
				}
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
	slices.Sort(c.EnvFiles)
	slices.Sort(c.ExtraOptions)
	slices.Sort(c.Networks)

	// Add systemd dependency on root target.
	//
	// NOTE(aksiksi): We must check auto-start here because the root target
	// could have auto-start set, which would implicitly bring up the container.
	if !g.NoCreateRootTarget && c.AutoStart {
		c.SystemdConfig.Unit.PartOf = append(c.SystemdConfig.Unit.PartOf, fmt.Sprintf("%s.target", rootTarget(g.Runtime, g.Project)))
		c.SystemdConfig.Unit.WantedBy = append(c.SystemdConfig.Unit.WantedBy, fmt.Sprintf("%s.target", rootTarget(g.Runtime, g.Project)))
	}

	// Unfortunately, UpheldBy does not work as expected, so we're keeping it
	// behind a flag.
	//
	// Refer to the "Known Issues" section in the README for details.
	if g.UseUpheldBy {
		// Set UpheldBy for this service's dependencies. This ensures that, when
		// the dependency comes up, this container will also be started - and
		// continuously restarted with backoff - until it comes up.
		//
		// See: https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html#Upholds=
		//
		// Why do we need to do this? Because, by default, systemd does not
		// attempt to start failed dependent units when the parent (dependency)
		// comes up. See: https://github.com/systemd/systemd/issues/1312.
		//
		// For further discussion, see: https://github.com/aksiksi/compose2nix/issues/19
		for _, containerName := range c.DependsOn {
			c.SystemdConfig.Unit.UpheldBy = append(c.SystemdConfig.Unit.UpheldBy, g.containerNameToService(containerName))
		}
	}

	// systemd configs provided via labels always override everything else.
	if err := c.SystemdConfig.ParseSystemdLabels(&service); err != nil {
		return nil, err
	}

	return c, nil
}

func (g *Generator) parseServiceBuild(service types.ServiceConfig, c *NixContainer) (*NixBuild, error) {
	cx := service.Build.Context
	isGitRepo := false

	if strings.HasPrefix(cx, "http") {
		// Process this as a Git repo.
		isGitRepo = true
	} else if !path.IsAbs(cx) {
		cx = path.Join(g.rootPath, cx)
	}

	var imageName string
	if c.Image != "" {
		imageName = c.Image
	} else {
		// If no image is set on the service, we'll define an image name based
		// on the container name.
		imageName = fmt.Sprintf("compose2nix/%s", c.Name)
	}

	// Always use the image name as a tag.
	tags := []string{imageName}
	// Apply additional tags on top, prefixed with the image name.
	for _, tag := range service.Build.Tags {
		tags = append(tags, fmt.Sprintf("%s:%s", imageName, tag))
	}

	// Set the image on the container.
	if g.Runtime == ContainerRuntimePodman {
		// Podman automatically prepends a registry name of "localhost" to any
		// tag we set.
		//
		// See: https://docs.podman.io/en/latest/markdown/podman-build.1.html#tag-t-imagename
		c.Image = fmt.Sprintf("localhost/%s", imageName)
	} else {
		c.Image = imageName
	}

	b := &NixBuild{
		Runtime:       g.Runtime,
		Context:       cx,
		PullPolicy:    NewServicePullPolicy(service.PullPolicy),
		IsGitRepo:     isGitRepo,
		Args:          service.Build.Args,
		Tags:          tags,
		Dockerfile:    service.Build.Dockerfile,
		ContainerName: c.Name,
	}

	if g.IncludeBuild {
		// Add dependency on build systemd service.
		c.SystemdConfig.Unit.After = append(c.SystemdConfig.Unit.After, b.Unit())
		c.SystemdConfig.Unit.Requires = append(c.SystemdConfig.Unit.Requires, b.Unit())
		if g.UseUpheldBy {
			c.SystemdConfig.Unit.UpheldBy = append(c.SystemdConfig.Unit.UpheldBy, b.Unit())
		}
	}

	return b, nil
}

func (g *Generator) buildNixContainers(composeProject *types.Project, networkMap map[string]*NixNetwork, volumeMap map[string]*NixVolume) (containers []*NixContainer, builds []*NixBuild, _ error) {
	for _, s := range composeProject.Services {
		if g.ServiceInclude != nil && !g.ServiceInclude.MatchString(s.Name) {
			log.Printf("Skipping service %q due to include regex %q", s.Name, g.ServiceInclude.String())
			continue
		}
		c, err := g.buildNixContainer(s, networkMap, volumeMap)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build container for service %q: %w", s.Name, err)
		}
		containers = append(containers, c)

		if s.Build != nil {
			b, err := g.parseServiceBuild(s, c)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse build for service %q: %w", s.Name, err)
			}
			builds = append(builds, b)
		}

		c.SystemdConfig.Sort()
	}
	slices.SortFunc(containers, func(c1, c2 *NixContainer) int {
		return cmp.Compare(c1.Name, c2.Name)
	})
	slices.SortFunc(builds, func(c1, c2 *NixBuild) int {
		return cmp.Compare(c1.ContainerName, c2.ContainerName)
	})
	return containers, builds, nil
}

func (g *Generator) containerNameToService(name string) string {
	return fmt.Sprintf("%s-%s.service", g.Runtime, name)
}

func (g *Generator) networkNameToService(name string) string {
	return fmt.Sprintf("%s-network-%s.service", g.Runtime, name)
}

func (g *Generator) volumeNameToService(name string) string {
	return fmt.Sprintf("%s-volume-%s.service", g.Runtime, name)
}

func (g *Generator) buildNixNetworks(composeProject *types.Project) ([]*NixNetwork, map[string]*NixNetwork) {
	networkMap := make(map[string]*NixNetwork)

	var networks []*NixNetwork
	for name, network := range composeProject.Networks {
		n := &NixNetwork{
			Runtime:      g.Runtime,
			Name:         g.Project.With(name),
			OriginalName: name,
			Driver:       network.Driver,
			DriverOpts:   network.DriverOpts,
			External:     bool(network.External),
			Labels:       network.Labels,
		}

		if network.Name != "" {
			n.Name = network.Name
		}
		networkMap[name] = n

		if network.Internal {
			n.ExtraOptions = append(n.ExtraOptions, "--internal")
		}
		if enableIPv6 := network.EnableIPv6; enableIPv6 != nil && *enableIPv6 {
			n.ExtraOptions = append(n.ExtraOptions, "--ipv6")
		}

		// IPAM configuration.
		// https://docs.docker.com/compose/compose-file/06-networks/#ipam
		// https://docs.docker.com/reference/cli/docker/network/create/
		// https://docs.podman.io/en/latest/markdown/podman-network-create.1.html

		// If driver is set to "default", we'll omit it and fallback to the
		// runtime.
		if network.Ipam.Driver != "" && network.Ipam.Driver != "default" {
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

		networks = append(networks, n)
	}
	slices.SortFunc(networks, func(n1, n2 *NixNetwork) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return networks, networkMap
}

func (g *Generator) buildNixVolumes(composeProject *types.Project) ([]*NixVolume, map[string]*NixVolume) {
	volumeMap := make(map[string]*NixVolume)
	var volumes []*NixVolume
	for name, volume := range composeProject.Volumes {
		v := &NixVolume{
			Runtime:      g.Runtime,
			Name:         g.Project.With(name),
			Driver:       volume.Driver,
			DriverOpts:   volume.DriverOpts,
			External:     bool(volume.External),
			Labels:       volume.Labels,
			RemoveOnStop: g.RemoveVolumes,
		}

		if volume.Name != "" {
			v.Name = volume.Name
		}
		volumeMap[name] = v

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

	return volumes, volumeMap
}
