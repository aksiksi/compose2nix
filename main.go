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
	appVersion = "0.3.2-pre"
	// LINT.ThenChange(flake.nix:version)
)

type Args struct {
	inputs                  string
	output                  string
	project                 string
	serviceInclude          string
	envFiles                string
	rootPath                string
	includeEnvFiles         bool
	envFilesOnly            bool
	ignoreMissingEnvFiles   bool
	autoStart               bool
	runtime                 string
	useComposeLogDriver     bool
	generateUnusedResources bool
	checkSystemdMounts      bool
	checkBindMounts         bool
	useUpheldBy             bool
	removeVolumes           bool
	createRootTarget        bool
	defaultStopTimeout      time.Duration
	build                   bool
	writeNixSetup           bool
	autoFormat              bool
	optionPrefix            string
	enableOption            bool
	rootlessUser            string
	version                 bool
}

var args = Args{}

func registerFlags() *Args {
	args := Args{}
	flag.StringVar(&args.inputs, "inputs", "docker-compose.yml", "one or more comma-separated path(s) to Compose file(s).")
	flag.StringVar(&args.output, "output", "docker-compose.nix", "path to output Nix file.")
	flag.StringVar(&args.project, "project", "", "project name used as args prefix for generated resources. this overrides any top-level \"name\" set in the Compose file(s).")
	flag.StringVar(&args.serviceInclude, "service_include", "", "regex pattern for services to include.")
	flag.StringVar(&args.envFiles, "env_files", "", "one or more comma-separated paths to .env file(s).")
	flag.StringVar(&args.rootPath, "root_path", "", "absolute path to use as the root for any relative paths in the Compose file (e.g., volumes, env files). defaults to the current working directory.")
	flag.BoolVar(&args.includeEnvFiles, "include_env_files", false, "include env files in the NixOS container definition.")
	flag.BoolVar(&args.envFilesOnly, "env_files_only", false, "only use env file(s) in the NixOS container definitions.")
	flag.BoolVar(&args.ignoreMissingEnvFiles, "ignore_missing_env_files", false, "if set, missing env files will be ignored.")
	flag.BoolVar(&args.autoStart, "auto_start", true, "auto-start setting for generated service(s). this applies to all services, not just containers.")
	flag.StringVar(&args.runtime, "runtime", "podman", `one of: ["podman", "docker"].`)
	flag.BoolVar(&args.useComposeLogDriver, "use_compose_log_driver", false, "if set, always use the Docker Compose log driver.")
	flag.BoolVar(&args.generateUnusedResources, "generate_unused_resources", false, "if set, unused resources (e.g., networks) will be generated even if no containers use them.")
	flag.BoolVar(&args.checkSystemdMounts, "check_systemd_mounts", false, "if set, volume paths will be checked against systemd mount paths on the current machine and marked as container dependencies.")
	flag.BoolVar(&args.checkBindMounts, "check_bind_mounts", false, "if set, check that bind mount paths exist. this is useful if running the generated Nix code on the same machine.")
	flag.BoolVar(&args.useUpheldBy, "use_upheld_by", false, "if set, upheldBy will be used for service dependencies (NixOS 24.05+).")
	flag.BoolVar(&args.removeVolumes, "remove_volumes", false, "if set, volumes will be removed on systemd service stop.")
	flag.BoolVar(&args.createRootTarget, "create_root_target", true, "if set, args root systemd target will be created, which when stopped tears down all resources.")
	flag.DurationVar(&args.defaultStopTimeout, "default_stop_timeout", defaultSystemdStopTimeout, "default stop timeout for generated container services.")
	flag.BoolVar(&args.build, "build", false, "if set, generated container build systemd services will be enabled.")
	flag.BoolVar(&args.writeNixSetup, "write_nix_setup", true, "if true, Nix setup code is written to output (runtime, DNS, autoprune, etc.)")
	flag.BoolVar(&args.autoFormat, "auto_format", false, `if true, Nix output will be formatted using "nixfmt" (must be present in $PATH).`)
	flag.StringVar(&args.optionPrefix, "option_prefix", "", "Prefix for the option. If empty, the project name will be used as the option name. (e.g. custom.containers)")
	flag.BoolVar(&args.enableOption, "enable_option", false, "generate args NixOS module option. this allows you to enable or disable the generated module from within your NixOS config. by default, the option will be named \"options.[project_name]\", but you can add args prefix using the \"option_prefix\" flag.")
	flag.StringVar(&args.rootlessUser, "rootless_user", "", "run all generated NixOS containers in rootless mode as this user. only supported with Podman.")
	flag.BoolVar(&args.version, "version", false, "display version and exit")
	return &args
}

