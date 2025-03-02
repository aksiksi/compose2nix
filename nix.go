package main

import (
	"fmt"
	"io"
	"strings"
	"text/template"
)

// Compose V2 uses "-" for container names: https://docs.docker.com/compose/migrate/#service-container-names
//
// Volumes and networks still use "_", but we'll ignore that here: https://github.com/docker/compose/issues/9618
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

func NewProject(name string) *Project {
	if name == "" {
		return nil
	}
	return &Project{Name: name, separator: DefaultProjectSeparator}
}

func (p *Project) With(name string) string {
	if p == nil {
		return name
	}
	return fmt.Sprintf("%s%s%s", p.Name, p.separator, name)
}

type IpamConfig struct {
	Subnet       string
	IPRange      string
	Gateway      string
	AuxAddresses []string
}

type NixNetwork struct {
	Runtime      ContainerRuntime
	Name         string
	OriginalName string
	Driver       string
	DriverOpts   map[string]string
	External     bool
	Labels       map[string]string
	IpamDriver   string
	IpamConfigs  []IpamConfig
	ExtraOptions []string
}

func (n *NixNetwork) Unit() string {
	return fmt.Sprintf("%s-network-%s.service", n.Runtime, n.Name)
}

func (n *NixNetwork) Command() string {
	cmd := fmt.Sprintf("%[1]s network inspect %[2]s || %[1]s network create %[2]s", n.Runtime, n.Name)
	if n.Driver != "" {
		cmd += fmt.Sprintf(" --driver=%s", n.Driver)
	}
	if len(n.DriverOpts) > 0 {
		driverOpts := mapToRepeatedKeyValFlag("--opt", n.DriverOpts)
		cmd += " " + strings.Join(driverOpts, " ")
	}

	if n.IpamDriver != "" {
		cmd += fmt.Sprintf(" --ipam-driver=%s", n.IpamDriver)
	}
	for _, cfg := range n.IpamConfigs {
		if cfg.Subnet != "" {
			cmd += fmt.Sprintf(" --subnet=%s", cfg.Subnet)
		}
		if cfg.IPRange != "" {
			cmd += fmt.Sprintf(" --ip-range=%s", cfg.IPRange)
		}
		if cfg.Gateway != "" {
			cmd += fmt.Sprintf(" --gateway=%s", cfg.Gateway)
		}
		for _, addr := range cfg.AuxAddresses {
			cmd += fmt.Sprintf(` --aux-address="%s"`, addr)
		}
	}

	if len(n.ExtraOptions) > 0 {
		cmd += " " + strings.Join(n.ExtraOptions, " ")
	}

	if len(n.Labels) > 0 {
		labels := mapToRepeatedKeyValFlag("--label", n.Labels)
		cmd += " " + strings.Join(labels, " ")
	}
	return cmd
}

type NixVolume struct {
	Runtime           ContainerRuntime
	Name              string
	Driver            string
	DriverOpts        map[string]string
	External          bool
	Labels            map[string]string
	RemoveOnStop      bool
	RequiresMountsFor []string
}

func (v *NixVolume) Path() string {
	return v.DriverOpts["device"]
}

func (v *NixVolume) Unit() string {
	return fmt.Sprintf("%s-volume-%s.service", v.Runtime, v.Name)
}

func (v *NixVolume) Command() string {
	cmd := fmt.Sprintf("%[1]s volume inspect %[2]s || %[1]s volume create %[2]s", v.Runtime, v.Name)
	if v.Driver != "" {
		cmd += fmt.Sprintf(" --driver=%s", v.Driver)
	}
	if len(v.DriverOpts) > 0 {
		driverOpts := mapToRepeatedKeyValFlag("--opt", v.DriverOpts)
		cmd += " " + strings.Join(driverOpts, " ")
	}
	if len(v.Labels) > 0 {
		labels := mapToRepeatedKeyValFlag("--label", v.Labels)
		cmd += " " + strings.Join(labels, " ")
	}
	return cmd
}

