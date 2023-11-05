package nixose

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
		Paths:    []string{composePath},
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
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}

func TestPodman(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath, outFilePath := getPaths(t)
	g := Generator{
		Runtime:  ContainerRuntimePodman,
		Paths:    []string{composePath},
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
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output diff: %s\n", diff)
	}
}
