package main

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
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

// indentNonEmpty indents the given text by the provided number of spaces while
// skipping empty lines.
func indentNonEmpty(spaces int, text string) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

func derefInt(v *int) int {
	return *v
}

func toNixValue(v any) any {
	switch v := v.(type) {
	case string:
		return fmt.Sprintf("%q", escapeNixString(v))
	default:
		return v
	}
}

func toNixList(s []string) string {
	b := strings.Builder{}
	for i, e := range s {
		// We purposefully do not use %q to avoid Go's built-in string escaping.
		b.WriteString(fmt.Sprintf(`"%s"`, escapeNixString(e)))
		if i < len(s)-1 {
			b.WriteString(" ")
		}
	}
	return fmt.Sprintf("[ %s ]", b.String())
}

func escapeNixString(s string) string {
	// https://nix.dev/manual/nix/latest/language/syntax#string-literal
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `${`, `\${`)
	return s
}

// escapeSystemdValue escapes whitespace so the string is treated as a single
// token when it appears in a space-separated systemd unit setting such as
// RequiresMountsFor=
// The result is intended to be wrapped by escapeNixString, which doubles the
// backslash so the Nix string literal evaluates back to a single backslash.
func escapeSystemdValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ':
			b.WriteString(`\ `)
		case '\t':
			b.WriteString(`\\t`)
		default:
			b.WriteRune(r)
		}
	}
	return escapeNixString(b.String())
}

func escapeIndentedNixString(s string) string {
	// https://nix.dev/manual/nix/latest/language/syntax#string-literal
	s = strings.ReplaceAll(s, `''`, `'''`)
	s = strings.ReplaceAll(s, `$`, `''$`)
	return s
}

var funcMap template.FuncMap = template.FuncMap{
	"derefInt":                derefInt,
	"toNixValue":              toNixValue,
	"toNixList":               toNixList,
	"escapeNixString":         escapeNixString,
	"escapeIndentedNixString": escapeIndentedNixString,
	"escapeSystemdValue":      escapeSystemdValue,
}
