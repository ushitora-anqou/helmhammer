package compiler_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"text/template"

	gojsonnet "github.com/google/go-jsonnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

type U struct {
	V string
}

type T struct {
	I   int
	X   string
	U   *U
	MSI map[string]int
}

func (t T) Method0() string {
	return "M0"
}

func (t T) Method0Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"args"},
		FunctionBody: &jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: "M0",
		},
	}
}

func (t T) Method1(a int) int {
	return a
}

func (t T) Method1Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.EIndex,
			BinOpLHS: &jsonnet.Expr{
				Kind:   jsonnet.EID,
				IDName: "args",
			},
			BinOpRHS: &jsonnet.Expr{
				Kind:       jsonnet.EIntLiteral,
				IntLiteral: 0,
			},
		},
	}
}

func convertIntoJsonnet(data any) *jsonnet.Expr {
	if data == nil {
		return &jsonnet.Expr{Kind: jsonnet.ENull}
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Bool:
		kind := jsonnet.EFalse
		if v.Bool() {
			kind = jsonnet.ETrue
		}
		return &jsonnet.Expr{Kind: kind}

	case reflect.Int:
		return &jsonnet.Expr{Kind: jsonnet.EIntLiteral, IntLiteral: int(v.Int())}

	case reflect.Float64:
		return &jsonnet.Expr{Kind: jsonnet.EFloatLiteral, FloatLiteral: v.Float()}

	case reflect.String:
		return &jsonnet.Expr{Kind: jsonnet.EStringLiteral, StringLiteral: v.String()}

	case reflect.Map:
		exprMap := map[*jsonnet.Expr]*jsonnet.Expr{}
		iter := v.MapRange()
		for iter.Next() {
			exprMap[&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: iter.Key().Interface().(string),
			}] = convertIntoJsonnet(iter.Value().Interface())
		}
		return &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map:  exprMap,
		}

	case reflect.Struct:
		exprMap := map[*jsonnet.Expr]*jsonnet.Expr{}
		ty := v.Type()
		for i := range ty.NumField() {
			field := ty.Field(i)
			exprMap[&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: field.Name,
			}] = convertIntoJsonnet(v.FieldByIndex(field.Index).Interface())
		}
		for i := range ty.NumMethod() {
			mthd := ty.Method(i)
			mthdJsonnet := v.MethodByName(mthd.Name + "Jsonnet")
			if !mthdJsonnet.IsValid() || mthdJsonnet.IsZero() {
				continue
			}
			ret := v.MethodByName(mthd.Name + "Jsonnet").Call([]reflect.Value{})
			exprMap[&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: mthd.Name,
			}] = ret[0].Interface().(*jsonnet.Expr)
		}
		return &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map:  exprMap,
		}

	case reflect.Pointer:
		return convertIntoJsonnet(reflect.Indirect(v).Interface())
	}

	panic(fmt.Sprintf("not implemented: %v", data))
}

func TestCompileValidTemplates(t *testing.T) {
	tVal := &T{
		I:   17,
		X:   "x",
		U:   &U{V: "v"},
		MSI: map[string]int{"one": 1, "two": 2},
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
		{".Method0", "-{{.Method0}}-", tVal},
		{".Method1(1234)", "-{{.Method1 1234}}-", tVal},
		{".Method1(.I)", "-{{.Method1 .I}}-", tVal},

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
		{
			name: "if .a.b",
			tpl:  `{{ if .a.b }}1{{ else }}0{{ end }}`,
			data: map[string]map[string]any{"a": {"b": false}},
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
				CallArgs: []*jsonnet.Expr{convertIntoJsonnet(tt.data)},
			}

			//log.Printf("%s", jsonnetExpr.StringWithPrologue())

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
}
