package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/joho/godotenv"
)

// mapToKeyValArray converts a map into a _sorted_ list of KEY=VAL entries.
func mapToKeyValArray(m map[string]string) []string {
	var arr []string
	for k, v := range m {
		arr = append(arr, fmt.Sprintf("%s=%s", k, v))
	}
	slices.Sort(arr)
	return arr
}

func mapToRepeatedKeyValFlag(flagName string, m map[string]string) []string {
	arr := mapToKeyValArray(m)
	for i, v := range arr {
		arr[i] = fmt.Sprintf("%s=%s", flagName, v)
	}
	return arr
}

// ReadEnvFiles reads the given set of env files into a list of KEY=VAL entries.
//
// If mergeWithEnv is set, the running env is merged with the provided env files. Any
// duplicate variables will be overridden by the running env.
//
// If ignoreMissing is set, any missing env files will be ignored. This is useful for cases
// where an env file is not available during conversion to Nix.
func ReadEnvFiles(envFiles []string, mergeWithEnv, ignoreMissing bool) (env []string, _ error) {
	for _, p := range envFiles {
		if strings.TrimSpace(p) == "" {
			continue
		}
		envMap, err := godotenv.Read(p)
		if err != nil {
			if ignoreMissing && errors.Is(err, os.ErrNotExist) {
				log.Printf("Ignoring missing env file %q...", p)
				continue
			}
			return nil, fmt.Errorf("failed to parse env file %q: %w", p, err)
		}
		for k, v := range envMap {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if mergeWithEnv {
		env = append(env, os.Environ()...)
	}

	return env, nil
}

// formatNixCode will format Nix code by calling 'nixfmt' and passing in the
// given code via stdin.
func formatNixCode(contents []byte) ([]byte, error) {
	// Check for existence of 'nixfmt' in $PATH.
	nixfmtPath, err := exec.LookPath("nixfmt")
	if err != nil {
		return nil, fmt.Errorf("'nixfmt' not found in $PATH: %w", err)
	}

	cmd := exec.Command(nixfmtPath)
	cmd.Stdin = bytes.NewBuffer(contents)

	// Overwrite contents with formatted output.
	contents, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run 'nixfmt' on contents: %w", err)
	}

	return contents, nil
}
