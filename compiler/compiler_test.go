package compiler_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/go-openapi/jsonpointer"
	gojsonnet "github.com/google/go-jsonnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/compiler/state"
	"github.com/ushitora-anqou/helmhammer/helm"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
	"sigs.k8s.io/yaml"
)

type U struct {
	V string
}

func (u U) TrueFalse(b bool) string {
	if b {
		return "true"
	}
	return ""
}

func (u U) TrueFalseJsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ERaw,
		Raw:  `function(heap, args) [heap, if args[0] then "true" else ""]`,
	}
}

type T struct {
	I         int
	U16       uint16
	X         string
	U         *U
	MSI       map[string]int
	MSIEmpty  map[string]int
	MSIZero   map[string]int
	MSIone    map[string]int
	SI        []int
	SIEmpty   []int
	SIZero    []int
	SB        []bool
	Empty0    any
	Empty3    any
	PSI       *[]int
	True      bool
	FloatZero float64
}

func (t T) Method0() string {
	return "M0"
}

func (t T) Method0Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"heap", "args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ERaw,
			Raw:  `[heap, "M0"]`,
		},
	}
}

func (t T) Method1(a int) int {
	return a
}

func (t T) Method1Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"heap", "args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ERaw,
			Raw:  `[heap, args[0]]`,
		},
	}
}

func (t T) Method2(a uint16, b string) string {
	return fmt.Sprintf("Method2: %d %s", a, b)
}

func (t T) Method2Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"heap", "args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ERaw,
			Raw:  `[heap, "Method2: %d %s" % [args[0], args[1]]]`,
		},
	}
}

func (t T) Method3(v any) string {
	return fmt.Sprintf("Method3: %v", v)
}

func (t T) Method3Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"heap", "args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ERaw,
			Raw: `[heap, "Method3: %s" % [(
				if args[0] == null then "<nil>"
				else error "not implemented"
			)]]`,
		},
	}
}

func (t T) MAdd(a int, b []int) []int {
	v := make([]int, len(b))
	for i, x := range b {
		v[i] = x + a
	}
	return v
}

func (t T) MAddJsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ERaw,
		Raw: strings.TrimSpace(`
function(heap, args)
	assert std.isNumber(args[0]);
	assert isAddr(args[1]);
	[heap, std.map(
		function(x) x + args[0],
		deref(heap, args[1]),
	)]`,
		),
	}
}

func newIntSlice(n ...int) *[]int {
	p := new([]int)
	*p = make([]int, len(n))
	copy(*p, n)
	return p
}

type compileTest struct {
	name string
	tpl  string
	data any
}

func testCompile(t *testing.T, tmpl0 *template.Template, tests []compileTest) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := tmpl0.Clone()
			require.NoError(t, err)
			tmpl, err = tmpl.New(tt.name).Parse(tt.tpl)
			require.NoError(t, err)
			jsonnetExpr, err := compiler.Compile(tmpl)
			require.NoError(t, err)
			jsonnetExpr = &jsonnet.Expr{
				Kind: jsonnet.ELocal,
				LocalBinds: []*jsonnet.LocalBind{
					{Name: "inputData", Body: jsonnet.CallFromConst(
						jsonnet.EmptyMap(), jsonnet.ConvertIntoJsonnet(tt.data))},
					{Name: "output", Body: &jsonnet.Expr{
						Kind: jsonnet.ECall,
						CallFunc: &jsonnet.Expr{
							Kind:          jsonnet.EIndexList,
							IndexListHead: jsonnetExpr,
							IndexListTail: []string{tt.name},
						},
						CallArgs: []*jsonnet.Expr{
							jsonnet.IndexInt("inputData", 0), // heap
							jsonnet.IndexInt("inputData", 1), // dot
						},
					}},
				},
				LocalBody: jsonnet.IndexInt("output", 0),
			}

			log.Printf("%s", jsonnetExpr.String())

			sb := strings.Builder{}
			tmpl.Option("missingkey=zero")
			err = tmpl.ExecuteTemplate(&sb, tt.name, tt.data)
			require.NoError(t, err)
			expected := sb.String()

			vm := gojsonnet.MakeVM()
			vm.StringOutput = true
			got, err := vm.EvaluateAnonymousSnippet(
				"file.jsonnet",
				jsonnetExpr.StringWithPrologue(),
			)
			require.NoError(t, err)
			got = strings.Trim(got, "\n")
			assert.Equal(t, expected, got)
		})
	}
}

