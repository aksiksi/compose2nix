package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
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
		f, err := os.Open(p)
		if err != nil {
			if ignoreMissing {
				log.Printf("Ignoring mising env file %s...", p)
				continue
			}
			return nil, fmt.Errorf("failed to open file %s: %w", p, err)
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

// formatNixCode will format Nix code by calling 'nixfmt' and passing in the
// given code via stdin.
func formatNixCode(contents []byte) ([]byte, error) {
	// Check for existence of 'nixfmt' in $PATH.
	nixfmtPath, err := exec.LookPath("nixfmt")
	if err != nil {
		return nil, fmt.Errorf("'nixfmt' not found in $PATH: %w", err)
	}

	cmd := exec.Command(nixfmtPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to setup stdin pipe: %w", err)
	}
	if err := func() error {
		defer stdin.Close()
		if _, err := stdin.Write(contents); err != nil {
			return fmt.Errorf("failed to write content to stdin: %w", err)
		}
		return nil
	}(); err != nil {
		return nil, err
	}

	// Overwrite contents with formatted output.
	contents, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run 'nixfmt' on contents: %w", err)
	}

	return contents, nil
}
