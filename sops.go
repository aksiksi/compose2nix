package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type SopsConfig struct {
	FilePath string
	secrets  map[string]bool
}

func NewSopsConfig(filePath string) *SopsConfig {
	return &SopsConfig{
		FilePath: filePath,
		secrets:  make(map[string]bool),
	}
}

func (s *SopsConfig) LoadSecrets() error {
	if strings.TrimSpace(s.FilePath) == "" {
		return fmt.Errorf("sops file path is empty")
	}

	content, err := os.ReadFile(s.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read sops file %q: %w", s.FilePath, err)
	}

	var yamlContent map[string]any
	if err := yaml.Unmarshal(content, &yamlContent); err != nil {
		return fmt.Errorf("failed to parse sops YAML %q: %w", s.FilePath, err)
	}

	s.extractSecrets(yamlContent, "")

	return nil
}

func (s *SopsConfig) extractSecrets(data map[string]any, prefix string) {
	for key, value := range data {
		if key == "sops" {
			continue
		}

		fullKey := key
		if prefix != "" {
			fullKey = prefix + "/" + key
		}

		switch v := value.(type) {
		case map[string]any:
			s.extractSecrets(v, fullKey)
		default:
			s.secrets[fullKey] = true
		}
	}
}

func (s *SopsConfig) HasSecret(name string) bool {
	if s.secrets == nil {
		return false
	}
	return s.secrets[name]
}
