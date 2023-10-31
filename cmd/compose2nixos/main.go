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
var readEnv = flag.Bool("use_env", true, "whether or not to read the env")
var envFiles = flag.String("env_files", "", "paths to .env files")
var output = flag.String("output", "", "path to output Nix file")
var namePrefix = flag.String("name_prefix", "", "prefix for generated container names")

func main() {
	flag.Parse()

	ctx := context.Background()

	if *paths == "" {
		log.Fatalf("one or more paths must be specified")
	}

	paths := strings.Split(*paths, ",")
	envFiles := strings.Split(*envFiles, ",")

	containers, err := compose2nixos.ParseWithEnv(ctx, paths, *namePrefix, envFiles, *readEnv)
	if err != nil {
		log.Fatal(err)
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(containers.ToNix()), os.FileMode(0644)); err != nil {
			log.Fatal(err)
		}
	}
}
