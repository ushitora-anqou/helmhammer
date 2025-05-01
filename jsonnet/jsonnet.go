package jsonnet

import (
	_ "embed"

	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	EAdd = iota
	ECall
	EFalse
	EFloatLiteral
	EFunction
	EID
	EIf
	EIndex
	EIndexList
	EIntLiteral
	EList
	ELocal
	EMap
	ENull
	ERaw
	EStringLiteral
	ETrue
)

type Expr struct {
	Kind           int
	StringLiteral  string
	List           []*Expr
	IfCond         *Expr
	IfThen         *Expr
	IfElse         *Expr
	CallFunc       *Expr
	CallArgs       []*Expr
	CallNamedArgs  map[string]*Expr
	IntLiteral     int
	IndexListHead  *Expr
	IndexListTail  []string
	IDName         string
	LocalBinds     []*LocalBind
	LocalBody      *Expr
	Map            map[*Expr]*Expr
	FunctionParams []string
	FunctionBody   *Expr
	BinOpLHS       *Expr
	BinOpRHS       *Expr
	FloatLiteral   float64
	Raw            string
}

func (e *Expr) precedence() int {
	switch e.Kind {
	case ERaw:
		fallthrough
	case EFalse:
		fallthrough
	case EID:
		fallthrough
	case EIntLiteral:
		fallthrough
	case EList:
		fallthrough
	case EMap:
		fallthrough
	case ENull:
		fallthrough
	case EStringLiteral:
		fallthrough
	case EFloatLiteral:
		fallthrough
	case ETrue:
		return 0

	case ECall:
		fallthrough
	case EIndex:
		fallthrough
	case EIndexList:
		return -1

	case EAdd:
		return -2

	case EFunction:
		fallthrough
	case EIf:
		fallthrough
	case ELocal:
		return -3
	}
	panic("invalid kind")
}

func wrapParen(b *strings.Builder, base, e *Expr) {
	// We don't have to wrap if equal precedences because everything is left assodiative in Jsonnet.
	shouldWrap := base.precedence() > e.precedence()
	if shouldWrap {
		b.WriteString("(")
	}
	b.WriteString(e.String())
	if shouldWrap {
		b.WriteString(")")
	}
}

