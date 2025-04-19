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

func TestCompileValidTemplates(t *testing.T) {
	tVal := map[string]any{
		"I": 17,
		"X": "x",
		"U": map[string]string{
			"V": "v",
		},
		"MSI": map[string]int{
			"one": 1,
			"two": 2,
		},
	}

	tests := []struct {
		name string
		tpl  string
		data any
	}{
		{"empty", "", nil},
		{"text", "some text", nil},
		{".U.V", "-{{.U.V}}-", tVal},
		{".X", "-{{.X}}-", tVal},
		//{"map .one", "{{.MSI.one}}", tVal},
		//{"map .two", "{{.MSI.two}}", tVal},
		//{"dot int", "<{{.}}>", 13},
		//{"dot float", "<{{.}}>", 15.1},
		//{"dot bool", "<{{.}}>", true},
		//{"dot string", "<{{.}}>", "hello"},
		//{"$ int", "{{$}}", 123},
		//{"$.I", "{{$.I}}", tVal},
		//{"$.U.V", "{{$.U.V}}", tVal},
		//{"declare in action", "{{$x := $.U.V}}{{$x}}", tVal},
		//{"simple assignment", "{{$x := 2}}{{$x = 3}}{{$x}}", tVal},
		//{"nested assignment", "{{$x := 2}}{{if true}}{{$x = 3}}{{end}}{{$x}}", tVal},
		//{"nested assignment changes the last declaration", "{{$x := 1}}{{if true}}{{$x := 2}}{{if true}}{{$x = 3}}{{end}}{{end}}{{$x}}", tVal},

		//{
		//	name: "if simple true",
		//	tpl:  `hel{{ if true }}lo2{{ else }}lo3{{ end }}`,
		//},
		//{
		//	name: "if simple false",
		//	tpl:  `hel{{ if false }}lo2{{ else }}lo3{{ end }}`,
		//},
		//{
		//	name: "if .",
		//	tpl:  `{{ if . }}1{{ else }}0{{ end }}`,
		//	data: true,
		//},
		//{
		//	name: "if .a",
		//	tpl:  `{{ if .a }}1{{ else }}0{{ end }}`,
		//	data: map[string]any{"a": false},
		//},
		//{
		//	name: "if .a.b",
		//	tpl:  `{{ if .a.b }}1{{ else }}0{{ end }}`,
		//	data: map[string]map[string]any{"a": {"b": false}},
		//},
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
					jsonnet.ConvertDataToJsonnetExpr(tt.data),
				},
			}

			sb := strings.Builder{}
			tpl.Option("missingkey=zero")
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
