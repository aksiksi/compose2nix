package compose2nixos

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"golang.org/x/exp/maps"
)

const nixContainerOption = "virtualisation.oci-containers.containers"

func containerToServiceName(name, project, projectSeparator string) string {
	if project != "" {
		return fmt.Sprintf("podman-%s%s%s.service", project, projectSeparator, name)
	} else {
		return fmt.Sprintf("podman-%s.service", name)
	}
}

func toNixList(elems []string, depth int) string {
	b := strings.Builder{}
	indent := strings.Repeat(" ", depth*2)
	b.WriteString("[\n")

	// Sort elements for stability.
	slices.Sort(elems)
	for _, elem := range elems {
		b.WriteString(fmt.Sprintf("%s%q\n", indent, elem))
	}

	b.WriteString(fmt.Sprintf("%s]", indent[:len(indent)-2]))
	return b.String()
}

func toNixAttributes(elems map[string]string, depth int, quoteKeys bool) string {
	b := strings.Builder{}
	b.WriteString("{\n")

	// Sort keys for stability.
	keys := maps.Keys(elems)
	slices.Sort(keys)
	indent := strings.Repeat(" ", depth*2)
	for _, k := range keys {
		v := elems[k]
		if !quoteKeys {
			b.WriteString(fmt.Sprintf("%s%s = %q;\n", indent, k, v))
		} else {
			b.WriteString(fmt.Sprintf("%s%q = %q;\n", indent, k, v))
		}
	}

	b.WriteString(fmt.Sprintf("%s}", indent[:len(indent)-2]))
	return b.String()
}

type NixNetwork struct {
	Name       string
	Labels     map[string]string
	Containers []string
}

// https://discourse.nixos.org/t/podman-container-to-container-networking/11647/2
func (n NixNetwork) ToNix(project, projectSeparator string, depth int) string {
	networkName := resourceNameWithProject(n.Name, project, projectSeparator)

	// TODO(aksiksi): Docker support.
	labels := mapToKeyValArray(n.Labels)
	for i, label := range labels {
		labels[i] = fmt.Sprintf("--label=%s", label)
	}

	var wantedBy []string
	for _, name := range n.Containers {
		wantedBy = append(wantedBy, containerToServiceName(name, project, projectSeparator))
	}

	s := strings.Builder{}
	indent := strings.Repeat(" ", depth*2)
	s.WriteString(fmt.Sprintf("%ssystemd.services.\"create-network-%s\" = {\n", indent, networkName))
	s.WriteString(fmt.Sprintf("%s  serviceConfig.Type = \"oneshot\";\n", indent))
	s.WriteString(fmt.Sprintf("%s  wantedBy = %s;\n", indent, toNixList(wantedBy, depth+2)))
	s.WriteString(fmt.Sprintf("%s  script = ''\n", indent))

	// The isolate option ensures that different networks cannot communicate.
	// See: https://github.com/containers/podman/issues/5805
	if len(labels) == 0 {
		s.WriteString(fmt.Sprintf("%s    ${pkgs.podman}/bin/podman network create %s --opt isolate=true --ignore\n", indent, networkName))
	} else {
		s.WriteString(fmt.Sprintf("%s    ${pkgs.podman}/bin/podman network create %s --opt isolate=true --ignore %s\n", indent, networkName, strings.Join(labels, " ")))
	}

	s.WriteString(fmt.Sprintf("%s  '';\n", indent))
	s.WriteString(fmt.Sprintf("%s};\n", indent))
	return s.String()
}

type NixNextworks []NixNetwork

func (n NixNextworks) ToNix(project, projectSeparator string) string {
	s := strings.Builder{}
	for _, net := range n {
		s.WriteString(net.ToNix(project, projectSeparator, 1))
	}
	return s.String()
}

// NOTE(aksiksi): Volume name is _not_ project scoped to match Compose semantics.
type NixVolume struct {
	Name       string
	Driver     string
	DriverOpts map[string]string
	Containers []string
}

func (v *NixVolume) ToNix(project, projectSeparator string, depth int) string {
	driverOptsString := strings.Join(mapToKeyValArray(v.DriverOpts), ",")

	var wantedBy []string
	for _, name := range v.Containers {
		wantedBy = append(wantedBy, containerToServiceName(name, project, projectSeparator))
	}

	s := strings.Builder{}
	indent := strings.Repeat(" ", depth*2)
	s.WriteString(fmt.Sprintf("%ssystemd.services.\"create-volume-%s\" = {\n", indent, v.Name))
	s.WriteString(fmt.Sprintf("%s  serviceConfig.Type = \"oneshot\";\n", indent))
	s.WriteString(fmt.Sprintf("%s  wantedBy = %s;\n", indent, toNixList(wantedBy, depth+2)))
	s.WriteString(fmt.Sprintf("%s  script = ''\n", indent))
	if v.Driver != "" {
		s.WriteString(fmt.Sprintf("%s    ${pkgs.podman}/bin/podman volume create %s --driver %s --opt %s --ignore\n", indent, v.Name, v.Driver, driverOptsString))
	} else {
		s.WriteString(fmt.Sprintf("%s    ${pkgs.podman}/bin/podman volume create %s --opt %s --ignore\n", indent, v.Name, driverOptsString))
	}
	s.WriteString(fmt.Sprintf("%s  '';\n", indent))
	s.WriteString(fmt.Sprintf("%s};\n", indent))

	return s.String()
}

type NixVolumes []NixVolume