func (e *Expr) String() string {
	switch e.Kind {
	case EAdd:
		b := strings.Builder{}
		wrapParen(&b, e, e.BinOpLHS)
		b.WriteString(" + ")
		wrapParen(&b, e, e.BinOpRHS)
		return b.String()

	case ECall:
		b := strings.Builder{}
		wrapParen(&b, e, e.CallFunc)
		b.WriteString("(")

		printedArgs := []string{}
		for _, arg := range e.CallArgs {
			printedArgs = append(printedArgs, arg.String())
		}
		for name, arg := range e.CallNamedArgs {
			printedArgs = append(printedArgs, fmt.Sprintf("%s=%s", name, arg.String()))
		}
		b.WriteString(strings.Join(printedArgs, ", "))

		b.WriteString(")")
		return b.String()

	case EFalse:
		return "false"

	case EFloatLiteral:
		return fmt.Sprintf("%f", e.FloatLiteral)

	case EFunction:
		b := strings.Builder{}
		b.WriteString("function(")
		for i, param := range e.FunctionParams {
			b.WriteString(param)
			if i != len(e.FunctionParams)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString(") ")
		b.WriteString(e.FunctionBody.String())
		return b.String()

	case EID:
		return e.IDName

	case EIf:
		b := strings.Builder{}
		b.WriteString("if ")
		wrapParen(&b, e, e.IfCond)
		b.WriteString(" then ")
		wrapParen(&b, e, e.IfThen)
		b.WriteString(" else ")
		wrapParen(&b, e, e.IfElse)
		return b.String()

	case EIndex:
		b := strings.Builder{}
		wrapParen(&b, e, e.BinOpLHS)
		b.WriteString("[")
		b.WriteString(e.BinOpRHS.String())
		b.WriteString("]")
		return b.String()

	case EIndexList:
		b := strings.Builder{}
		wrapParen(&b, e, e.IndexListHead)
		for _, elm := range e.IndexListTail {
			b.WriteString("[\"")
			b.WriteString(escapeString(elm, false, true))
			b.WriteString("\"]")
		}
		return b.String()

	case EIntLiteral:
		return strconv.Itoa(e.IntLiteral)

	case EList:
		b := strings.Builder{}
		b.WriteString("[")
		for i, head := range e.List {
			b.WriteString(head.String())
			if i != len(e.List)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString("]")
		return b.String()

	case ELocal:
		b := strings.Builder{}
		b.WriteString("local ")
		for i, bind := range e.LocalBinds {
			b.WriteString(bind.Name)
			b.WriteString(" = ")
			b.WriteString(bind.Body.String())
			if i != len(e.LocalBinds)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString("; ")
		b.WriteString(e.LocalBody.String())
		return b.String()

	case EMap:
		b := strings.Builder{}
		b.WriteString("{")
		cnt := 0
		for k, v := range e.Map {
			cnt++
			switch k.Kind {
			case EID:
				b.WriteString(k.IDName)
				b.WriteString(": ")
				b.WriteString(v.String())
				if cnt != len(e.Map) {
					b.WriteString(", ")
				}
			case EStringLiteral:
				b.WriteString("\"")
				b.WriteString(escapeString(k.StringLiteral, false, true))
				b.WriteString("\"")
				b.WriteString(": ")
				b.WriteString(v.String())
				if cnt != len(e.Map) {
					b.WriteString(", ")
				}
			default:
				panic("unimplemented: not string key of map")
			}
		}
		b.WriteString("}")
		return b.String()

	case ENull:
		return "null"

	case ERaw:
		return e.Raw

	case EStringLiteral:
		return fmt.Sprintf("\"%s\"", escapeString(e.StringLiteral, false, true))

	case ETrue:
		return "true"
	}

	panic("Expr.String: invalid kind")
}

type LocalBind struct {
	Name string
	Body *Expr
}

//go:embed prologue.jsonnet
var prologue string

func (e *Expr) StringWithPrologue() string {
	keyword := "// DON'T USE BELOW\n"
	index := strings.Index(prologue, keyword)
	if index == -1 {
		panic("invalid prologue")
	}
	return prologue[0:index] + e.String()
}

var emptyString = &Expr{
	Kind: EStringLiteral,
}

func EmptyString() *Expr {
	return emptyString
}

// get [id].[key0].[key1]. ... .[keyn-1].
// if len(keys) == 0 then return [id].
func Index(id string, keys ...string) *Expr {
	head := &Expr{
		Kind:   EID,
		IDName: id,
	}
	if len(keys) == 0 {
		return head
	}
	return &Expr{
		Kind:          EIndexList,
		IndexListHead: head,
		IndexListTail: keys,
	}
}

// get [lhs] + [rhs], where rhs is a map.
func AddMap(lhs *Expr, rhs map[*Expr]*Expr) *Expr {
	if len(rhs) == 0 {
		return lhs
	}
	return &Expr{
		Kind:     EAdd,
		BinOpLHS: lhs,
		BinOpRHS: &Expr{
			Kind: EMap,
			Map:  rhs,
		},
	}
}

func CallIsTrue(v *Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("helmhammer", "isTrue"),
		CallArgs: []*Expr{v},
	}
}

func CallJoin(list []*Expr) *Expr {
	return &Expr{
		Kind: ECall,
		CallFunc: &Expr{
			Kind:          EIndexList,
			IndexListHead: &Expr{Kind: EID, IDName: "helmhammer"},
			IndexListTail: []string{"join"},
		},
		CallArgs: []*Expr{
			{Kind: EList, List: list},
		},
	}
}

func CallRange(args ...*Expr) *Expr {
	return &Expr{
		Kind: ECall,
		CallFunc: &Expr{
			Kind:          EIndexList,
			IndexListHead: &Expr{Kind: EID, IDName: "helmhammer"},
			IndexListTail: []string{"range"},
		},
		CallArgs: args,
	}
}

func CallField(args ...*Expr) *Expr {
	return &Expr{
		Kind: ECall,
		CallFunc: &Expr{
			Kind:          EIndexList,
			IndexListHead: &Expr{Kind: EID, IDName: "helmhammer"},
			IndexListTail: []string{"field"},
		},
		CallArgs: args,
	}
}

func ConvertIntoJsonnet(data any) *Expr {
	v := reflect.ValueOf(data)

	if !v.IsValid() {
		return &Expr{Kind: ENull}
	}

	switch v.Kind() {
	case reflect.Bool:
		kind := EFalse
		if v.Bool() {
			kind = ETrue
		}
		return &Expr{Kind: kind}

	case reflect.Int:
		return &Expr{Kind: EIntLiteral, IntLiteral: int(v.Int())}

	case reflect.Uint16:
		return &Expr{Kind: EIntLiteral, IntLiteral: int(v.Uint())}

	case reflect.Float64:
		return &Expr{Kind: EFloatLiteral, FloatLiteral: v.Float()}

	case reflect.String:
		return &Expr{Kind: EStringLiteral, StringLiteral: v.String()}

	case reflect.Map:
		if v.IsNil() {
			return &Expr{Kind: ENull}
		}
		exprMap := map[*Expr]*Expr{}
		iter := v.MapRange()
		for iter.Next() {
			exprMap[&Expr{
				Kind:          EStringLiteral,
				StringLiteral: iter.Key().Interface().(string),
			}] = ConvertIntoJsonnet(iter.Value().Interface())
		}
		return &Expr{
			Kind: EMap,
			Map:  exprMap,
		}

	case reflect.Struct:
		exprMap := map[*Expr]*Expr{}
		ty := v.Type()
		for i := range ty.NumField() {
			field := ty.Field(i)
			if !field.IsExported() {
				continue
			}
			exprMap[&Expr{
				Kind:          EStringLiteral,
				StringLiteral: field.Name,
			}] = ConvertIntoJsonnet(v.FieldByIndex(field.Index).Interface())
		}
		for i := range ty.NumMethod() {
			mthd := ty.Method(i)
			mthdJsonnet := v.MethodByName(mthd.Name + "Jsonnet")
			if !mthdJsonnet.IsValid() || mthdJsonnet.IsZero() {
				continue
			}
			ret := v.MethodByName(mthd.Name + "Jsonnet").Call([]reflect.Value{})
			exprMap[&Expr{
				Kind:          EStringLiteral,
				StringLiteral: mthd.Name,
			}] = ret[0].Interface().(*Expr)
		}
		return &Expr{
			Kind: EMap,
			Map:  exprMap,
		}

	case reflect.Pointer:
		if v.IsNil() {
			return &Expr{Kind: ENull}
		}
		return ConvertIntoJsonnet(reflect.Indirect(v).Interface())

	case reflect.Slice:
		if v.IsNil() {
			return &Expr{Kind: ENull}
		}
		list := []*Expr{}
		for i := range v.Len() {
			list = append(list, ConvertIntoJsonnet(v.Index(i).Interface()))
		}
		return &Expr{
			Kind: EList,
			List: list,
		}
	}

	panic(fmt.Sprintf("not implemented: %v", data))
}

func IdentityFunction() *Expr {
	return &Expr{
		Kind: ERaw,
		Raw:  `function(x) x`,
	}
}

func escapeString(s string, escapeSingleQuote bool, escapeDoubleQuote bool) string {
	var b strings.Builder
	buf := []byte(s)
	for _, ch := range buf {
		switch ch {
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case '"':
			if escapeDoubleQuote {
				b.WriteString("\\\"")
			} else {
				b.WriteByte(ch)
			}
		case '\'':
			if escapeSingleQuote {
				b.WriteString("\\'")
			} else {
				b.WriteByte(ch)
			}
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func CallChartMain(
	chartName, chartVersion, chartAppVersion string,
	releaseName, releaseService string,
	keys []string, defaultValues *Expr,
	crds [][]byte, body *Expr) *Expr {
	exprKeys := []*Expr{}
	for _, key := range keys {
		exprKeys = append(exprKeys, &Expr{
			Kind:          EStringLiteral,
			StringLiteral: key,
		})
	}

	crdsList := []*Expr{}
	for _, crd := range crds {
		crdsList = append(crdsList, &Expr{
			Kind:          EStringLiteral,
			StringLiteral: string(crd),
		})
	}

	return &Expr{
		Kind:     ECall,
		CallFunc: Index("helmhammer", "chartMain"),
		CallArgs: []*Expr{
			{Kind: EStringLiteral, StringLiteral: chartName},
			{Kind: EStringLiteral, StringLiteral: chartVersion},
			{Kind: EStringLiteral, StringLiteral: chartAppVersion},
			{Kind: EStringLiteral, StringLiteral: releaseName},
			{Kind: EStringLiteral, StringLiteral: releaseService},
			{Kind: EList, List: exprKeys},
			defaultValues,
			{Kind: EList, List: crdsList},
			body,
		},
	}
}

func PredefinedFunctions() map[string]*Expr {
	m := map[string]*Expr{}
	for _, name := range []string{
		"and",
		"concat",
		"contains",
		"default",
		"dir",
		"eq",
		"indent",
		"list",
		"lower",
		"ne",
		"nindent",
		"not",
		"or",
		"print",
		"printf",
		"quote",
		"replace",
		"required",
		"sha256sum",
		"toYaml",
		"trimSuffix",
		"trunc",
	} {
		m[name] = Index("helmhammer", name)
	}
	return m
}