type OsGetWd struct{}

func (*OsGetWd) GetWd() (string, error) {
	return os.Getwd()
}

func main() {
	args := registerFlags()

	flag.Parse()

	if args.version {
		fmt.Printf("compose2nix v%s\n", appVersion)
		return
	}
	if args.output == "" {
		log.Fatal("No output path specified.")
	}

	ctx := context.Background()

	if strings.TrimSpace(args.inputs) == "" {
		log.Fatalf("One or more paths must be specified")
	}

	inputs := strings.Split(args.inputs, ",")

	var envFilesList []string
	if args.envFiles != "" {
		envFilesList = strings.Split(args.envFiles, ",")
	}

	var containerRuntime ContainerRuntime
	if args.runtime == "podman" {
		containerRuntime = ContainerRuntimePodman
	} else if args.runtime == "docker" {
		containerRuntime = ContainerRuntimeDocker
	} else {
		log.Fatalf("Invalid --runtime: %q", args.runtime)
	}

	if args.rootlessUser != "" && containerRuntime != ContainerRuntimePodman {
		log.Fatal("Rootless mode only supported with Podman runtime")
	}

	var serviceIncludeRegexp *regexp.Regexp
	if args.serviceInclude != "" {
		pat, err := regexp.Compile(args.serviceInclude)
		if err != nil {
			log.Fatalf("Failed to parse -service_include pattern %q: %v", args.serviceInclude, err)
		}
		serviceIncludeRegexp = pat
	}

	start := time.Now()
	g := Generator{
		Project:                 NewProject(args.project),
		Runtime:                 containerRuntime,
		Inputs:                  inputs,
		EnvFiles:                envFilesList,
		RootPath:                args.rootPath,
		IncludeEnvFiles:         args.includeEnvFiles,
		EnvFilesOnly:            args.envFilesOnly,
		IgnoreMissingEnvFiles:   args.ignoreMissingEnvFiles,
		ServiceInclude:          serviceIncludeRegexp,
		AutoStart:               args.autoStart,
		UseComposeLogDriver:     args.useComposeLogDriver,
		GenerateUnusedResources: args.generateUnusedResources,
		CheckSystemdMounts:      args.checkSystemdMounts,
		CheckBindMounts:         args.checkBindMounts,
		UseUpheldBy:             args.useUpheldBy,
		RemoveVolumes:           args.removeVolumes,
		NoCreateRootTarget:      !args.createRootTarget,
		WriteHeader:             true,
		NoWriteNixSetup:         !args.writeNixSetup,
		AutoFormat:              args.autoFormat,
		DefaultStopTimeout:      args.defaultStopTimeout,
		IncludeBuild:            args.build,
		GetWorkingDir:           &OsGetWd{},
		OptionPrefix:            args.optionPrefix,
		RootlessUser:            args.rootlessUser,
		EnableOption:            args.enableOption,
	}
	containerConfig, err := g.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Generated NixOS config in %v\n", time.Since(start))

	dir := path.Dir(args.output)
	if _, err := os.Stat(dir); err != nil {
		log.Fatalf("Directory %q does not exist: %v", dir, err)
	}
	f, err := os.Create(args.output)
	if err != nil {
		log.Fatalf("Failed to create file %q: %v", args.output, err)
	}
	if err := containerConfig.Write(f); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Wrote NixOS config to %s\n", args.output)
}