func (n NixVolumes) ToNix(project, projectSeparator string) string {
	s := strings.Builder{}
	for _, v := range n {
		s.WriteString(v.ToNix(project, projectSeparator, 1))
	}
	return s.String()
}

// https://search.nixos.org/options?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=oci-container
type NixContainer struct {
	Name         string
	Image        string
	Environment  map[string]string
	EnvFiles     []string
	Volumes      map[string]string
	Ports        []string
	Labels       map[string]string
	Networks     []string
	DependsOn    []string
	ExtraOptions []string
	User         string
	AutoStart    bool
}

func (c *NixContainer) ToNix(project, projectSeparator string, depth int) string {
	s := strings.Builder{}
	indent := strings.Repeat(" ", depth*2)

	var name string
	if project != "" {
		name = fmt.Sprintf("%s%s%s", project, projectSeparator, c.Name)
	} else {
		name = c.Name
	}
	s.WriteString(fmt.Sprintf("%s%q = {\n", indent, name))

	s.WriteString(fmt.Sprintf("%s  image = %q;\n", indent, c.Image))
	if len(c.Environment) > 0 {
		s.WriteString(fmt.Sprintf("%s  environment = %s;\n", indent, toNixAttributes(c.Environment, depth+2, false)))
	}
	if len(c.EnvFiles) > 0 {
		var nixEnvFiles []string
		for _, p := range c.EnvFiles {
			nixEnvFiles = append(nixEnvFiles, fmt.Sprintf("${./%s}", path.Base(p)))
		}
		s.WriteString(fmt.Sprintf("%s  environmentFiles = %s;\n", indent, toNixList(nixEnvFiles, depth+2)))
	}
	if len(c.Volumes) > 0 {
		s.WriteString(fmt.Sprintf("%s  volumes = %s;\n", indent, toNixList(maps.Values(c.Volumes), depth+2)))
	}
	if len(c.Ports) > 0 {
		s.WriteString(fmt.Sprintf("%s  ports = %s;\n", indent, toNixList(c.Ports, depth+2)))
	}
	if len(c.Labels) > 0 {
		s.WriteString(fmt.Sprintf("%s  labels = %s;\n", indent, toNixAttributes(c.Labels, depth+2, true)))
	}
	if len(c.DependsOn) > 0 {
		s.WriteString(fmt.Sprintf("%s  dependsOn = %s;\n", indent, toNixList(c.DependsOn, depth+2)))
	}
	if len(c.ExtraOptions) > 0 {
		s.WriteString(fmt.Sprintf("%s  extraOptions = %s;\n", indent, toNixList(c.ExtraOptions, depth+2)))
	}
	if c.User != "" {
		s.WriteString(fmt.Sprintf("%s  user = %q;\n", indent, c.User))
	}
	if !c.AutoStart {
		s.WriteString(fmt.Sprintf("%s  autoStart = false;\n", indent))
	}

	s.WriteString(fmt.Sprintf("%s};\n", indent))
	return s.String()
}

type NixContainers []NixContainer

func (n NixContainers) ToNix(project, projectSeparator string) string {
	s := strings.Builder{}
	s.WriteString(fmt.Sprintf("  %s = {\n", nixContainerOption))
	for _, c := range n {
		s.WriteString(c.ToNix(project, projectSeparator, 2))
	}
	s.WriteString("  };\n")
	return s.String()
}

type NixContainerConfig struct {
	// TODO(aksiksi): Merge project and sep. into a type.
	Project          string
	ProjectSeparator string
	Containers       NixContainers
	Networks         NixNextworks
	Volumes          NixVolumes
}

func (c NixContainerConfig) ToNix() string {
	s := strings.Builder{}
	s.WriteString("{ pkgs, ... }:\n\n")
	s.WriteString("{\n")

	s.WriteString("  # Containers\n")
	s.WriteString(c.Containers.ToNix(c.Project, c.ProjectSeparator))

	if len(c.Networks) > 0 {
		s.WriteString("  # Networks\n")
		s.WriteString(c.Networks.ToNix(c.Project, c.ProjectSeparator))
	}

	if len(c.Volumes) > 0 {
		s.WriteString("  # Volumes\n")
		s.WriteString(c.Volumes.ToNix(c.Project, c.ProjectSeparator))
	}

	s.WriteString(c.NixUpScript())
	s.WriteString(c.NixDownScript())

	s.WriteString("}\n")
	return s.String()
}

func (c NixContainerConfig) NixUpScript() string {
	s := strings.Builder{}

	var scriptName string
	if c.Project != "" {
		scriptName = fmt.Sprintf("compose-%s-up.sh", c.Project)
	} else {
		scriptName = "compose-up.sh"
	}

	s.WriteString(fmt.Sprintf("  up = writeShellScript \"%s\" ''\n", scriptName))
	s.WriteString("    echo \"TODO: Create all resources.\"\n")
	s.WriteString("  '';\n")
	return s.String()
}

func (c NixContainerConfig) NixDownScript() string {
	s := strings.Builder{}

	var scriptName string
	if c.Project != "" {
		scriptName = fmt.Sprintf("compose-%s-down.sh", c.Project)
	} else {
		scriptName = "compose-down.sh"
	}

	s.WriteString(fmt.Sprintf("  down = writeShellScript \"%s\" ''\n", scriptName))
	s.WriteString("    echo \"TODO: Delete all resources.\"\n")
	s.WriteString("  '';\n")
	return s.String()
}
