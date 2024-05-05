package main

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
	default:
		return v
	}
}

func toNixList(s []string) string {
	b := strings.Builder{}
	for i, e := range s {
		b.WriteString(fmt.Sprintf("%q", e))
		if i < len(s)-1 {
			b.WriteString(" ")
		}
	}
	return fmt.Sprintf("[ %s ]", b.String())
}

func escapeChars(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "$", "\\$")

	return s
}

var funcMap template.FuncMap = template.FuncMap{
	"derefInt":    derefInt,
	"toNixValue":  toNixValue,
	"toNixList":   toNixList,
	"escapeChars": escapeChars,
}
