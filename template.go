package nixose

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
)

//go:embed templates/*.tmpl
var templateFS embed.FS
var nixTemplates = template.New("nix").Funcs(sprig.FuncMap()).Funcs(funcMap)

func labelMapToLabelFlags(l map[string]string) []string {
	// https://docs.docker.com/engine/reference/commandline/run/#label
	// https://docs.podman.io/en/latest/markdown/podman-run.1.html#label-l-key-value
	labels := mapToKeyValArray(l)
	for i, label := range labels {
		labels[i] = fmt.Sprintf("--label=%s", label)
	}
	return labels
}

func execTemplate(t *template.Template) func(string, any) (string, error) {
	return func(name string, v any) (string, error) {
		var s strings.Builder
		err := t.ExecuteTemplate(&s, name, v)
		return s.String(), err
	}
}

func derefInt(v *int) int {
	return *v
}

func toNixValue(v any) any {
	switch v.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	default:
		return v
	}
}

var funcMap template.FuncMap = template.FuncMap{
	"derefInt":             derefInt,
	"labelMapToLabelFlags": labelMapToLabelFlags,
	"mapToKeyValArray":     mapToKeyValArray,
	"toNixValue":           toNixValue,
}
