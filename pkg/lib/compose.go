package compose2nixos

import (
	"cmp"
	"context"
	"fmt"
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

func nixContainerFromService(service types.ServiceConfig, project string, envFiles []string, autoStart bool, envFilesOnly bool) NixContainer {
	dependsOn := service.GetDependencies()
	if project != "" {
		for i := range dependsOn {
			dependsOn[i] = fmt.Sprintf("%s%s", project, dependsOn[i])
		}
	}
	c := NixContainer{
		Project:   project,
		Name:      service.Name,
		Image:     service.Image,
		Labels:    service.Labels,
		Ports:     portConfigsToPortStrings(service.Ports),
		User:      service.User,
		Volumes:   make(map[string]string),
		Networks:  maps.Keys(service.Networks),
		DependsOn: dependsOn,
		AutoStart: autoStart,
	}
	slices.Sort(c.Networks)

	if !envFilesOnly {
		c.Environment = composeEnvironmentToMap(service.Environment)
	} else {
		c.EnvFiles = envFiles
	}

	var networkNames []string
	for _, name := range c.Networks {
		networkNames = append(networkNames, fmt.Sprintf("%s%s", project, name))
	}
	// TODO(aksiksi): Change this based on Podman vs. Docker.
	c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--networks=%s", strings.Join(networkNames, ",")))

	for _, v := range service.Volumes {
		c.Volumes[v.Source] = v.String()
	}

	return c
}

func nixContainersFromServices(services []types.ServiceConfig, project string, envFiles []string, autoStart bool, envFilesOnly bool) NixContainers {
	var containers []NixContainer
	for _, s := range services {
		containers = append(containers, nixContainerFromService(s, project, envFiles, autoStart, envFilesOnly))
	}
	slices.SortFunc(containers, func(c1, c2 NixContainer) int {
		return cmp.Compare(c1.Name, c2.Name)
	})
	return containers
}

func nixNetworksFromProject(p *types.Project, project string, containers NixContainers) NixNextworks {
	var networks []NixNetwork
	for name, network := range p.Networks {
		n := NixNetwork{
			Project: project,
			Name:    name,
			Labels:  network.Labels,
		}
		// Keep track of all containers that are in this network.
		for _, c := range containers {
			if slices.Contains(c.Networks, name) {
				n.Containers = append(n.Containers, c.Name)
			}
		}
		networks = append(networks, n)
	}
	slices.SortFunc(networks, func(n1, n2 NixNetwork) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return networks
}

func nixVolumesFromProject(p *types.Project, project string, containers NixContainers) NixVolumes {
	var volumes []NixVolume
	for name, volume := range p.Volumes {
		v := NixVolume{
			Name:       name,
			Driver:     volume.Driver,
			DriverOpts: volume.DriverOpts,
		}
		// Keep track of all containers that use this named volume.
		for _, c := range containers {
			if _, ok := c.Volumes[name]; ok {
				// Need to include project here b/c volumes are not project-aware.
				v.Containers = append(v.Containers, fmt.Sprintf("%s%s", project, c.Name))
			}
		}
		volumes = append(volumes, v)
	}
	slices.SortFunc(volumes, func(n1, n2 NixVolume) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return volumes
}

func ParseWithEnv(ctx context.Context, paths []string, project string, autoStart bool, envFiles []string, envFilesOnly bool) (*NixContainerConfig, error) {
	env, err := ReadEnvFiles(envFiles, !envFilesOnly)
	if err != nil {
		return nil, err
	}
	p, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(paths),
		Environment: types.NewMapping(env),
	})
	if err != nil {
		return nil, err
	}

	containers := nixContainersFromServices(p.Services, project, envFiles, autoStart, envFilesOnly)
	networks := nixNetworksFromProject(p, project, containers)
	volumes := nixVolumesFromProject(p, project, containers)

	return &NixContainerConfig{
		Networks:   networks,
		Containers: containers,
		Volumes:    volumes,
	}, nil
}
