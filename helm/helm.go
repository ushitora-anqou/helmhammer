package helm

import (
	"errors"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
)

type Chart struct {
	Name             string
	Version          string
	AppVersion       string
	TemplateBasePath string
	Condition        string
	RenderedKeys     []string
	Values           map[string]any
	CRDObjects       []helmchart.CRD
	Files            map[string][]byte
	SubCharts        []*Chart
}

type RootChart struct {
	*Chart
	Capabilities *chartutil.Capabilities
	Template     *template.Template
}

func loadChartsRecursively(
	tmpls *template.Template,
	chart *helmchart.Chart,
) (*Chart, error) {
	basePath := chart.ChartFullPath()
	for _, tmpl := range chart.Templates {
		if tmpl == nil {
			continue
		}
		if _, err := tmpls.New(path.Join(basePath, tmpl.Name)).Parse(string(tmpl.Data)); err != nil {
			return nil, err
		}
	}

	keys := []string{}
	for _, tmpl := range chart.Templates {
		filename := path.Join(basePath, tmpl.Name)
		if strings.HasPrefix(path.Base(tmpl.Name), "_") ||
			strings.HasSuffix(filename, "NOTES.txt") {
			continue
		}
		keys = append(keys, filename)
	}
	sort.Strings(keys)

	files := map[string][]byte{}
	for _, file := range chart.Files {
		files[file.Name] = file.Data
	}

	subCharts := []*Chart{}
	for _, dep := range chart.Dependencies() {
		subChart, err := loadChartsRecursively(tmpls, dep)
		if err != nil {
			return nil, err
		}
		subCharts = append(subCharts, subChart)
	}
	for _, dep := range chart.Metadata.Dependencies {
		index := slices.IndexFunc(subCharts, func(c *Chart) bool {
			return c.Name == dep.Name
		})
		if index == -1 {
			return nil, errors.New("invalid helm chart: missing dependency")
		}
		subCharts[index].Condition = dep.Condition
	}
	slices.SortFunc(subCharts, func(l, r *Chart) int {
		return strings.Compare(l.Name, r.Name)
	})

	return &Chart{
		RenderedKeys:     keys,
		Values:           chart.Values,
		Name:             chart.Metadata.Name,
		Version:          chart.Metadata.Version,
		AppVersion:       chart.Metadata.AppVersion,
		CRDObjects:       chart.CRDObjects(),
		TemplateBasePath: path.Join(chart.ChartFullPath(), "templates"),
		Files:            files,
		SubCharts:        subCharts,
		Condition:        "",
	}, nil
}

func Load(chartDir string) (*RootChart, error) {
	chart, err := loader.Load(chartDir)
	if err != nil {
		return nil, err
	}

	tmpls := template.New(chartDir)
	tmpls.Funcs(funcMap())
	rootChart, err := loadChartsRecursively(tmpls, chart)
	if err != nil {
		return nil, err
	}

	return &RootChart{
		Chart:        rootChart,
		Template:     tmpls,
		Capabilities: chartutil.DefaultCapabilities.Copy(),
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
		"lookup":        func(string, any) (any, error) { return "not implemented", nil },
	}

	maps.Copy(f, extra)

	return f
}
