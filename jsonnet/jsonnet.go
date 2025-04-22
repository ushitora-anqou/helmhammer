package jsonnet

import (
	"fmt"
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
		for i, arg := range e.CallArgs {
			b.WriteString(arg.String())
			if i != len(e.CallArgs)-1 {
				b.WriteString(", ")
			}
		}
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
			b.WriteString(elm) // FIXME escape
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
				b.WriteString(k.IDName) // FIXME: escape
				b.WriteString(": ")
				b.WriteString(v.String())
				if cnt != len(e.Map) {
					b.WriteString(", ")
				}
			case EStringLiteral:
				b.WriteString("\"")
				b.WriteString(k.StringLiteral) // FIXME: escape
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
		return fmt.Sprintf("\"%s\"", e.StringLiteral) // FIXME: escape

	case ETrue:
		return "true"
	}

	panic("Expr.String: invalid kind")
}

type LocalBind struct {
	Name string
	Body *Expr
}

func (e *Expr) StringWithPrologue() string {
	return fmt.Sprintf(`
local helmhammer = {
	field(receiver, fieldName, args):
		if std.isFunction(receiver[fieldName]) then receiver[fieldName](args)
		else receiver[fieldName],

	join(ary):
		std.join("", std.map(std.toString, ary)),

	isTrue(v):
		if std.isArray(v) || std.isObject(v) || std.isString(v) then std.length(v) > 0
		else if std.isBoolean(v) then v
		else if std.isFunction(v) then v != null
		else if std.isNumber(v) then v != 0
		else true,

	range(state, values, f):
		if values == null then state
		else if std.isArray(values) then
			std.foldl(
				function(acc, value)
					local postState = f(acc.state, acc.i, value);
					{
						i: acc.i + 1,
						state: {
							v: acc.state.v + postState.v,
							vs: postState.vs,
						},
					},
				values,
				{
					i: 0,
					state: state,
				},
			).state
		else error "range: not implemented"
};
%s
`, e.String())
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
