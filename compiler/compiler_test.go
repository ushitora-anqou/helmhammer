package compiler_test

import (
	"strings"
	"testing"
	"text/template"

	gojsonnet "github.com/google/go-jsonnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

func TestCompile(t *testing.T) {
	tests := []struct {
		name string
		tpl  string
		data any
	}{
		{
			name: "empty",
			tpl:  ``,
		},
		{
			name: "simple string without mustaches",
			tpl:  `hello`,
		},
		{
			name: "if simple true",
			tpl:  `hel{{ if true }}lo2{{ else }}lo3{{ end }}`,
		},
		{
			name: "if simple false",
			tpl:  `hel{{ if false }}lo2{{ else }}lo3{{ end }}`,
		},
		{
			name: "if .",
			tpl:  `{{ if . }}1{{ else }}0{{ end }}`,
			data: true,
		},
		{
			name: "if .a",
			tpl:  `{{ if .a }}1{{ else }}0{{ end }}`,
			data: map[string]any{"a": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl0 := template.New("gotpl")
			tpl, err := tpl0.New("file").Parse(tt.tpl)
			require.NoError(t, err)
			jsonnetExpr, err := compiler.Compile(tpl.Root)
			require.NoError(t, err)
			jsonnetExpr = &jsonnet.Expr{
				Kind:     jsonnet.ECall,
				CallFunc: jsonnetExpr,
				CallArgs: []*jsonnet.Expr{
					convertDataToJsonnetExpr(tt.data),
				},
			}

			sb := strings.Builder{}
			err = tpl.ExecuteTemplate(&sb, "file", tt.data)
			require.NoError(t, err)
			expected := sb.String()

			vm := gojsonnet.MakeVM()
			vm.StringOutput = true
			got, err := vm.EvaluateAnonymousSnippet(
				"file.jsonnet",
				jsonnetExpr.StringWithPrologue(),
			)
			got = strings.Trim(got, "\n")
			assert.Equal(t, expected, got)
		})
	}

	assert.Equal(t, 123, 123)
}

func convertDataToJsonnetExpr(data any) *jsonnet.Expr {
	switch v := data.(type) {
	case nil:
		return &jsonnet.Expr{Kind: jsonnet.ENull}
	case bool:
		kind := jsonnet.EFalse
		if v {
			kind = jsonnet.ETrue
		}
		return &jsonnet.Expr{Kind: kind}
	case map[string]any:
		exprMap := map[*jsonnet.Expr]*jsonnet.Expr{}
		for k, v1 := range v {
			exprMap[&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: k,
			}] = convertDataToJsonnetExpr(v1)
		}
		return &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map:  exprMap,
		}
	}
	panic("not implemented")
}
