package compose2nix

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
	switch v := v.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []string:
		var l []string
		for _, s := range v {
			l = append(l, toNixValue(s).(string))
		}
		return fmt.Sprintf("[ %s ]", strings.Join(l, " "))
	default:
		return v
	}
}

var funcMap template.FuncMap = template.FuncMap{
	"derefInt":                derefInt,
	"mapToKeyValArray":        mapToKeyValArray,
	"mapToRepeatedKeyValFlag": mapToRepeatedKeyValFlag,
	"toNixValue":              toNixValue,
}
