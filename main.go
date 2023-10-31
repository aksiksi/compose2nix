package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

var paths = flag.String("paths", "", "paths to Compose files")
var readEnv = flag.Bool("use_env", true, "whether or not to read the env")
var envFiles = flag.String("env_files", "", "paths to .env files")

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
	indent := strings.Repeat(" ", depth*2)
	for k, v := range elems {
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
	indent := strings.Repeat(" ", depth*2)
	for k, v := range elems {
		var s string
		if v != nil {
			s = *v
		}
		indent := strings.Repeat(" ", depth*2)
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

func (n *NixContainer) ToNix() string {
	s := strings.Builder{}
	s.WriteString(fmt.Sprintf("virtualisation.oci-containers.containers.%s = {\n", n.Name))
	s.WriteString(fmt.Sprintf("  image = %q;\n", n.Image))
	if len(n.Environment) > 0 {
		s.WriteString(fmt.Sprintf("  environment = %s;\n", toNixAttributesNil(n.Environment, 2, false)))
	}
	if len(n.Labels) > 0 {
		s.WriteString(fmt.Sprintf("  labels = %s;\n", toNixAttributes(n.Labels, 2, true)))
	}
	if len(n.Volumes) > 0 {
		s.WriteString(fmt.Sprintf("  volumes = %s;\n", toNixList(n.Volumes, 2)))
	}
	if n.User != "" {
		s.WriteString(fmt.Sprintf("  user = %q;\n", n.User))
	}
	// TODO(aksiksi): Extra options and ports.
	return s.String()
}

func NixContainerFromService(service *types.ServiceConfig) *NixContainer {
	n := &NixContainer{
		Name:        service.Name,
		Image:       service.Image,
		Environment: service.Environment,
		Labels:      service.Labels,
		User:        service.User,
		// TODO(aksiksi): Extra options and ports.
	}
	for _, v := range service.Volumes {
		n.Volumes = append(n.Volumes, v.String())
	}
	return n
}

func findService(services types.Services, name string) *types.ServiceConfig {
	for i, s := range services {
		if s.Name == name {
			return &services[i]
		}
	}
	return nil
}

func readEnvFiles(envFiles []string, mergeWithEnv bool) (env []string, _ error) {
	for _, p := range envFiles {
		if strings.TrimSpace(p) == "" {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("path %s does not exist", p)
				continue
			}
			return nil, fmt.Errorf("failed to stat file: %w", err)
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := s.Text()
			env = append(env, line)
		}
	}

	if mergeWithEnv {
		env = append(env, os.Environ()...)
	}

	return env, nil

}

func main() {
	flag.Parse()

	ctx := context.Background()

	if *paths == "" {
		log.Fatalf("one or more paths must be specified")
	}

	paths := strings.Split(*paths, ",")
	envFiles := strings.Split(*envFiles, ",")
	env, err := readEnvFiles(envFiles, *readEnv)
	if err != nil {
		log.Fatal(err)
	}

	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		ConfigFiles: types.ToConfigFiles(paths),
		Environment: types.NewMapping(env),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("name: %s, working_dir: %s\n", project.Name, project.WorkingDir)
	n := NixContainerFromService(findService(project.Services, "radarr"))
	fmt.Printf("nix container: %+v\n", n)
	fmt.Printf("%s", n.ToNix())
	//fmt.Printf("service: %+v\n", findService(project.Services, "radarr"))
	//fmt.Printf("networks: %v\n", project.Networks)
	//fmt.Printf("volumes: %v\n", project.Volumes)
}
