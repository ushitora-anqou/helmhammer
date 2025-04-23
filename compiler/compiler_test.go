package compiler_test

import (
	"fmt"
	"log"
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
	I        int
	U16      uint16
	X        string
	U        *U
	MSI      map[string]int
	MSIEmpty map[string]int
	MSIone   map[string]int
	SI       []int
	SIEmpty  []int
	SB       []bool
	Empty0   any
	Empty3   any
	PSI      *[]int
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

func (t T) Method2(a uint16, b string) string {
	return fmt.Sprintf("Method2: %d %s", a, b)
}

func (t T) Method2Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ERaw,
			Raw:  `"Method2: %d %s" % [args[0], args[1]]`,
		},
	}
}

func (t T) Method3(v any) string {
	return fmt.Sprintf("Method3: %v", v)
}

func (t T) Method3Jsonnet() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"args"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ERaw,
			Raw: `"Method3: %s" % [(
				if args[0] == null then "<nil>"
				else error "not implemented"
			)]`,
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
		Raw:  `function(args) std.map(function(x) x + args[0], args[1])`,
	}
}

func newIntSlice(n ...int) *[]int {
	p := new([]int)
	*p = make([]int, len(n))
	copy(*p, n)
	return p
}

func TestCompileValidTemplates(t *testing.T) {
	tVal := &T{
		I:      17,
		U16:    16,
		X:      "x",
		U:      &U{V: "v"},
		MSI:    map[string]int{"one": 1, "two": 2, "three": 3},
		MSIone: map[string]int{"one": 1},
		SI:     []int{3, 4, 5},
		SB:     []bool{true, false},
		Empty3: []int{7, 8},
		PSI:    newIntSlice(21, 22, 23),
	}

	// The following test table comes from Go compiler's test code:
	// 	https://cs.opensource.google/go/go/+/refs/tags/go1.24.2:src/text/template/exec_test.go
	tests := []struct {
		name string
		tpl  string
		data any
	}{
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
		{".Method2(3, .X)", "-{{.Method2 3 .X}}-", tVal},
		{".Method2(.U16, `str`)", "-{{.Method2 .U16 `str`}}-", tVal},
		{".Method2(.U16, $x)", "{{if $x := .X}}-{{.Method2 .U16 $x}}{{end}}-", tVal},
		{".Method3(nil constant)", "-{{.Method3 nil}}-", tVal},
		{"method on var", "{{if $x := .}}-{{$x.Method2 .U16 $x.X}}{{end}}-", tVal},

		// Range.
		{"range []int", "{{range .SI}}-{{.}}-{{end}}", tVal},
		{"range empty no else", "{{range .SIEmpty}}-{{.}}-{{end}}", tVal},
		{"range []int else", "{{range .SI}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty else", "{{range .SIEmpty}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		//{"range []int break else", "{{range .SI}}-{{.}}-{{break}}NOTREACHED{{else}}EMPTY{{end}}", tVal},
		//{"range []int continue else", "{{range .SI}}-{{.}}-{{continue}}NOTREACHED{{else}}EMPTY{{end}}", "-3--4--5-", tVal, true},
		{"range []bool", "{{range .SB}}-{{.}}-{{end}}", tVal},
		{"range []int method", "{{range .SI | .MAdd .I}}-{{.}}-{{end}}", tVal},
		{"range map", "{{range .MSI}}-{{.}}-{{end}}", tVal},
		{"range empty map no else", "{{range .MSIEmpty}}-{{.}}-{{end}}", tVal},
		{"range map else", "{{range .MSI}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty map else", "{{range .MSIEmpty}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty interface", "{{range .Empty3}}-{{.}}-{{else}}EMPTY{{end}}", tVal},
		{"range empty nil", "{{range .Empty0}}-{{.}}-{{end}}", tVal},
		{"range $x SI", "{{range $x := .SI}}<{{$x}}>{{end}}", tVal},
		{"range $x $y SI", "{{range $x, $y := .SI}}<{{$x}}={{$y}}>{{end}}", tVal},
		{"range $x MSIone", "{{range $x := .MSIone}}<{{$x}}>{{end}}", tVal},
		{"range $x $y MSIone", "{{range $x, $y := .MSIone}}<{{$x}}={{$y}}>{{end}}", tVal},
		{"range $x PSI", "{{range $x := .PSI}}<{{$x}}>{{end}}", tVal},
		{"declare in range", "{{range $x := .PSI}}<{{$foo:=$x}}{{$x}}>{{end}}", tVal},
		//{"range count", `{{range $i, $x := count 5}}[{{$i}}]{{$x}}{{end}}`, "[0]a[1]b[2]c[3]d[4]e", tVal, true},
		//{"range nil count", `{{range $i, $x := count 0}}{{else}}empty{{end}}`, "empty", tVal, true},
		//{"range iter.Seq[int]", `{{range $i := .}}{{$i}}{{end}}`, "01", fVal1(2), true},
		//{"i = range iter.Seq[int]", `{{$i := 0}}{{range $i = .}}{{$i}}{{end}}`, "01", fVal1(2), true},
		//{"range iter.Seq[int] over two var", `{{range $i, $c := .}}{{$c}}{{end}}`, "", fVal1(2), false},
		//{"i, c := range iter.Seq2[int,int]", `{{range $i, $c := .}}{{$i}}{{$c}}{{end}}`, "0112", fVal2(2), true},
		//{"i, c = range iter.Seq2[int,int]", `{{$i := 0}}{{$c := 0}}{{range $i, $c = .}}{{$i}}{{$c}}{{end}}`, "0112", fVal2(2), true},
		//{"i = range iter.Seq2[int,int]", `{{$i := 0}}{{range $i = .}}{{$i}}{{end}}`, "01", fVal2(2), true},
		//{"i := range iter.Seq2[int,int]", `{{range $i := .}}{{$i}}{{end}}`, "01", fVal2(2), true},
		//{"i,c,x range iter.Seq2[int,int]", `{{$i := 0}}{{$c := 0}}{{$x := 0}}{{range $i, $c = .}}{{$i}}{{$c}}{{end}}`, "0112", fVal2(2), true},
		//{"i,x range iter.Seq[int]", `{{$i := 0}}{{$x := 0}}{{range $i = .}}{{$i}}{{end}}`, "01", fVal1(2), true},
		//{"range iter.Seq[int] else", `{{range $i := .}}{{$i}}{{else}}empty{{end}}`, "empty", fVal1(0), true},
		//{"range iter.Seq2[int,int] else", `{{range $i := .}}{{$i}}{{else}}empty{{end}}`, "empty", fVal2(0), true},
		//{"range int8", rangeTestInt, rangeTestData[int8](), int8(5), true},
		//{"range int16", rangeTestInt, rangeTestData[int16](), int16(5), true},
		//{"range int32", rangeTestInt, rangeTestData[int32](), int32(5), true},
		//{"range int64", rangeTestInt, rangeTestData[int64](), int64(5), true},
		//{"range int", rangeTestInt, rangeTestData[int](), int(5), true},
		//{"range uint8", rangeTestInt, rangeTestData[uint8](), uint8(5), true},
		//{"range uint16", rangeTestInt, rangeTestData[uint16](), uint16(5), true},
		//{"range uint32", rangeTestInt, rangeTestData[uint32](), uint32(5), true},
		//{"range uint64", rangeTestInt, rangeTestData[uint64](), uint64(5), true},
		//{"range uint", rangeTestInt, rangeTestData[uint](), uint(5), true},
		//{"range uintptr", rangeTestInt, rangeTestData[uintptr](), uintptr(5), true},
		//{"range uintptr(0)", `{{range $v := .}}{{print $v}}{{else}}empty{{end}}`, "empty", uintptr(0), true},
		{"range 5", `{{range $v := 5}}{{printf "%d" $v}}{{end}}`, nil},
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
				CallArgs: []*jsonnet.Expr{jsonnet.ConvertIntoJsonnet(tt.data)},
			}

			log.Printf("%s", jsonnetExpr.StringWithPrologue())

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
			require.NoError(t, err)
			got = strings.Trim(got, "\n")
			assert.Equal(t, expected, got)
		})
	}
}
