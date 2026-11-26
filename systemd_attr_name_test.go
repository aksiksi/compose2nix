package main

import (
	"strings"
	"testing"
)

func TestSystemdKeysAreValidNixAttributeNames(t *testing.T) {
	config := &NixContainerConfig{
		Containers: []*NixContainer{{
			Name:  "app",
			Image: "alpine",
			SystemdConfig: &NixContainerSystemdConfig{
				Service: ServiceConfig{Options: map[string]any{"1foo": "bar", "RuntimeMaxSec": 100}},
				Unit:    UnitConfig{Options: map[string]any{"with": "value"}},
			},
		}},
	}

	got := config.String()
	for _, want := range []string{
		`"1foo" = lib.mkOverride 90 "bar";`,
		`RuntimeMaxSec = lib.mkOverride 90 100;`,
		`"with" = lib.mkOverride 90 "value";`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}
