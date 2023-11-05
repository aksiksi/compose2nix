package nixose

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
)

//go:embed templates/*.tmpl
var templateFS embed.FS
var nixTemplates = template.New("nix").Funcs(sprig.FuncMap()).Funcs(funcMap)

func labelMapToLabelFlags(l map[string]string) []string {
	// https://docs.docker.com/engine/reference/commandline/run/#label
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#label-l-key-value
	labels := mapToKeyValArray(l)
	for i, label := range labels {
		labels[i] = fmt.Sprintf("--label=%s", label)
	}
	return labels
}

func execTemplate(t *template.Template) func(string, any) (string, error) {
	return func(name string, v any) (string, error) {
		var s strings.Builder
		err := t.ExecuteTemplate(&s, name, v)
		return s.String(), err
	}
}

var funcMap template.FuncMap = template.FuncMap{
	"labelMapToLabelFlags": labelMapToLabelFlags,
	"mapToKeyValArray":     mapToKeyValArray,
}

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

// https://search.nixos.org/options?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=oci-container
type NixContainer struct {
	Project      *Project
	Runtime      ContainerRuntime
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
	if err := nixTemplates.ExecuteTemplate(&s, "main.tmpl", c); err != nil {
		panic(err)
	}
	return s.String()
}
