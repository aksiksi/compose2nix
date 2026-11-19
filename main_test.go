package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestHandleRuntimeEnvFileFlags(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr string
	}{
		{"simple list", "a.env,b.env", []string{"a.env", "b.env"}, ""},
		{"spaces after comma", "a.env, b.env", []string{"a.env", "b.env"}, ""},
		{"surrounding spaces", " a.env , b.env ", []string{"a.env", "b.env"}, ""},
		{"trailing comma", "a.env,", nil, "empty file path"},
		{"leading comma", ",a.env", nil, "empty file path"},
		{"boolean true", "true", nil, "not a boolean"},
		{"boolean false", "false", nil, "not a boolean"},
		{"empty value", "", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldInclude, oldOnly, oldIgnore := *includeEnvFiles, *envFilesOnly, *ignoreMissingEnvFiles
			defer func() {
				*includeEnvFiles, *envFilesOnly, *ignoreMissingEnvFiles = oldInclude, oldOnly, oldIgnore
			}()
			*includeEnvFiles, *envFilesOnly, *ignoreMissingEnvFiles = tt.value, false, false

			got, err := handleRuntimeEnvFileFlags()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got err %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
