package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

type fakeSystemd struct {
	PathToMount map[string]string
}

func (s *fakeSystemd) FindMountForPath(path string) (string, error) {
	for p, m := range s.PathToMount {
		if strings.HasPrefix(path, p) {
			return m, nil
		}
	}
	return "", nil
}

func getPaths(t *testing.T) (string, string, string) {
	outFileName := fmt.Sprintf("%s_out.nix", t.Name())
	composePath := path.Join("testdata", "docker-compose.yml")
	envFilePath := path.Join("testdata", "input.env")
	outFilePath := path.Join("testdata", outFileName)
	return composePath, envFilePath, outFilePath
}

func TestDocker(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath, outFilePath := getPaths(t)
	g := Generator{
		Runtime:  ContainerRuntimeDocker,
		Inputs:   []string{composePath},
		EnvFiles: []string{envFilePath},
	}
	c, err := g.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput, err := os.ReadFile(outFilePath)
	if err != nil {
		t.Fatal(err)
	}
	got, want := c.String(), string(wantOutput)
	if *update {
		if err := os.WriteFile(outFilePath, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}

func TestDocker_WithProject(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath, outFilePath := getPaths(t)
	g := Generator{
		Project:  NewProject("myproject"),
		Runtime:  ContainerRuntimeDocker,
		Inputs:   []string{composePath},
		EnvFiles: []string{envFilePath},
	}
	c, err := g.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput, err := os.ReadFile(outFilePath)
	if err != nil {
		t.Fatal(err)
	}
	got, want := c.String(), string(wantOutput)
	if *update {
		if err := os.WriteFile(outFilePath, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}

func TestPodman(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath, outFilePath := getPaths(t)
	g := Generator{
		Runtime:  ContainerRuntimePodman,
		Inputs:   []string{composePath},
		EnvFiles: []string{envFilePath},
	}
	c, err := g.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput, err := os.ReadFile(outFilePath)
	if err != nil {
		t.Fatal(err)
	}
	got, want := c.String(), string(wantOutput)
	if *update {
		if err := os.WriteFile(outFilePath, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}

func TestPodman_WithProject(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath, outFilePath := getPaths(t)
	g := Generator{
		Project:  NewProject("myproject"),
		Runtime:  ContainerRuntimePodman,
		Inputs:   []string{composePath},
		EnvFiles: []string{envFilePath},
	}
	c, err := g.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput, err := os.ReadFile(outFilePath)
	if err != nil {
		t.Fatal(err)
	}
	got, want := c.String(), string(wantOutput)
	if *update {
		if err := os.WriteFile(outFilePath, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}

func TestUnusedResources(t *testing.T) {
	ctx := context.Background()
	composeFile := strings.TrimSpace("version: \"3.7\"\nnetworks:\n  test:\nvolumes:\n  some-volume:")
	path := path.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte(composeFile), 0644); err != nil {
		t.Fatal(err)
	}

	testcases := []struct {
		runtime ContainerRuntime
		want    string
	}{
		{
			runtime: ContainerRuntimeDocker,
			want: `{ pkgs, ... }:

{
  # Runtime
  virtualisation.docker = {
    enable = true;
    autoPrune.enable = true;
  };
  virtualisation.oci-containers.backend = "docker";
}
`,
		},
		{
			runtime: ContainerRuntimePodman,
			want: `{ pkgs, ... }:

{
  # Runtime
  virtualisation.podman = {
    enable = true;
    autoPrune.enable = true;
    dockerCompat = true;
    defaultNetwork.settings = {
      # Required for container networking to be able to use names.
      dns_enabled = true;
    };
  };
  virtualisation.oci-containers.backend = "podman";
}
`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.runtime.String(), func(t *testing.T) {
			g := Generator{
				Runtime: tc.runtime,
				Inputs:  []string{path},
			}
			c, err := g.Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, c.String()); diff != "" {
				t.Errorf("output diff: %s\n", diff)
			}
		})
	}
}

func TestDocker_SystemdMount(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath, outFilePath := getPaths(t)
	fakeSystemd := &fakeSystemd{
		PathToMount: map[string]string{
			"/mnt/media":  "mnt-media.mount",
			"/mnt/photos": "mnt-photos.mount",
		},
	}
	g := Generator{
		Runtime:            ContainerRuntimeDocker,
		Inputs:             []string{composePath},
		EnvFiles:           []string{envFilePath},
		SystemdProvider:    fakeSystemd,
		CheckSystemdMounts: true,
	}
	c, err := g.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput, err := os.ReadFile(outFilePath)
	if err != nil {
		t.Fatal(err)
	}
	got, want := c.String(), string(wantOutput)
	if *update {
		if err := os.WriteFile(outFilePath, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	} else if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}
