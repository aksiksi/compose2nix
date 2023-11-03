package compose2nixos

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
	labels := mapToKeyValArray(l)
	for i, label := range labels {
		labels[i] = fmt.Sprintf("--label=%s", label)
	}
	return labels
}

func execTemplate(t *template.Template) func(string, interface{}) (string, error) {
	return func(name string, v interface{}) (string, error) {
		var buf strings.Builder
		err := t.ExecuteTemplate(&buf, name, v)
		return buf.String(), err
	}
}

var funcMap template.FuncMap = template.FuncMap{
	"labelMapToLabelFlags": labelMapToLabelFlags,
	"mapToKeyValArray":     mapToKeyValArray,
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
