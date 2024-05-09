package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	// LINT.OnChange(version)
	appVersion = "0.1.9"
	// LINT.ThenChange(flake.nix:version,README.md:version)
)

// TODO(aksiksi): Investigate parsing flags into structs using the *Val functions.
var inputs = flag.String("inputs", "docker-compose.yml", "one or more comma-separated path(s) to Compose file(s).")
var envFiles = flag.String("env_files", "", "one or more comma-separated paths to .env file(s).")
var includeEnvFiles = flag.Bool("include_env_files", false, "include env files in the NixOS container definition.")
var envFilesOnly = flag.Bool("env_files_only", false, "only use env file(s) in the NixOS container definitions.")
var ignoreMissingEnvFiles = flag.Bool("ignore_missing_env_files", false, "if set, missing env files will be ignored.")
var output = flag.String("output", "docker-compose.nix", "path to output Nix file.")
var project = flag.String("project", "", "project name used as a prefix for generated resources. this overrides any top-level \"name\" set in the Compose file(s).")
var serviceInclude = flag.String("service_include", "", "regex pattern for services to include.")
var autoStart = flag.Bool("auto_start", true, "auto-start setting for generated service(s). this applies to all services, not just containers.")
var runtime = flag.String("runtime", "podman", `one of: ["podman", "docker"].`)
var useComposeLogDriver = flag.Bool("use_compose_log_driver", false, "if set, always use the Docker Compose log driver.")
var generateUnusedResources = flag.Bool("generate_unused_resources", false, "if set, unused resources (e.g., networks) will be generated even if no containers use them.")
var checkSystemdMounts = flag.Bool("check_systemd_mounts", false, "if set, volume paths will be checked against systemd mount paths on the current machine and marked as container dependencies.")
var useUpheldBy = flag.Bool("use_upheld_by", false, "if set, upheldBy will be used for service dependencies (NixOS 24.05+).")
var removeVolumes = flag.Bool("remove_volumes", false, "if set, volumes will be removed on systemd service stop.")
var createRootTarget = flag.Bool("create_root_target", true, "if set, a root systemd target will be created, which when stopped tears down all resources.")
var defaultStopTimeout = flag.Duration("default_stop_timeout", defaultSystemdStopTimeout, "default stop timeout for generated container services.")
var writeNixSetup = flag.Bool("write_nix_setup", true, "if true, Nix setup code is written to output (runtime, DNS, autoprune, etc.)")
var version = flag.Bool("version", false, "display version and exit")

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("compose2nix v%s\n", appVersion)
		return
	}

	ctx := context.Background()

	if strings.TrimSpace(*inputs) == "" {
		log.Fatalf("One or more paths must be specified")
	}

	inputs := strings.Split(*inputs, ",")
	envFiles := strings.Split(*envFiles, ",")

	var containerRuntime ContainerRuntime
	if *runtime == "podman" {
		containerRuntime = ContainerRuntimePodman
	} else if *runtime == "docker" {
		containerRuntime = ContainerRuntimeDocker
	} else {
		log.Fatalf("Invalid --runtime: %q", *runtime)
	}

	var serviceIncludeRegexp *regexp.Regexp
	if *serviceInclude != "" {
		pat, err := regexp.Compile(*serviceInclude)
		if err != nil {
			log.Fatalf("Failed to parse -service_include pattern %q: %v", *serviceInclude, err)
		}
		serviceIncludeRegexp = pat
	}

	start := time.Now()
	g := Generator{
		Project:                 NewProject(*project),
		Runtime:                 containerRuntime,
		Inputs:                  inputs,
		EnvFiles:                envFiles,
		IncludeEnvFiles:         *includeEnvFiles,
		EnvFilesOnly:            *envFilesOnly,
		IgnoreMissingEnvFiles:   *ignoreMissingEnvFiles,
		ServiceInclude:          serviceIncludeRegexp,
		AutoStart:               *autoStart,
		UseComposeLogDriver:     *useComposeLogDriver,
		GenerateUnusedResources: *generateUnusedResources,
		CheckSystemdMounts:      *checkSystemdMounts,
		UseUpheldBy:             *useUpheldBy,
		RemoveVolumes:           *removeVolumes,
		NoCreateRootTarget:      !*createRootTarget,
		WriteHeader:             true,
		NoWriteNixSetup:         !*writeNixSetup,
		DefaultStopTimeout:      *defaultStopTimeout,
	}
	containerConfig, err := g.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Generated NixOS config in %v\n", time.Since(start))

	if *output != "" {
		dir := path.Dir(*output)
		if _, err := os.Stat(dir); err != nil {
			log.Fatalf("Directory %q does not exist", dir)
		}
		if err := os.WriteFile(*output, []byte(containerConfig.String()), 0644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Wrote NixOS config to %s\n", *output)
	}
}
