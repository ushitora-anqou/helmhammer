package jsonnet

import (
	_ "embed"
	"errors"
	"maps"
	"slices"

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
	CallNamedArgs  []*NamedArg
	IntLiteral     int
	IndexListHead  *Expr
	IndexListTail  []string
	IDName         string
	LocalBinds     []*LocalBind
	LocalBody      *Expr
	Map            []*MapEntry
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
		for _, arg := range e.CallNamedArgs {
			printedArgs = append(printedArgs, fmt.Sprintf("%s=%s", arg.Name, arg.Arg.String()))
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
		for _, entry := range e.Map {
			k := entry.K
			v := entry.V
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

type NamedArg struct {
	Name string
	Arg  *Expr
}

type MapEntry struct {
	K, V *Expr
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
func AddMap(lhs *Expr, rhs []*MapEntry) *Expr {
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

func CallIsTrueOnHeap(heap *Expr, v *Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("isTrueOnHeap"),
		CallArgs: []*Expr{heap, v},
	}
}

func CallJoin(heap *Expr, list []*Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("_join"),
		CallArgs: []*Expr{
			heap,
			{Kind: EList, List: list},
		},
	}
}

func CallRange(args ...*Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("range"),
		CallArgs: args,
	}
}

func CallField(args ...*Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("field"),
		CallArgs: args,
	}
}

func CallFromConst(heap *Expr, v *Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("fromConst"),
		CallArgs: []*Expr{heap, v},
	}
}

