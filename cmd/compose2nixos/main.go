package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"

	compose2nixos "github.com/aksiksi/compose2nixos/pkg/lib"
)

var paths = flag.String("paths", "", "paths to Compose files")
var envFiles = flag.String("env_files", "", "paths to .env files")
var envFilesOnly = flag.Bool("env_files_only", false, "only use env files in the NixOS container definitions")
var output = flag.String("output", "", "path to output Nix file")
var project = flag.String("project", "", "project name used as a prefix for generated resources")
var autoStart = flag.Bool("auto_start", true, "control auto-start setting for containers")

func main() {
	flag.Parse()

	ctx := context.Background()

	if *paths == "" {
		log.Fatalf("one or more paths must be specified")
	}

	paths := strings.Split(*paths, ",")
	envFiles := strings.Split(*envFiles, ",")

	containers, err := compose2nixos.ParseWithEnv(ctx, paths, *project, *autoStart, envFiles, *envFilesOnly)
	if err != nil {
		log.Fatal(err)
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(containers.ToNix()), os.FileMode(0644)); err != nil {
			log.Fatal(err)
		}
	}
}
