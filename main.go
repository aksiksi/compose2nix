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

func readEnvFiles(envFilePaths []string, mergeEnv bool) (env []string, _ error) {
	for _, p := range envFilePaths {
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

	if mergeEnv {
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

	fmt.Printf("networks: %v\n", project.Networks)
	fmt.Printf("volumes: %v\n", project.Volumes)
}
