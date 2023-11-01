package compose2nixos

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

const DefaultProjectSeparator = "-"

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

func projectWithSeparator(project, projectSeparator string) string {
	return fmt.Sprintf("%s%s", project, projectSeparator)
}

func resourceNameWithProject(name, project, projectSeparator string) string {
	if project != "" {
		return fmt.Sprintf("%s%s", projectWithSeparator(project, projectSeparator), name)
	} else {
		return name
	}
}

type Parser struct {
	Project          string
	ProjectSeparator string
	Paths            []string
	EnvFiles         []string
	AutoStart        bool
	EnvFilesOnly     bool
	composeProject   *types.Project
}

func (p *Parser) buildNixContainer(service types.ServiceConfig) NixContainer {
	dependsOn := service.GetDependencies()
	if p.Project != "" {
		for i := range dependsOn {
			dependsOn[i] = fmt.Sprintf("%s%s", p.Project, dependsOn[i])
		}
	}
	c := NixContainer{
		Name:      service.Name,
		Image:     service.Image,
		Labels:    service.Labels,
		Ports:     portConfigsToPortStrings(service.Ports),
		User:      service.User,
		Volumes:   make(map[string]string),
		Networks:  maps.Keys(service.Networks),
		DependsOn: dependsOn,
		AutoStart: p.AutoStart,
	}
	slices.Sort(c.Networks)

	if !p.EnvFilesOnly {
		c.Environment = composeEnvironmentToMap(service.Environment)
	} else {
		c.EnvFiles = p.EnvFiles
	}

	for _, name := range c.Networks {
		networkName := resourceNameWithProject(name, p.Project, p.ProjectSeparator)
		// TODO(aksiksi): Change this based on Podman vs. Docker.
		c.ExtraOptions = append(c.ExtraOptions, fmt.Sprintf("--network=%s", networkName))
	}

	for _, v := range service.Volumes {
		c.Volumes[v.Source] = v.String()
	}

	return c
}

func (p *Parser) buildNixContainers() NixContainers {
	var containers []NixContainer
	for _, s := range p.composeProject.Services {
		containers = append(containers, p.buildNixContainer(s))
	}
	slices.SortFunc(containers, func(c1, c2 NixContainer) int {
		return cmp.Compare(c1.Name, c2.Name)
	})
	return containers
}

func (p *Parser) buildNixNetworks(containers NixContainers) NixNextworks {
	var networks []NixNetwork
	for name, network := range p.composeProject.Networks {
		n := NixNetwork{
			Name:   name,
			Labels: network.Labels,
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

func (p *Parser) buildNixVolumes(containers NixContainers) NixVolumes {
	var volumes []NixVolume
	for name, volume := range p.composeProject.Volumes {
		v := NixVolume{
			Name:       name,
			Driver:     volume.Driver,
			DriverOpts: volume.DriverOpts,
		}

		// FIXME(aksiksi): Podman does not properly handle NFS if the volume
		// is a regular mount. So, we can just "patch" each container's volume
		// mapping to use a direct bind mount instead of a volume and then skip
		// creation of the volume entirely.
		if v.Driver == "" {
			bindPath := v.DriverOpts["device"]
			if bindPath == "" {
				log.Fatalf("volume %q has no device set", name)
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
				// Need to include project here b/c volumes are not project-aware.
				v.Containers = append(v.Containers, resourceNameWithProject(c.Name, p.Project, p.ProjectSeparator))
			}
		}
		volumes = append(volumes, v)
	}
	slices.SortFunc(volumes, func(n1, n2 NixVolume) int {
		return cmp.Compare(n1.Name, n2.Name)
	})
	return volumes
}

func (p *Parser) Parse(ctx context.Context) (*NixContainerConfig, error) {
	if p.Project != "" && p.ProjectSeparator == "" {
		p.ProjectSeparator = DefaultProjectSeparator
	}

	env, err := ReadEnvFiles(p.EnvFiles, !p.EnvFilesOnly)
	if err != nil {
		return nil, err
	}
	composeProject, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(p.Paths),
		Environment: types.NewMapping(env),
	})
	if err != nil {
		return nil, err
	}
	p.composeProject = composeProject

	containers := p.buildNixContainers()
	networks := p.buildNixNetworks(containers)
	volumes := p.buildNixVolumes(containers)

	return &NixContainerConfig{
		Project:          p.Project,
		ProjectSeparator: p.ProjectSeparator,
		Containers:       containers,
		Networks:         networks,
		Volumes:          volumes,
	}, nil
}
