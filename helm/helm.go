package helm

import (
	"text/template"

	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

func Load(chartDir string) (*template.Template, error) {
	chart, err := loader.Load(chartDir)
	if err != nil {
		return nil, err
	}

	tmpls := template.New(chartDir)
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
