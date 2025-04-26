package helm

import (
	"maps"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

type Chart struct {
	Template     *template.Template
	RenderedKeys []string
	Values       map[string]any
	Name         string
	Version      string
	AppVersion   string
}

func Load(chartDir string) (*Chart, error) {
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

	keys := []string{}
	for _, tmpl := range chart.Templates {
		filename := tmpl.Name
		if strings.HasPrefix(path.Base(tmpl.Name), "_") ||
			strings.HasSuffix(filename, "NOTES.txt") {
			continue
		}
		keys = append(keys, filename)
	}
	sort.Strings(keys)

	return &Chart{
		Template:     tmpls,
		RenderedKeys: keys,
		Values:       chart.Values,
		Name:         chart.Metadata.Name,
		Version:      chart.Metadata.Version,
		AppVersion:   chart.Metadata.AppVersion,
	}, nil
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
