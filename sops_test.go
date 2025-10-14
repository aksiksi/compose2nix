package main

import (
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var sortSlicesOpt = cmpopts.SortSlices(func(a, b string) bool {
	return a < b
})

// Test-only helper method
func (s *SopsConfig) getAvailableSecrets() []string {
	var secrets []string
	for secret := range s.secrets {
		secrets = append(secrets, secret)
	}
	return secrets
}

func TestSopsConfig_LoadSecrets(t *testing.T) {
	sopsPath := path.Join("testdata", "sops-example", "secrets", "pinnacle.yaml")
	sopsConfig := NewSopsConfig(sopsPath)

	err := sopsConfig.LoadSecrets()
	if err != nil {
		t.Fatalf("Failed to load secrets: %v", err)
	}

	want := []string{"example.env", "folder/example-2.env"}
	for _, secret := range want {
		if !sopsConfig.HasSecret(secret) {
			t.Errorf("Expected secret %q not found", secret)
		}
	}

	got := sopsConfig.getAvailableSecrets()
	if diff := cmp.Diff(got, want, sortSlicesOpt); diff != "" {
		t.Errorf("diff in secrets (-want,+got)\n%s", diff)
	}
}

func TestParseCommaSeparatedSecrets(t *testing.T) {
	sopsPath := path.Join("testdata", "sops-example", "secrets", "pinnacle.yaml")
	sopsConfig := NewSopsConfig(sopsPath)

	err := sopsConfig.LoadSecrets()
	if err != nil {
		t.Fatalf("Failed to load secrets: %v", err)
	}

	// Test with mock container
	c := &NixContainer{
		Labels: map[string]string{
			"compose2nix.settings.sops.secrets": "example.env,folder/example-2.env",
		},
		SopsSecrets: []string{},
	}

	err = parseNixContainerLabels(c, sopsConfig)
	if err != nil {
		t.Fatalf("Failed to parse labels: %v", err)
	}

	expectedSecrets := []string{"example.env", "folder/example-2.env"}
	if len(c.SopsSecrets) != len(expectedSecrets) {
		t.Fatalf("Expected %d secrets, got %d", len(expectedSecrets), len(c.SopsSecrets))
	}

	for i, expected := range expectedSecrets {
		if c.SopsSecrets[i] != expected {
			t.Errorf("Expected secret %q at index %d, got %q", expected, i, c.SopsSecrets[i])
		}
	}

	// Test with whitespace
	c2 := &NixContainer{
		Labels: map[string]string{
			"compose2nix.settings.sops.secrets": " example.env , folder/example-2.env ",
		},
		SopsSecrets: []string{},
	}

	err = parseNixContainerLabels(c2, sopsConfig)
	if err != nil {
		t.Fatalf("Failed to parse labels with whitespace: %v", err)
	}

	if len(c2.SopsSecrets) != len(expectedSecrets) {
		t.Fatalf("Expected %d secrets with whitespace, got %d", len(expectedSecrets), len(c2.SopsSecrets))
	}
}

func TestSecretsWithoutConfigFails(t *testing.T) {
	c := &NixContainer{
		Labels: map[string]string{
			"compose2nix.settings.sops.secrets": "example.env,folder/example-2.env",
		},
		SopsSecrets: []string{},
	}

	// Pass nil sopsConfig
	err := parseNixContainerLabels(c, nil /* sopsConfig */)
	if err == nil {
		t.Fatalf("Should error when sopsConfig is nil")
	}

	// Should have no sops secrets since config is nil
	if len(c.SopsSecrets) != 0 {
		t.Fatalf("Expected 0 secrets when sopsConfig is nil, got %d", len(c.SopsSecrets))
	}
}

func TestLoadSecretsWithEmptyPath(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tabs only", "\t\t"},
		{"mixed whitespace", " \t \n "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sopsConfig := NewSopsConfig(tt.filePath)

			err := sopsConfig.LoadSecrets()
			if err == nil {
				t.Fatal("Expected error when file path is empty or whitespace")
			}

			expectedError := "sops file path is empty"
			if err.Error() != expectedError {
				t.Errorf("Expected error message %q, got %q", expectedError, err.Error())
			}
		})
	}
}
