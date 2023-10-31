package compose2nixos

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

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

func nixContainerFromService(service types.ServiceConfig, namePrefix string, autoStart bool) *NixContainer {
	dependsOn := service.GetDependencies()
	if namePrefix != "" {
		for i := range dependsOn {
			dependsOn[i] = fmt.Sprintf("%s%s", namePrefix, dependsOn[i])
		}
	}

	n := &NixContainer{
		Name:        service.Name,
		Prefix:      namePrefix,
		Image:       service.Image,
		Environment: service.Environment,
		Labels:      service.Labels,
		Ports:       portConfigsToPortStrings(service.Ports),
		User:        service.User,
		DependsOn:   dependsOn,
		AutoStart:   autoStart,
		// TODO(aksiksi): Extra options.
	}
	for _, v := range service.Volumes {
		n.Volumes = append(n.Volumes, v.String())
	}
	return n
}

func nixContainersFromServices(services []types.ServiceConfig, namePrefix string, autoStart bool) NixContainers {
	var containers []*NixContainer
	for _, s := range services {
		containers = append(containers, nixContainerFromService(s, namePrefix, autoStart))
	}
	slices.SortFunc(containers, func(c1, c2 *NixContainer) int {
		return cmp.Compare(c1.Name, c2.Name)
	})
	return containers
}

func ParseWithEnv(ctx context.Context, paths []string, namePrefix string, autoStart bool, envFiles []string, mergeWithEnv bool) (NixContainers, error) {
	env, err := ReadEnvFiles(envFiles, mergeWithEnv)
	if err != nil {
		return nil, err
	}
	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(paths),
		Environment: types.NewMapping(env),
	})
	if err != nil {
		return nil, err
	}
	return nixContainersFromServices(project.Services, namePrefix, autoStart), nil
}
