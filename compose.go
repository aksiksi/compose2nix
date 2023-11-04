package nixose

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"golang.org/x/exp/maps"
)

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

func (g *Generator) buildNixContainer(service types.ServiceConfig) NixContainer {
	dependsOn := service.GetDependencies()
	if g.Project != nil {
		for i := range dependsOn {
			dependsOn[i] = g.Project.With(dependsOn[i])
		}
	}
	c := NixContainer{
		Project:   g.Project,
		Runtime:   g.Runtime,
		Name:      service.Name,
		Image:     service.Image,
		Labels:    service.Labels,
		Ports:     portConfigsToPortStrings(service.Ports),
		User:      service.User,
		Volumes:   make(map[string]string),
		Networks:  maps.Keys(service.Networks),
		DependsOn: dependsOn,
		AutoStart: g.AutoStart,
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