// NixContainerSystemdConfig configures the container's systemd config.
// In particular, this allows control of the container restart policy through systemd
// service and unit configs.
//
// Each key-value pair in a map represents a systemd key and its value (e.g., Restart=always).
// Users can provide custom config keys by setting the compose2nix.systemd.* label on the service.
type NixContainerSystemdConfig struct {
	Service ServiceConfig
	Unit    UnitConfig
	// NixOS treats these differently, probably to fix the rename issue in
	// earlier systemd versions.
	// See: https://unix.stackexchange.com/a/464098
	StartLimitBurst *int
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
	Command       []string
	AutoStart     bool
}

func (c *NixContainer) Unit() string {
	return fmt.Sprintf("%s-%s.service", c.Runtime, c.Name)
}

// https://docs.docker.com/reference/compose-file/services/#pull_policy
// https://docs.podman.io/en/latest/markdown/podman-build.1.html#pull-policy
type ServicePullPolicy int

const (
	ServicePullPolicyInvalid ServicePullPolicy = iota
	ServicePullPolicyAlways
	ServicePullPolicyNever
	ServicePullPolicyMissing
	ServicePullPolicyBuild
	ServicePullPolicyUnset
)

func NewServicePullPolicy(s string) ServicePullPolicy {
	switch strings.TrimSpace(s) {
	case "always":
		return ServicePullPolicyAlways
	case "never":
		return ServicePullPolicyNever
	case "missing", "if_not_present":
		return ServicePullPolicyMissing
	case "build":
		return ServicePullPolicyBuild
	default:
		return ServicePullPolicyUnset
	}
}

// https://docs.docker.com/reference/compose-file/build/
// https://docs.docker.com/reference/cli/docker/buildx/build/
type NixBuild struct {
	Runtime       ContainerRuntime
	Context       string
	PullPolicy    ServicePullPolicy
	IsGitRepo     bool
	Args          map[string]*string
	Tags          []string
	Dockerfile    string // Relative to context path.
	ContainerName string // Name of the resolved Nix container.
}

func (b *NixBuild) UnitName() string {
	return fmt.Sprintf("%s-build-%s", b.Runtime, b.ContainerName)
}

func (b *NixBuild) Unit() string {
	return b.UnitName() + ".service"
}

func (b *NixBuild) Command() string {
	cmd := fmt.Sprintf("%s build", b.Runtime)

	for _, tag := range b.Tags {
		cmd += fmt.Sprintf(" -t %s", tag)
	}
	for name, arg := range b.Args {
		if arg != nil {
			cmd += fmt.Sprintf(" --build-arg %s=%s", name, *arg)
		} else {
			cmd += fmt.Sprintf(" --build-arg %s", name)
		}
	}
	if b.Dockerfile != "" && b.Dockerfile != "Dockerfile" {
		cmd += fmt.Sprintf(" -f %s", b.Dockerfile)
	}

	if b.IsGitRepo {
		cmd += " " + b.Context
	} else {
		cmd += " ."
	}

	return cmd
}

type NixContainerConfig struct {
	Version          string
	Project          *Project
	Runtime          ContainerRuntime
	Containers       []*NixContainer
	Builds           []*NixBuild
	Networks         []*NixNetwork
	Volumes          []*NixVolume
	CreateRootTarget bool
	WriteNixSetup    bool
	AutoFormat       bool
	AutoStart        bool
	IncludeBuild     bool
	Option           string
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

// Write writes out the Nix config to the provided Writer.
//
// If the AutoFormat option on this struct is set to "true", this method will
// attempt to format the Nix config by calling "nixfmt" and passing in the
// fully built config via stdin.
func (c *NixContainerConfig) Write(out io.Writer) error {
	config := []byte(c.String())

	if c.AutoFormat {
		formatted, err := formatNixCode(config)
		if err != nil {
			return err
		}
		config = formatted
	}

	if _, err := out.Write(config); err != nil {
		return fmt.Errorf("failed to write Nix code: %w", err)
	}

	return nil
}

func rootTarget(runtime ContainerRuntime, project *Project) string {
	return fmt.Sprintf("%s-compose-%s", runtime, project.With("root"))
}

func (c *NixContainerConfig) rootTargetTemplateFunc() string {
	if !c.CreateRootTarget {
		return ""
	}
	return rootTarget(c.Runtime, c.Project)
}

func (c *NixContainerConfig) configTemplateFunc() *NixContainerConfig {
	return c
}