func TestCompileValidTemplates(t *testing.T) {
	tVal := &T{
		I:       17,
		U16:     16,
		X:       "x",
		U:       &U{V: "v"},
		MSI:     map[string]int{"one": 1, "two": 2, "three": 3},
		MSIZero: map[string]int{},
		MSIone:  map[string]int{"one": 1},
		SI:      []int{3, 4, 5},
		SB:      []bool{true, false},
		Empty3:  []int{7, 8},
		PSI:     newIntSlice(21, 22, 23),
		True:    true,
		SIZero:  []int{},
	}

	// The following test table comes from Go compiler's test code:
	// 	https://cs.opensource.google/go/go/+/refs/tags/go1.24.2:src/text/template/exec_test.go
	tests := []compileTest{
		{"empty", "", nil},
		{"text", "some text", nil},
		{".U.V", "-{{.U.V}}-", tVal},
		{".X", "-{{.X}}-", tVal},
		{"map .one", "{{.MSI.one}}", tVal},
		{"map .two", "{{.MSI.two}}", tVal},
		{"dot int", "<{{.}}>", 13},
		{"dot float", "<{{.}}>", 15.1},
		{"dot bool", "<{{.}}>", true},
		{"dot string", "<{{.}}>", "hello"},
		{"$ int", "{{$}}", 123},
		{"$.I", "{{$.I}}", tVal},
		{"$.U.V", "{{$.U.V}}", tVal},
		{"declare in action", "{{$x := $.U.V}}{{$x}}", tVal},
		{"simple assignment", "{{$x := 2}}{{$x = 3}}{{$x}}", tVal},
		{"nested assignment", "{{$x := 2}}{{if true}}{{$x = 3}}{{end}}{{$x}}", tVal},
		{"nested assignment changes the last declaration", "{{$x := 1}}{{if true}}{{$x := 2}}{{if true}}{{$x = 3}}{{end}}{{end}}{{$x}}", tVal},
		{"parenthesized non-function with no args", "{{(1)}}", nil},

		// Range.
		{"range []int", "{{range .SI}}-{{.}}-{{end}}", tVal},
		{"range empty no else", "{{range .SIEmpty}}-{{.}}-{{end}}", tVal},
		{"range zero []int no else", "{{range .SIZero}}-{{.}}-{{end}}", tVal},
		{"range []int else", "{{range .SI}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty else", "{{range .SIEmpty}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range zero []int else", "{{range .SIZero}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range []bool", "{{range .SB}}-{{.}}-{{end}}", tVal},
		//{"range []int method", "{{range .SI | .MAdd .I}}-{{.}}-{{end}}", tVal},
		{"range map", "{{range .MSI}}-{{.}}-{{end}}", tVal},
		{"range empty map no else", "{{range .MSIEmpty}}-{{.}}-{{end}}", tVal},
		{"range zero map no else", "{{range .MSIZero}}-{{.}}-{{end}}", tVal},
		{"range map else", "{{range .MSI}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty map else", "{{range .MSIEmpty}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range zero map else", "{{range .MSIZero}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty interface", "{{range .Empty3}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty nil", "{{range .Empty0}}-{{.}}-{{end}}", tVal},
		{"range $x SI", "{{range $x := .SI}}<{{$x}}>{{end}}", tVal},
		{"range $x $y SI", "{{range $x, $y := .SI}}<{{$x}}={{$y}}>{{end}}", tVal},
		{"range $x MSIone", "{{range $x := .MSIone}}<{{$x}}>{{end}}", tVal},
		{"range $x $y MSIone", "{{range $x, $y := .MSIone}}<{{$x}}={{$y}}>{{end}}", tVal},
		{"range $x PSI", "{{range $x := .PSI}}<{{$x}}>{{end}}", tVal},
		{"declare in range", "{{range $x := .PSI}}<{{$foo:=$x}}{{$x}}>{{end}}", tVal},
		{"range 5", `{{range $v := 5}}{{printf "%d" $v}}{{end}}`, nil},

		// Method calls.
		{".Method0", "-{{.Method0}}-", tVal},
		{".Method1(1234)", "-{{.Method1 1234}}-", tVal},
		{".Method1(.I)", "-{{.Method1 .I}}-", tVal},
		{".Method2(3, .X)", "-{{.Method2 3 .X}}-", tVal},
		{".Method2(.U16, `str`)", "-{{.Method2 .U16 `str`}}-", tVal},
		{".Method2(.U16, $x)", "{{if $x := .X}}-{{.Method2 .U16 $x}}{{end}}-", tVal},
		{".Method3(nil constant)", "-{{.Method3 nil}}-", tVal},
		{"method on var", "{{if $x := .}}-{{$x.Method2 .U16 $x.X}}{{end}}-", tVal},
		{"method on chained var",
			"{{range .MSIone}}{{if $.U.TrueFalse $.True}}{{$.U.TrueFalse $.True}}{{else}}WRONG{{end}}{{end}}",
			tVal},
		//{"chained method",
		//	"{{range .MSIone}}{{if $.GetU.TrueFalse $.True}}{{$.U.TrueFalse $.True}}{{else}}WRONG{{end}}{{end}}",
		//	tVal},
		//	{"chained method on variable",
		//		"{{with $x := .}}{{with .SI}}{{$.GetU.TrueFalse $.True}}{{end}}{{end}}",
		//		"true", tVal, true},
		//	{".NilOKFunc not nil", "{{call .NilOKFunc .PI}}", "false", tVal, true},
		//	{".NilOKFunc nil", "{{call .NilOKFunc nil}}", "true", tVal, true},
		//	{"method on nil value from slice", "-{{range .}}{{.Method1 1234}}{{end}}-", "-1234-", tSliceOfNil, true},
		//	{"method on typed nil interface value", "{{.NonEmptyInterfaceTypedNil.Method0}}", "M0", tVal, true},

		// If.
		{"if true", "{{if true}}TRUE{{end}}", tVal},
		{"if false", "{{if false}}TRUE{{else}}FALSE{{end}}", tVal},
		{"if 1", "{{if 1}}NON-ZERO{{else}}ZERO{{end}}", tVal},
		{"if 0", "{{if 0}}NON-ZERO{{else}}ZERO{{end}}", tVal},
		{"if 1.5", "{{if 1.5}}NON-ZERO{{else}}ZERO{{end}}", tVal},
		{"if 0.0", "{{if .FloatZero}}NON-ZERO{{else}}ZERO{{end}}", tVal},
		{"if emptystring", "{{if ``}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if string", "{{if `notempty`}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if emptyslice", "{{if .SIEmpty}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if zeroslice", "{{if .SIZero}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if slice", "{{if .SI}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if emptymap", "{{if .MSIEmpty}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if zeromap", "{{if .MSIZero}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if map", "{{if .MSI}}NON-EMPTY{{else}}EMPTY{{end}}", tVal},
		{"if $x with $y int", "{{if $x := true}}{{with $y := .I}}{{$x}},{{$y}}{{end}}{{end}}", tVal},
		{"if $x with $x int", "{{if $x := true}}{{with $x := .I}}{{$x}},{{end}}{{$x}}{{end}}", tVal},
		{"if else if", "{{if false}}FALSE{{else if true}}TRUE{{end}}", tVal},
		{"if else chain", "{{if eq 1 3}}1{{else if eq 2 3}}2{{else if eq 3 3}}3{{end}}", tVal},

		// With.
		{"with true", "{{with true}}{{.}}{{end}}", tVal},
		{"with false", "{{with false}}{{.}}{{else}}FALSE{{end}}", tVal},
		{"with 1", "{{with 1}}{{.}}{{else}}ZERO{{end}}", tVal},
		{"with 0", "{{with 0}}{{.}}{{else}}ZERO{{end}}", tVal},
		{"with 1.5", "{{with 1.5}}{{.}}{{else}}ZERO{{end}}", tVal},
		{"with 0.0", "{{with .FloatZero}}{{.}}{{else}}ZERO{{end}}", tVal},
		{"with emptystring", "{{with ``}}{{.}}{{else}}EMPTY{{end}}", tVal},
		{"with string", "{{with `notempty`}}{{.}}{{else}}EMPTY{{end}}", tVal},
		{"with emptyslice", "{{with .SIEmpty}}{{.}}{{else}}EMPTY{{end}}", tVal},
		//{"with slice", "{{with .SI}}{{.}}{{else}}EMPTY{{end}}", tVal},
		{"with emptymap", "{{with .MSIEmpty}}{{.}}{{else}}EMPTY{{end}}", tVal},
		//{"with map", "{{with .MSIone}}{{.}}{{else}}EMPTY{{end}}", tVal},
		//{"with empty interface, struct field", "{{with .Empty4}}{{.V}}{{end}}", tVal},
		{"with $x int", "{{with $x := .I}}{{$x}}{{end}}", tVal},
		{"with $x struct.U.V", "{{with $x := $}}{{$x.U.V}}{{end}}", tVal},
		{"with variable and action", "{{with $x := $}}{{$y := $.U.V}}{{$y}}{{end}}", tVal},
		{"with else with", "{{with 0}}{{.}}{{else with true}}{{.}}{{end}}", tVal},
		{"with else with chain", "{{with 0}}{{.}}{{else with false}}{{.}}{{else with `notempty`}}{{.}}{{end}}", tVal},

		{"templates", `{{define "foo"}}{{.}}{{end}}{{template "foo" 3}}`, nil},
	}

	testCompile(t, template.New("gotpl"), tests)
}

