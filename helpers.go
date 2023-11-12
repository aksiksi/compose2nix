package compose2nix

import (
	"bufio"
	"cmp"
	"fmt"
	"log"
	"os"
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

func removeDuplicates[T cmp.Ordered](s []T) []T {
	if len(s) <= 1 {
		return s
	}
	slices.Sort(s)
	last := 1
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			s[last] = s[i]
			last++
		}
	}
	return s[:last]
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
