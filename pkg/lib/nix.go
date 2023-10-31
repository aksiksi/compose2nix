package compose2nixos

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/exp/maps"
)

const nixContainerOption = "virtualisation.oci-containers.containers"

func toNixList(elems []string, depth int) string {
	b := strings.Builder{}
	indent := strings.Repeat(" ", depth*2)
	b.WriteString("[\n")
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

func toNixAttributesNil(elems map[string]*string, depth int, quoteKeys bool) string {
	b := strings.Builder{}
	b.WriteString("{\n")

	// Sort keys for stability.
	keys := maps.Keys(elems)
	slices.Sort(keys)
	indent := strings.Repeat(" ", depth*2)
	for _, k := range keys {
		v := elems[k]
		var s string
		if v != nil {
			s = *v
		}
		if !quoteKeys {
			b.WriteString(fmt.Sprintf("%s%s = %q;\n", indent, k, s))
		} else {
			b.WriteString(fmt.Sprintf("%s%q = %q;\n", indent, k, s))
		}
	}
	b.WriteString(fmt.Sprintf("%s}", indent[:len(indent)-2]))
	return b.String()
}

// https://search.nixos.org/options?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=oci-container
type NixContainer struct {
	Name         string
	Image        string
	Environment  map[string]*string
	Ports        []string
	Labels       map[string]string
	Volumes      []string
	User         string
	ExtraOptions []string
}

func (n *NixContainer) ToNix(depth int) string {
	s := strings.Builder{}
	indent := strings.Repeat(" ", depth*2)
	s.WriteString(fmt.Sprintf("%s%s = {\n", indent, n.Name))
	s.WriteString(fmt.Sprintf("%s  image = %q;\n", indent, n.Image))
	if len(n.Environment) > 0 {
		s.WriteString(fmt.Sprintf("%s  environment = %s;\n", indent, toNixAttributesNil(n.Environment, depth+2, false)))
	}
	if len(n.Labels) > 0 {
		s.WriteString(fmt.Sprintf("%s  labels = %s;\n", indent, toNixAttributes(n.Labels, depth+2, true)))
	}
	if len(n.Volumes) > 0 {
		s.WriteString(fmt.Sprintf("%s  volumes = %s;\n", indent, toNixList(n.Volumes, depth+2)))
	}
	if len(n.Ports) > 0 {
		s.WriteString(fmt.Sprintf("%s  ports = %s;\n", indent, toNixList(n.Ports, depth+2)))
	}
	if n.User != "" {
		s.WriteString(fmt.Sprintf("%s  user = %q;\n", indent, n.User))
	}
	s.WriteString(fmt.Sprintf("%s};\n", indent))
	// TODO(aksiksi): Extra options.
	return s.String()
}

type NixContainers []*NixContainer

func (n NixContainers) ToNix() string {
	s := strings.Builder{}
	s.WriteString("{ pkgs, ... }:\n\n")
	s.WriteString("{\n")
	s.WriteString(fmt.Sprintf("  %s = {\n", nixContainerOption))
	for _, c := range n {
		s.WriteString(c.ToNix(2))
	}
	s.WriteString("  };\n")
	s.WriteString("}\n")
	return s.String()
}