func CallAllocate(heap *Expr, v *Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("allocate"),
		CallArgs: []*Expr{heap, v},
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
		exprMap := []*MapEntry{}
		iter := v.MapRange()
		for iter.Next() {
			exprMap = append(exprMap, &MapEntry{
				K: &Expr{
					Kind:          EStringLiteral,
					StringLiteral: iter.Key().Interface().(string),
				},
				V: ConvertIntoJsonnet(iter.Value().Interface()),
			})
		}
		return &Expr{
			Kind: EMap,
			Map:  exprMap,
		}

	case reflect.Struct:
		exprMap := []*MapEntry{}
		ty := v.Type()
		for i := range ty.NumField() {
			field := ty.Field(i)
			if !field.IsExported() {
				continue
			}
			exprMap = append(exprMap, &MapEntry{
				K: &Expr{
					Kind:          EStringLiteral,
					StringLiteral: field.Name,
				},
				V: ConvertIntoJsonnet(v.FieldByIndex(field.Index).Interface()),
			})
		}
		for i := range ty.NumMethod() {
			mthd := ty.Method(i)
			mthdJsonnet := v.MethodByName(mthd.Name + "Jsonnet")
			if !mthdJsonnet.IsValid() || mthdJsonnet.IsZero() {
				continue
			}
			ret := v.MethodByName(mthd.Name + "Jsonnet").Call([]reflect.Value{})
			exprMap = append(exprMap, &MapEntry{
				K: &Expr{
					Kind:          EStringLiteral,
					StringLiteral: mthd.Name,
				},
				V: ret[0].Interface().(*Expr),
			})
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

func CallChartMetadata(
	name, version, appVersion, templateBasePath, condition string,
	renderedKeys []string,
	defaultValues *Expr,
	crds [][]byte,
	compiledFiles map[string]*Expr,
	compiledSubChartMetadata []*Expr,
) *Expr {
	exprKeys := []*Expr{}
	for _, key := range renderedKeys {
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
		CallFunc: Index("chartMetadata"),
		CallArgs: []*Expr{
			{Kind: EStringLiteral, StringLiteral: name},
			{Kind: EStringLiteral, StringLiteral: version},
			{Kind: EStringLiteral, StringLiteral: appVersion},
			{Kind: EStringLiteral, StringLiteral: templateBasePath},
			{Kind: EStringLiteral, StringLiteral: condition},
			{Kind: EList, List: exprKeys},
			defaultValues,
			{Kind: EList, List: crdsList},
			Map(compiledFiles),
			{Kind: EList, List: compiledSubChartMetadata},
		},
	}
}

func CallChartMain(capabilities, rootChart, initialHeap, body *Expr) *Expr {
	return &Expr{
		Kind:     ECall,
		CallFunc: Index("chartMain"),
		CallArgs: []*Expr{capabilities, rootChart, initialHeap, body},
	}
}

func Map(src map[string]*Expr) *Expr {
	m := make([]*MapEntry, 0, len(src))
	for _, k := range slices.Sorted(maps.Keys(src)) {
		m = append(m, &MapEntry{
			K: &Expr{Kind: EStringLiteral, StringLiteral: k},
			V: src[k],
		})
	}
	return &Expr{
		Kind: EMap,
		Map:  m,
	}
}

func EmptyMap() *Expr {
	return Map(map[string]*Expr{})
}

func IndexInt(lhs string, rhs int) *Expr {
	return &Expr{
		Kind:     EIndex,
		BinOpLHS: Index(lhs),
		BinOpRHS: &Expr{
			Kind:       EIntLiteral,
			IntLiteral: rhs,
		},
	}
}

func CallDeref(heap *Expr, v *Expr) *Expr {
	return &Expr{
		Kind: ECall,
		CallFunc: &Expr{
			Kind: ERaw,
			Raw:  "deref",
		},
		CallArgs: []*Expr{heap, v},
	}
}

func CallToConst(heap *Expr, v *Expr) *Expr {
	return &Expr{
		Kind: ECall,
		CallFunc: &Expr{
			Kind: ERaw,
			Raw:  "toConst",
		},
		CallArgs: []*Expr{heap, v},
	}
}

func deepAllocate(heap []*Expr, src any) (*Expr, []*Expr, error) {
	v := reflect.ValueOf(src)

	if !v.IsValid() {
		return &Expr{Kind: ENull}, heap, nil
	}

	switch v.Kind() {
	case reflect.Bool:
		fallthrough
	case reflect.Int:
		fallthrough
	case reflect.Uint16:
		fallthrough
	case reflect.Float64:
		fallthrough
	case reflect.String:
		return ConvertIntoJsonnet(src), heap, nil

	case reflect.Pointer:
		if v.IsNil() {
			return &Expr{Kind: ENull}, heap, nil
		}
		return deepAllocate(heap, reflect.Indirect(v).Interface())

	case reflect.Slice:
		exprs := []*Expr{}
		for i := range v.Len() {
			var expr *Expr
			var err error
			expr, heap, err = deepAllocate(heap, v.Index(i).Interface())
			if err != nil {
				return nil, heap, err
			}
			exprs = append(exprs, expr)
		}
		return deepAllocateCollection(heap, &Expr{Kind: EList, List: exprs})

	case reflect.Map:
		if v.IsNil() {
			return &Expr{Kind: ENull}, heap, nil
		}

		entries := map[string]any{}
		for iter := v.MapRange(); iter.Next(); {
			entries[iter.Key().Interface().(string)] = iter.Value().Interface()
		}

		return deepAllocateMap(heap, entries)

	case reflect.Struct:
		entries := map[string]any{}
		ty := v.Type()
		for i := range ty.NumField() {
			field := ty.Field(i)
			if !field.IsExported() {
				continue
			}
			entries[field.Name] = v.FieldByIndex(field.Index).Interface()
		}

		return deepAllocateMap(heap, entries)
	}

	return nil, heap, errors.New("deepAllocate: unknown type")
}

func deepAllocateMap(heap []*Expr, entries map[string]any) (*Expr, []*Expr, error) {
	exprs := map[string]*Expr{}
	for _, k := range slices.Sorted(maps.Keys(entries)) {
		var expr *Expr
		var err error
		expr, heap, err = deepAllocate(heap, entries[k])
		if err != nil {
			return nil, heap, err
		}
		exprs[k] = expr
	}
	return deepAllocateCollection(heap, Map(exprs))
}

func deepAllocateCollection(heap []*Expr, collection *Expr) (*Expr, []*Expr, error) {
	pointer := len(heap)
	heap = append(heap, collection)
	return Map(
		map[string]*Expr{
			"p": {Kind: EList, List: []*Expr{{
				Kind: EStringLiteral, StringLiteral: strconv.Itoa(pointer),
			}}},
		},
	), heap, nil
}

func DeepAllocate(heap []*Expr, src any) (*Expr, []*Expr, error) {
	return deepAllocate(heap, src)
}
