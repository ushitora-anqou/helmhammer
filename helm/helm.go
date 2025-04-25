package helm

import (
	"maps"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

func Load(chartDir string) (*template.Template, error) {
	chart, err := loader.Load(chartDir)
	if err != nil {
		return nil, err
	}

	tmpls := template.New(chartDir)
	tmpls.Funcs(funcMap())
	for _, tmpl := range chart.Templates {
		if tmpl == nil {
			continue
		}
		if _, err := tmpls.New(tmpl.Name).Parse(string(tmpl.Data)); err != nil {
			return nil, err
		}
	}

	return tmpls, nil
}

func funcMap() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")

	extra := template.FuncMap{
		"toToml":        func(any) string { return "not implemented" },
		"fromToml":      func(string) map[string]any { return nil },
		"toYaml":        func(any) string { return "not implemented" },
		"toYamlPretty":  func(any) string { return "not implemented" },
		"fromYaml":      func(string) map[string]any { return nil },
		"fromYamlArray": func(string) []any { return nil },
		"toJson":        func(any) string { return "not implemented" },
		"fromJson":      func(string) map[string]any { return nil },
		"fromJsonArray": func(string) []any { return nil },
		"include":       func(string, any) string { return "not implemented" },
		"tpl":           func(string, any) any { return "not implemented" },
		"required":      func(string, any) (any, error) { return "not implemented", nil },
	}

	maps.Copy(f, extra)

	return f
}
