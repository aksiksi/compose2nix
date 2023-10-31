package compose2nixos

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
)

func FindService(services types.Services, name string) *types.ServiceConfig {
	for i, s := range services {
		if s.Name == name {
			return &services[i]
		}
	}
	return nil
}

func ReadEnvFiles(envFiles []string, mergeWithEnv bool) (env []string, _ error) {
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
