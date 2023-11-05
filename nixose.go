package nixose

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/compose-spec/compose-go/types"
)

const DefaultProjectSeparator = "-"

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

func NewProject(name, separator string) *Project {
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
	Project    *Project
	Runtime    ContainerRuntime
	Name       string
	Labels     map[string]string
	Containers []string
}

type NixVolume struct {
	Project    *Project
	Runtime    ContainerRuntime
	Name       string
	Driver     string
	DriverOpts map[string]string
	Containers []string
}

// NixContainerSystemdConfig configures the container's systemd config.
// In particular, this allows control of the container restart policy through systemd
// service and unit configs.
//
// Each key-value pair in a map represents a systemd key and its value (e.g., Restart=always).
// Users can provide custom config keys by setting the nixose.systemd.* label on the service.
type NixContainerSystemdConfig struct {
	Service map[string]any
	Unit    map[string]any
	// NixOS treats these differently, probably to fix the rename issue in
	// earlier systemd versions.
	// See: https://unix.stackexchange.com/a/464098
	StartLimitBurst       *int
	StartLimitIntervalSec *int
}

// https://search.nixos.org/options?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=oci-container
type NixContainer struct {
	Project       *Project
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
	ExtraOptions  []string
	SystemdConfig *NixContainerSystemdConfig
	User          string
	AutoStart     bool

	// Original Docker Compose service.
	service *types.ServiceConfig
}

type NixContainerConfig struct {
	Project    *Project
	Runtime    ContainerRuntime
	Containers []NixContainer
	Networks   []NixNetwork
	Volumes    []NixVolume
}

func (c NixContainerConfig) String() string {
	s := strings.Builder{}
	execTemplateFuncMap := template.FuncMap{
		"execTemplate": execTemplate(nixTemplates),
	}
	nixTemplates := template.Must(nixTemplates.Funcs(execTemplateFuncMap).ParseFS(templateFS, "templates/*.tmpl"))
	if err := nixTemplates.ExecuteTemplate(&s, "main.nix.tmpl", c); err != nil {
		panic(err)
	}
	return s.String()
}
