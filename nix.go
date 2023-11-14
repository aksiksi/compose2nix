package main

import (
	"fmt"
	"strings"
	"text/template"
)

const DefaultProjectSeparator = "_"

type ContainerRuntime int

const (
	ContainerRuntimeInvalid ContainerRuntime = iota
	ContainerRuntimeDocker
	ContainerRuntimePodman
)

func (c ContainerRuntime) String() string {
	switch c {
	case ContainerRuntimeDocker:
		return "docker"
	case ContainerRuntimePodman:
		return "podman"
	case ContainerRuntimeInvalid:
		return "invalid-container-runtime"
	default:
		panic("Unreachable")
	}
}

type Project struct {
	Name      string
	separator string
}

func NewProject(name string) *Project {
	if name == "" {
		return nil
	}
	return &Project{Name: name, separator: DefaultProjectSeparator}
}

func NewProjectWithSeparator(name, separator string) *Project {
	if name == "" {
		return nil
	}
	if separator == "" {
		separator = DefaultProjectSeparator
	}
	return &Project{name, separator}
}

func (p *Project) With(name string) string {
	if p == nil {
		return name
	}
	return fmt.Sprintf("%s%s%s", p.Name, p.separator, name)
}

type NixNetwork struct {
	Runtime    ContainerRuntime
	Name       string
	Labels     map[string]string
	Containers []string
}

func (n *NixNetwork) Unit() string {
	return fmt.Sprintf("%s-network-%s.service", n.Runtime, n.Name)
}

type NixVolume struct {
	Runtime      ContainerRuntime
	Name         string
	Driver       string
	DriverOpts   map[string]string
	Containers   []string
	RemoveOnStop bool
}

func (v *NixVolume) Path() string {
	return v.DriverOpts["device"]
}

func (v *NixVolume) Unit() string {
	return fmt.Sprintf("%s-volume-%s.service", v.Runtime, v.Name)
}

// NixContainerSystemdConfig configures the container's systemd config.
// In particular, this allows control of the container restart policy through systemd
// service and unit configs.
//
// Each key-value pair in a map represents a systemd key and its value (e.g., Restart=always).
// Users can provide custom config keys by setting the nixose.systemd.* label on the service.
type NixContainerSystemdConfig struct {
	Service ServiceConfig
	Unit    UnitConfig
	// NixOS treats these differently, probably to fix the rename issue in
	// earlier systemd versions.
	// See: https://unix.stackexchange.com/a/464098
	StartLimitBurst       *int
	StartLimitIntervalSec *int
}

func NewNixContainerSystemdConfig() *NixContainerSystemdConfig {
	return &NixContainerSystemdConfig{
		Service: ServiceConfig{},
		Unit:    UnitConfig{},
	}
}

// https://search.nixos.org/options?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=oci-container
type NixContainer struct {
	Runtime       ContainerRuntime
	Name          string
	Image         string
	Environment   map[string]string
	EnvFiles      []string
	Volumes       map[string]string
	Ports         []string
	Labels        map[string]string
	Networks      []string
	DependsOn     []string
	LogDriver     string
	ExtraOptions  []string
	SystemdConfig *NixContainerSystemdConfig
	User          string
}

func (c *NixContainer) Unit() string {
	return fmt.Sprintf("%s-%s.service", c.Runtime, c.Name)
}

type NixContainerConfig struct {
	Version           string
	Project           *Project
	Runtime           ContainerRuntime
	Containers        []*NixContainer
	Networks          []*NixNetwork
	Volumes           []*NixVolume
	CreateRootService bool
	AutoStart         bool
}

func (c *NixContainerConfig) String() string {
	s := strings.Builder{}
	internalFuncMap := template.FuncMap{
		"cfg":          c.configTemplateFunc,
		"execTemplate": execTemplate(nixTemplates),
		"rootTarget":   c.rootTargetTemplateFunc,
	}
	nixTemplates := template.Must(nixTemplates.Funcs(internalFuncMap).ParseFS(templateFS, "templates/*.tmpl"))
	if err := nixTemplates.ExecuteTemplate(&s, "main.nix.tmpl", c); err != nil {
		// This should never be hit under normal operation.
		panic(err)
	}
	return s.String()
}

func (c *NixContainerConfig) rootTargetTemplateFunc() string {
	// NOTE(aksiksi): We can cache the list of units if this slows things down.
	if !c.CreateRootService || len(c.Units()) == 0 {
		return ""
	}
	return fmt.Sprintf("%s-compose-%s", c.Runtime, c.Project.With("root"))
}

func (c *NixContainerConfig) configTemplateFunc() *NixContainerConfig {
	return c
}

func (c *NixContainerConfig) Units() []string {
	var units []string
	for _, container := range c.Containers {
		units = append(units, container.Unit())
	}
	for _, network := range c.Networks {
		units = append(units, network.Unit())
	}
	for _, volume := range c.Volumes {
		units = append(units, volume.Unit())
	}
	return units
}