func TestCompileChartValid(t *testing.T) {
	testdataDir := "testdata"

	tests := []struct {
		name, chartDir, namespace, valuesYaml, expectedOutput string
		patches                                               []string
		yamlPaths                                             []string
	}{
		{name: "skeleton", chartDir: "skeleton", expectedOutput: "skeleton.expected"},

		{
			name:           "testchart",
			chartDir:       "testchart",
			valuesYaml:     "testchart.values.yaml",
			expectedOutput: "testchart.expected",
		},

		{
			name:           "topolvm 0: empty values",
			chartDir:       "thirdparty/topolvm-15.5.4",
			expectedOutput: "topolvm-15.5.4-0.expected",
			// Some fields won't be equal due to toYaml's different behaviour.
			patches: []string{
				`{"op": "remove", "path": "/2/spec/template/metadata/annotations/checksum~1config"}`,
			},
			yamlPaths: []string{`/31/data/lvmd.yaml`},
		},

		{
			name:           "topolvm 1: some values",
			chartDir:       "thirdparty/topolvm-15.5.4",
			namespace:      "topolvm-system",
			valuesYaml:     "topolvm-15.5.4-1.values.yaml",
			expectedOutput: "topolvm-15.5.4-1.expected",

			// ❯ cat topolvm-15.5.4-1.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "topolvm-lvmd-0" and .kind == "DaemonSet") | index(true)'
			// 2
			patches: []string{
				`{"op": "remove", "path": "/2/spec/template/metadata/annotations/checksum~1config"}`,
			},

			// ❯ cat topolvm-15.5.4-1.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "topolvm-lvmd-0" and .kind == "ConfigMap") | index(true)'
			// 32
			yamlPaths: []string{`/32/data/lvmd.yaml`},
		},

		{
			name:           "reloader 0: empty",
			chartDir:       "thirdparty/reloader-2.1.3",
			expectedOutput: "reloader-2.1.3-0.expected",
		},

		{
			name:           "reloader 1: some values",
			chartDir:       "thirdparty/reloader-2.1.3",
			namespace:      "reloader",
			valuesYaml:     "reloader-2.1.3-1.values.yaml",
			expectedOutput: "reloader-2.1.3-1.expected",
		},

		{
			name:           "cloudflare-tunnel-ingress-controller 0: empty",
			chartDir:       "thirdparty/cloudflare-tunnel-ingress-controller-0.0.18",
			expectedOutput: "cloudflare-tunnel-ingress-controller-0.0.18-0.expected",
		},

		{
			name:           "cloudflare-tunnel-ingress-controller 1: some values",
			chartDir:       "thirdparty/cloudflare-tunnel-ingress-controller-0.0.18",
			namespace:      "ctic",
			valuesYaml:     "cloudflare-tunnel-ingress-controller-0.0.18-1.values.yaml",
			expectedOutput: "cloudflare-tunnel-ingress-controller-0.0.18-1.expected",
		},

		{
			name:           "sidekiq-prometheus-exporter 0: empty",
			chartDir:       "thirdparty/sidekiq-prometheus-exporter-0.2.1",
			expectedOutput: "sidekiq-prometheus-exporter-0.2.1-0.expected",
		},

		{
			name:           "sidekiq-prometheus-exporter 1: some values",
			chartDir:       "thirdparty/sidekiq-prometheus-exporter-0.2.1",
			namespace:      "sidekiq-prometheus-exporter",
			valuesYaml:     "sidekiq-prometheus-exporter-0.2.1-1.values.yaml",
			expectedOutput: "sidekiq-prometheus-exporter-0.2.1-1.expected",
		},

		{
			name:           "cert-manager 0: empty",
			chartDir:       "thirdparty/cert-manager-v1.17.2",
			expectedOutput: "cert-manager-v1.17.2-0.expected",
		},

		{
			name:           "cert-manager 1: some values",
			chartDir:       "thirdparty/cert-manager-v1.17.2",
			namespace:      "cert-manager",
			valuesYaml:     "cert-manager-v1.17.2-1.values.yaml",
			expectedOutput: "cert-manager-v1.17.2-1.expected",
		},

		{
			name:           "argo-cd 0: empty",
			chartDir:       "thirdparty/argo-cd-7.9.0",
			expectedOutput: "argo-cd-7.9.0-0.expected",
			patches: []string{
				`{"op": "remove", "path": "/3/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/4/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/7/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/8/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/9/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/7/spec/template/metadata/annotations/checksum~1cm"}`,
				`{"op": "remove", "path": "/8/spec/template/metadata/annotations/checksum~1cm"}`,
				`{"op": "remove", "path": "/9/spec/template/metadata/annotations/checksum~1cm"}`,
			},
		},

		{
			name:           "argo-cd 1: some values",
			chartDir:       "thirdparty/argo-cd-7.9.0",
			namespace:      "argocd",
			valuesYaml:     "argo-cd-7.9.0-1.values.yaml",
			expectedOutput: "argo-cd-7.9.0-1.expected",
			patches: []string{
				`{"op": "remove", "path": "/3/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/4/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/7/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/8/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/9/spec/template/metadata/annotations/checksum~1cmd-params"}`,
				`{"op": "remove", "path": "/7/spec/template/metadata/annotations/checksum~1cm"}`,
				`{"op": "remove", "path": "/8/spec/template/metadata/annotations/checksum~1cm"}`,
				`{"op": "remove", "path": "/9/spec/template/metadata/annotations/checksum~1cm"}`,
			},
		},

		{
			name:           "promtail 0: empty",
			chartDir:       "thirdparty/promtail-6.16.6",
			expectedOutput: "promtail-6.16.6-0.expected",

			// ❯ cat compiler/testdata/promtail-6.16.6-0.expected | jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.spec.template.metadata.annotations | has("checksum/config")) | index(true)'
			// 0
			patches: []string{
				`{"op": "remove", "path": "/0/spec/template/metadata/annotations/checksum~1config"}`,
			},

			// ❯ cat compiler/testdata/promtail-6.16.6-0.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "promtail" and .kind == "Secret") | index(true)'
			// 3
			yamlPaths: []string{
				`/3/stringData/promtail.yaml`,
			},
		},

		{
			name:           "promtail 1: some values",
			chartDir:       "thirdparty/promtail-6.16.6",
			namespace:      "promtail",
			valuesYaml:     "promtail-6.16.6-1.values.yaml",
			expectedOutput: "promtail-6.16.6-1.expected",

			// ❯ cat compiler/testdata/promtail-6.16.6-0.expected | jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.spec.template.metadata.annotations | has("checksum/config")) | index(true)'
			// 0
			patches: []string{
				`{"op": "remove", "path": "/0/spec/template/metadata/annotations/checksum~1config"}`,
			},

			// ❯ cat compiler/testdata/promtail-6.16.6-0.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "promtail" and .kind == "Secret") | index(true)'
			// 3
			yamlPaths: []string{
				`/3/stringData/promtail.yaml`,
			},
		},

		{
			name:           "loki: some values",
			chartDir:       "thirdparty/loki-6.29.0",
			namespace:      "loki",
			valuesYaml:     "loki-6.29.0-1.values.yaml",
			expectedOutput: "loki-6.29.0-1.expected",
			patches: []string{
				`{"op": "remove", "path": "/1/spec/template/metadata/annotations/checksum~1config"}`,
				`{"op": "remove", "path": "/2/spec/template/metadata/annotations/checksum~1config"}`,
			},
			// ❯ cat compiler/testdata/loki-6.29.0-1.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "loki" and .kind == "ConfigMap") | index(true)'
			yamlPaths: []string{`/5/data/config.yaml`},
		},

		{
			name:           "tempo: empty",
			chartDir:       "thirdparty/tempo-1.21.1",
			expectedOutput: "tempo-1.21.1-0.expected",
			patches: []string{
				`{"op": "remove", "path": "/0/spec/template/metadata/annotations/checksum~1config"}`,
			},
			// cat compiler/testdata/tempo-1.21.1-0.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "tempo" and .kind == "ConfigMap") | index(true)'
			yamlPaths: []string{`/1/data/tempo.yaml`},
		},

		{
			name:           "tempo: some values",
			chartDir:       "thirdparty/tempo-1.21.1",
			namespace:      "tempo",
			valuesYaml:     "tempo-1.21.1-1.values.yaml",
			expectedOutput: "tempo-1.21.1-1.expected",
			patches: []string{
				`{"op": "remove", "path": "/0/spec/template/metadata/annotations/checksum~1config"}`,
			},
			// cat compiler/testdata/tempo-1.21.1-0.expected|jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | map(.metadata.name == "tempo" and .kind == "ConfigMap") | index(true)'
			yamlPaths: []string{`/1/data/tempo.yaml`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalizeManifests := func(src []byte, patch jsonpatch.Patch) ([]map[string]any, []map[string]any) {
				var parsed []map[string]any
				err := json.Unmarshal(src, &parsed)
				require.NoError(t, err)

				slices.SortFunc(parsed, func(a map[string]any, b map[string]any) int {
					aMetadata := a["metadata"].(map[string]any)
					bMetadata := b["metadata"].(map[string]any)
					return strings.Compare(
						fmt.Sprintf("%s-%s-%s-%s", a["apiVersion"], a["kind"], aMetadata["namespace"], aMetadata["name"]),
						fmt.Sprintf("%s-%s-%s-%s", b["apiVersion"], b["kind"], bMetadata["namespace"], bMetadata["name"]),
					)
				})

				sorted, err := json.Marshal(parsed)
				require.NoError(t, err)

				patched, err := patch.Apply(sorted)
				require.NoError(t, err)

				err = json.Unmarshal(patched, &parsed)
				require.NoError(t, err)

				var unpatchedParsed []map[string]any
				err = json.Unmarshal(sorted, &unpatchedParsed)
				require.NoError(t, err)

				return parsed, unpatchedParsed
			}

			var patch jsonpatch.Patch
			if tt.patches != nil || tt.yamlPaths != nil {
				yamlPatches := convertPathsToRemovalPatches(tt.yamlPaths)
				src := fmt.Sprintf(
					"[%s]",
					strings.Join(slices.Concat(tt.patches, yamlPatches), ","),
				)
				var err error
				patch, err = jsonpatch.DecodePatch([]byte(src))
				require.NoError(t, err)
			}

			chart, err := helm.Load(filepath.Join(testdataDir, tt.chartDir))
			require.NoError(t, err)

			state.ResetGenID()
			compiledChart, err := compiler.CompileChart(chart)
			require.NoError(t, err)

			jsonnetExpr := &jsonnet.Expr{
				Kind:     jsonnet.ECall,
				CallFunc: compiledChart,
				CallArgs: []*jsonnet.Expr{},
				CallNamedArgs: []*jsonnet.NamedArg{
					{Name: "includeCrds", Arg: &jsonnet.Expr{Kind: jsonnet.ETrue}},
				},
			}
			if tt.namespace != "" {
				jsonnetExpr.CallNamedArgs = append(jsonnetExpr.CallNamedArgs, &jsonnet.NamedArg{
					Name: "namespace",
					Arg:  jsonnet.ConvertIntoJsonnet(tt.namespace),
				})
			}
			if tt.valuesYaml != "" {
				valuesYaml, err := os.ReadFile(filepath.Join(testdataDir, tt.valuesYaml))
				require.NoError(t, err)
				var values any
				err = yaml.Unmarshal(valuesYaml, &values)
				require.NoError(t, err)
				jsonnetExpr.CallNamedArgs = append(jsonnetExpr.CallNamedArgs, &jsonnet.NamedArg{
					Name: "values",
					Arg:  jsonnet.ConvertIntoJsonnet(values),
				})
			}
			vm := gojsonnet.MakeVM()
			vm.MaxStack = 2000
			gotString, err := vm.EvaluateAnonymousSnippet(
				"file.jsonnet",
				jsonnetExpr.StringWithPrologue(),
			)
			require.NoError(t, err)
			got, unpatchedGot := finalizeManifests([]byte(strings.Trim(gotString, "\n")), patch)

			expectedSrc, err := os.ReadFile(filepath.Join(testdataDir, tt.expectedOutput))
			require.NoError(t, err)
			expected, unpatchedExpected := finalizeManifests(expectedSrc, patch)

			assert.Equal(t, expected, got)

			for _, yamlPath := range tt.yamlPaths {
				p, err := jsonpointer.New(yamlPath)
				require.NoError(t, err)

				expectedRaw, _, err := p.Get(unpatchedExpected)
				require.NoError(t, err)
				expected, ok := expectedRaw.(string)
				require.True(t, ok)
				var expectedParsed any
				err = yaml.Unmarshal([]byte(expected), &expectedParsed)
				require.NoError(t, err)

				gotRaw, _, err := p.Get(unpatchedGot)
				assert.NoError(t, err)
				got, ok := gotRaw.(string)
				assert.True(t, ok)
				var gotParsed any
				err = yaml.Unmarshal([]byte(got), &gotParsed)
				assert.NoError(t, err)

				assert.Equal(t, expectedParsed, gotParsed)
			}

			state.ResetGenID()
			compiledChart1, err := compiler.CompileChart(chart)
			require.NoError(t, err)
			assert.Equal(t, compiledChart.String(), compiledChart1.String())

		})
	}
}

func convertPathsToRemovalPatches(paths []string) []string {
	patches := make([]string, 0, len(paths))
	for _, path := range paths {
		patches = append(patches, fmt.Sprintf(`{"op":"remove","path":"%s"}`, path))
	}
	return patches
}
