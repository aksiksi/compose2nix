package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
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

type testGetWd struct {
	workingDir string
}

func (t *testGetWd) GetWd() (string, error) {
	return t.workingDir, nil
}

func runSubtestsWithGenerator(t *testing.T, g *Generator) {
	t.Helper()
	ctx := context.Background()

	if g.RootPath == "" && g.GetWorkingDir == nil {
		// Set root path to current directory so we can use relative paths
		// in tests. We cannot use cwd() here because test output cannot encode
		// absolute paths.
		g.RootPath = "."
	}

	for _, runtime := range []ContainerRuntime{ContainerRuntimeDocker, ContainerRuntimePodman} {
		t.Run(runtime.String(), func(t *testing.T) {
			testName := strings.ReplaceAll(t.Name(), "/", ".")
			outFilePath := path.Join("testdata", fmt.Sprintf("%s.nix", testName))
			g.Runtime = runtime
			c, err := g.Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			gotBuf := new(bytes.Buffer)
			if err := c.Write(gotBuf); err != nil {
				t.Fatal(err)
			}
			got := gotBuf.String()
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
		Inputs:                  []string{composePath},
		EnvFiles:                []string{envFilePath},
		AutoStart:               true,
		GenerateUnusedResources: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestBasicAutoFormat(t *testing.T) {
	if _, err := exec.LookPath("nixfmt"); err != nil {
		t.Skip()
	}
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:                  []string{composePath},
		EnvFiles:                []string{envFilePath},
		AutoStart:               true,
		GenerateUnusedResources: true,
		AutoFormat:              true,
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

func TestEnableOption(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Project:      NewProject("myproject"),
		Inputs:       []string{composePath},
		EnvFiles:     []string{envFilePath},
		EnableOption: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestEnableOption_WithHeader(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Project:      NewProject("myproject"),
		Inputs:       []string{composePath},
		EnvFiles:     []string{envFilePath},
		EnableOption: true,
		WriteHeader:  true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestOptionPrefix(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Project:      NewProject("myproject"),
		Inputs:       []string{composePath},
		EnvFiles:     []string{envFilePath},
		OptionPrefix: "custom.containers",
		EnableOption: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestUnusedResources(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Project: NewProject("myproject"),
		Inputs:  []string{composePath},
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

func TestUpheldBy(t *testing.T) {
	composePath, envFilePath := getPaths(t, true)
	g := &Generator{
		Inputs:      []string{composePath},
		EnvFiles:    []string{envFilePath},
		UseUpheldBy: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestCommandAndEntrypoint(t *testing.T) {
	composePath, envFilePath := getPaths(t, false)
	g := &Generator{
		Project:  NewProject("test"),
		Inputs:   []string{composePath},
		EnvFiles: []string{envFilePath},
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

func TestEnvFiles(t *testing.T) {
	composePath, envFilePath := getPaths(t, false)
	g := &Generator{
		Project:         NewProject("test"),
		Inputs:          []string{composePath},
		EnvFiles:        []string{envFilePath},
		IncludeEnvFiles: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestEnvFilesOnly(t *testing.T) {
	composePath, envFilePath := getPaths(t, false)
	g := &Generator{
		Project:         NewProject("test"),
		Inputs:          []string{composePath},
		EnvFiles:        []string{envFilePath},
		IncludeEnvFiles: true,
		EnvFilesOnly:    true,
	}
	runSubtestsWithGenerator(t, g)
}

// TODO(aksiksi): Clean this test up.
func TestIgnoreMissingEnvFiles(t *testing.T) {
	ctx := context.Background()
	composePath, envFilePath := getPaths(t, true)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	g := &Generator{
		Runtime:               ContainerRuntimeDocker,
		RootPath:              cwd,
		Inputs:                []string{composePath},
		EnvFiles:              []string{path.Join(t.TempDir(), "bad-path"), envFilePath},
		IncludeEnvFiles:       true,
		EnvFilesOnly:          true,
		IgnoreMissingEnvFiles: true,
	}

	if _, err := g.Run(ctx); err != nil {
		t.Error(err)
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

func TestExternalNetworksAndVolumes(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs: []string{composePath},
	}
	runSubtestsWithGenerator(t, g)
}

func TestNetworkAndVolumeNames(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs: []string{composePath},
	}
	runSubtestsWithGenerator(t, g)
}

func TestNetworkSettings(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Project:                 NewProject("test"),
		Inputs:                  []string{composePath},
		GenerateUnusedResources: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestRelativeServiceVolumes(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:   []string{composePath},
		RootPath: "/my/root",
	}
	runSubtestsWithGenerator(t, g)
}

func TestRelativeServiceVolumes_CurrentDirectory(t *testing.T) {
	composePath := path.Join("testdata", "TestRelativeServiceVolumes.compose.yml")
	g := &Generator{
		Inputs:        []string{composePath},
		GetWorkingDir: &testGetWd{"/some/path/"},
	}
	runSubtestsWithGenerator(t, g)
}

func TestNoRestart(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:  []string{composePath},
		Project: NewProject("test"),
	}
	runSubtestsWithGenerator(t, g)
}

// Verifies that we adhere to spec.
// https://github.com/compose-spec/compose-spec/blob/main/spec.md#environment
func TestEmptyEnv(t *testing.T) {
	composePath, _ := getPaths(t, false)

	// Setup an env file that overrides an empty env var.
	p := path.Join(t.TempDir(), "test.env")
	content := "EMPTY_BUT_OVERRIDDEN_BY_ENV_FILE=abcde"
	if err := os.WriteFile(p, []byte(content), 0666); err != nil {
		t.Fatal(err)
	}

	g := &Generator{
		Inputs:   []string{composePath},
		Project:  NewProject("test"),
		EnvFiles: []string{p},
	}
	runSubtestsWithGenerator(t, g)
}

func TestAutoStart(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:  []string{composePath},
		Project: NewProject("test"),
	}
	runSubtestsWithGenerator(t, g)
}

func TestDeployDevices(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:  []string{composePath},
		Project: NewProject("test"),
	}
	runSubtestsWithGenerator(t, g)
}

func TestEscapeChars(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs: []string{composePath},
	}
	runSubtestsWithGenerator(t, g)
}

func TestNoCreateRootTarget(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:             []string{composePath},
		Project:            NewProject("test"),
		NoCreateRootTarget: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestComposeEnvFiles(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:          []string{composePath},
		Project:         NewProject("test"),
		EnvFiles:        []string{"testdata/first.env"},
		IncludeEnvFiles: true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestCheckBindMounts(t *testing.T) {
	ctx := context.Background()
	d := t.TempDir()
	bindMountPath := path.Join(d, "my-bind-mount")

	if err := os.Mkdir(bindMountPath, 0755); err != nil {
		t.Fatal(err)
	}

	composeFile := fmt.Sprintf(`
services:
  my-service:
    image: nginx
    volumes:
      - %s:dest-path
`, bindMountPath)

	composePath := path.Join(d, "compose.yml")
	f, err := os.Create(composePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(composeFile)); err != nil {
		t.Fatal(err)
	}

	g := &Generator{
		Inputs:          []string{composePath},
		Project:         NewProject("test"),
		RootPath:        ".",
		CheckBindMounts: true,
	}

	if _, err := g.Run(ctx); err != nil {
		t.Error(err)
	}
}

func TestBuildSpec(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:   []string{composePath},
		Project:  NewProject("test"),
		RootPath: "/some/path",
	}
	runSubtestsWithGenerator(t, g)
}

func TestBuildSpec_BuildEnabled(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:       []string{composePath},
		Project:      NewProject("test"),
		RootPath:     "/some/path",
		IncludeBuild: true,
		UseUpheldBy:  true,
	}
	runSubtestsWithGenerator(t, g)
}

func TestSopsIntegration(t *testing.T) {
	composePath, _ := getPaths(t, false)
	sopsPath := path.Join("testdata", "sops-example", "secrets", "pinnacle.yaml")

	sopsConfig := NewSopsConfig(sopsPath)
	if err := sopsConfig.LoadSecrets(); err != nil {
		t.Fatalf("Failed to load sops config: %v", err)
	}

	g := &Generator{
		Inputs:     []string{composePath},
		Project:    NewProject("test"),
		RootPath:   ".",
		SopsConfig: sopsConfig,
	}
	runSubtestsWithGenerator(t, g)
}

func TestVolumeSubpath(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:  []string{composePath},
		Project: NewProject("myproject"),
	}
	runSubtestsWithGenerator(t, g)
}

func TestGroupAdd(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:  []string{composePath},
		Project: NewProject("test"),
	}
	runSubtestsWithGenerator(t, g)
}

func TestIpc(t *testing.T) {
	composePath, _ := getPaths(t, false)
	g := &Generator{
		Inputs:  []string{composePath},
		Project: NewProject("test"),
	}
	runSubtestsWithGenerator(t, g)
}
