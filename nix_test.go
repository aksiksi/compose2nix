package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

func getPaths(t *testing.T, useCommonInput bool) (string, string) {
	t.Helper()
	var composePath string
	if useCommonInput {
		composePath = path.Join("testdata", "compose.yml")
	} else {
		composePath = path.Join("testdata", fmt.Sprintf("%s.compose.yml", t.Name()))
	}
	envFilePath := path.Join("testdata", "input.env")
	return composePath, envFilePath
}

func runSubtestsWithGenerator(t *testing.T, g *Generator) {
	t.Helper()
	ctx := context.Background()
	for _, runtime := range []ContainerRuntime{ContainerRuntimeDocker, ContainerRuntimePodman} {
		t.Run(runtime.String(), func(t *testing.T) {
			testName := strings.ReplaceAll(t.Name(), "/", ".")
			outFilePath := path.Join("testdata", fmt.Sprintf("%s.nix", testName))
			g.Runtime = runtime
			c, err := g.Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			got := c.String()
			if *update {
				if err := os.WriteFile(outFilePath, []byte(got), 0644); err != nil {
					t.Fatal(err)
				}
				return
			}
			wantOutput, err := os.ReadFile(outFilePath)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(wantOutput), got); diff != "" {
				t.Errorf("output diff: %s\n", diff)
			}
		})
	}
}

func TestBasic(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:                 []string{composePath},
		EnvFiles:               []string{envFilePath},
		AutoStart:              true,
		GenerateUnusedResoures: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestProject(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Project:  NewProject("myproject"),
		Inputs:   []string{composePath},
		EnvFiles: []string{envFilePath},
	}
	runSubtestsWithGenerator(t, g)
}

func TestUnusedResources(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Project:            NewProject("myproject"),
		Inputs:             []string{composePath},
		NoCreateRootTarget: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestSystemdMount(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:             []string{composePath},
		EnvFiles:           []string{envFilePath},
		CheckSystemdMounts: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestRemoveVolumes(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:        []string{composePath},
		EnvFiles:      []string{envFilePath},
		RemoveVolumes: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestEnvFilesOnly(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:          []string{composePath},
		EnvFiles:        []string{envFilePath},
		IncludeEnvFiles: true,
		EnvFilesOnly:    true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestIgnoreMissingEnvFiles(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath := getPaths(t, true)
	g := Generator{
		Runtime:               ContainerRuntimeDocker,
		Inputs:                []string{composePath},
		EnvFiles:              []string{path.Join(t.TempDir(), "bad-path"), envFilePath},
		IncludeEnvFiles:       true,
		EnvFilesOnly:          true,
		IgnoreMissingEnvFiles: true,
	}
	if _, err := g.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOverrideSystemdStopTimeout(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:             []string{composePath},
		EnvFiles:           []string{envFilePath},
		DefaultStopTimeout: 10 * time.Second,
	}
	runSubtestsWithGenerator(t, g)
}

func TestNoWriteNixSetup(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:          []string{composePath},
		EnvFiles:        []string{envFilePath},
		NoWriteNixSetup: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestMacvlanSupport(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs: []string{composePath},
	}
	runSubtestsWithGenerator(t, g)
}

func TestMultipleNetworks(t *testing.T) {
	// Supported in Docker too.
	// See: https://github.com/moby/moby/issues/35543
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs: []string{composePath},
	}
	runSubtestsWithGenerator(t, g)
}
