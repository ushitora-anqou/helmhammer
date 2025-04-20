package compiler

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/template/parse"

	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

var nextGenID = 0

func genid() int {
	nextGenID++
	return nextGenID
}

type stateName = string

func generateStateName() stateName {
	return fmt.Sprintf("state%d", genid())
}

func stateValue(v *jsonnet.Expr, vs *jsonnet.Expr) *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.EMap,
		Map: map[*jsonnet.Expr]*jsonnet.Expr{
			{Kind: jsonnet.EID, IDName: "v"}:  v,
			{Kind: jsonnet.EID, IDName: "vs"}: vs,
		},
	}
}

type state struct {
	name stateName

	// body should be a value of a map like { v: ..., vs: ... }.
	// v shoule be the printed value, and vs should be the variable map.
	body *jsonnet.Expr
}

func (s *state) toLocal(body *jsonnet.Expr) *jsonnet.Expr {
	// local [s.name] = [s.body]; [body]
	return &jsonnet.Expr{
		Kind: jsonnet.ELocal,
		LocalBinds: []*jsonnet.LocalBind{
			{
				Name: s.name,
				Body: s.body,
			},
		},
		LocalBody: body,
	}
}

type scope struct {
	parent    *scope
	variables map[string]*variable
}

type scopeT = scope

type variable struct {
	defined bool
}

func (sc *scope) defineVariable(name string) error {
	_, ok := sc.variables[name]
	if ok {
		return fmt.Errorf("variable already defined: %s", name)
	}
	sc.variables[name] = &variable{
		defined: true,
	}
	return nil
}

func (sc *scope) assignVariable(name string) error {
	if _, ok := sc.variables[name]; ok { // defined or assigned in this scope
		return nil
	}
	if sc.parent != nil {
		if _, ok := sc.parent.getVariable(name); !ok {
			return fmt.Errorf("variable not found: %s", name)
		}
	}
	sc.variables[name] = &variable{
		defined: false,
	}
	return nil
}

func (sc *scope) getVariable(name string) (*variable, bool) {
	v, ok := sc.variables[name]
	if ok {
		return v, true
	}
	if sc.parent == nil {
		return nil, false
	}
	return sc.parent.getVariable(name)
}

func withScope(
	parent *scope,
	preStateName stateName,
	nested func(scope *scope) (*state, error),
) (*state, error) {
	env := &scope{
		parent:    parent,
		variables: make(map[string]*variable),
	}
	nestedPostState, err := nested(env)
	if err != nil {
		return nil, err
	}

	// { ..., NAME: [preStateName].vs.[name], ... }
	// if v is assigned in the scope
	// for (name, v) in env.variables
	assignedVars := make(map[*jsonnet.Expr]*jsonnet.Expr)
	for name, v := range env.variables {
		if v.defined {
			continue
		}
		// { ..., NAME: sX.vs.NAME, ... }
		assignedVars[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: name,
		}] = compileIDDotKeys(nestedPostState.name, "vs", name)
	}

	// local [nestedPostState.name] = [nestedPostState.body];
	// { v: [nestedPostState.v], vs: [preStateName].vs + [assignedVars] }
	expr := *nestedPostState.toLocal(
		stateValue(
			compileIDDotKeys(nestedPostState.name, "v"),
			compileAddMap(compileIDDotKeys(preStateName, "vs"), assignedVars),
		),
	)

	return &state{name: generateStateName(), body: &expr}, nil
}

func Compile(node parse.Node) (*jsonnet.Expr, error) {
	initialStateName := generateStateName()
	postState, err := withScope(
		nil,
		initialStateName,
		func(scope *scope) (*state, error) {
			if err := scope.defineVariable("$"); err != nil {
				return nil, err
			}
			return compileNode(scope, initialStateName, node)
		},
	)
	if err != nil {
		return nil, err
	}

	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"values"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ELocal,
			LocalBinds: []*jsonnet.LocalBind{
				{
					Name: "helmhammer",
					Body: &jsonnet.Expr{
						Kind: jsonnet.EAdd,
						BinOpLHS: &jsonnet.Expr{
							Kind:   jsonnet.EID,
							IDName: "helmhammer0",
						},
						BinOpRHS: &jsonnet.Expr{
							Kind: jsonnet.EMap,
							Map: map[*jsonnet.Expr]*jsonnet.Expr{
								{Kind: jsonnet.EStringLiteral, StringLiteral: "dot"}: {
									Kind:   jsonnet.EID,
									IDName: "values",
								},
							},
						},
					},
				},
				{
					Name: initialStateName,
					Body: stateValue(
						jsonnet.EmptyString(),
						&jsonnet.Expr{
							Kind: jsonnet.EMap,
							Map: map[*jsonnet.Expr]*jsonnet.Expr{
								{Kind: jsonnet.EStringLiteral, StringLiteral: "$"}: compileDot(),
							},
						},
					),
				},
			},
			LocalBody: &jsonnet.Expr{
				Kind:          jsonnet.EIndexList,
				IndexListHead: postState.body,
				IndexListTail: []string{"v"},
			},
		},
	}, nil
}

func compileNode(scope *scope, preStateName string, node parse.Node) (*state, error) {
	postStateName := generateStateName()

	switch node := node.(type) {
	case *parse.ActionNode:
		vExpr, vsExpr, err := compilePipeline(scope, preStateName, node.Pipe)
		if err != nil {
			return nil, err
		}
		if len(node.Pipe.Decl) == 0 {
			return &state{
				name: postStateName,
				body: stateValue(vExpr, vsExpr),
			}, nil
		}
		return &state{
			name: postStateName,
			body: stateValue(jsonnet.EmptyString(), vsExpr),
		}, nil

	case *parse.BreakNode:
	case *parse.CommentNode:
	case *parse.ContinueNode:

	case *parse.IfNode:
		vExpr, vsExpr, err := compilePipeline(scope, preStateName, node.Pipe)
		if err != nil {
			return nil, err
		}
		nestedPreState := &state{
			name: generateStateName(),
			body: stateValue(vExpr, vsExpr),
		}

		nestedPostStateThen, err := withScope(
			scope,
			nestedPreState.name,
			func(scope *scopeT) (*state, error) {
				return compileNode(scope, nestedPreState.name, node.List)
			},
		)

		var nestedPostStateElse *state
		if node.ElseList == nil {
			nestedPostStateElse = &state{
				name: generateStateName(),
				body: &jsonnet.Expr{
					Kind:   jsonnet.EID,
					IDName: nestedPreState.name,
				},
			}
		} else {
			nestedPostStateElse, err = withScope(
				scope,
				nestedPreState.name,
				func(scope *scopeT) (*state, error) {
					return compileNode(scope, nestedPreState.name, node.ElseList)
				},
			)
			if err != nil {
				return nil, err
			}
		}

		return &state{
			name: postStateName,
			body: nestedPreState.toLocal(
				&jsonnet.Expr{
					Kind:   jsonnet.EIf,
					IfCond: compileIDDotKeys(nestedPreState.name, "v"),
					IfThen: nestedPostStateThen.body,
					IfElse: nestedPostStateElse.body,
				},
			),
		}, nil

	case *parse.ListNode:
		states := []*state{}
		varsToBeJoined := []*jsonnet.Expr{}
		stateName := preStateName
		for _, node := range node.Nodes {
			newState, err := compileNode(scope, stateName, node)
			if err != nil {
				return nil, err
			}
			states = append(states, newState)
			varsToBeJoined = append(varsToBeJoined, compileIDDotKeys(newState.name, "v"))
			stateName = newState.name
		}
		body := stateValue(
			&jsonnet.Expr{
				Kind: jsonnet.ECall,
				CallFunc: &jsonnet.Expr{
					Kind:          jsonnet.EIndexList,
					IndexListHead: &jsonnet.Expr{Kind: jsonnet.EID, IDName: "helmhammer"},
					IndexListTail: []string{"join"},
				},
				CallArgs: []*jsonnet.Expr{
					{Kind: jsonnet.EList, List: varsToBeJoined},
				},
			},
			compileIDDotKeys(stateName, "vs"),
		)
		for i := len(states) - 1; i >= 0; i-- {
			body = states[i].toLocal(body)
		}
		return &state{name: postStateName, body: body}, nil

	case *parse.RangeNode:
	case *parse.TemplateNode:

	case *parse.TextNode:
		return &state{
			name: postStateName,
			body: stateValue(
				&jsonnet.Expr{
					Kind:          jsonnet.EStringLiteral,
					StringLiteral: string(node.Text),
				},
				compileIDDotKeys(preStateName, "vs"),
			),
		}, nil

	case *parse.WithNode:
	}
	return nil, fmt.Errorf("unknown node: %v", reflect.ValueOf(node).Type())
}

func compilePipeline(scope *scope, preStateName string, pipe *parse.PipeNode) (*jsonnet.Expr, *jsonnet.Expr, error) {
	if pipe == nil {
		return nil, nil, errors.New("pipe is nil")
	}

	var expr *jsonnet.Expr
	for _, cmd := range pipe.Cmds {
		var err error
		expr, err = compileCommand(scope, preStateName, cmd, expr)
		if err != nil {
			return nil, nil, err
		}
	}

	assignments := map[*jsonnet.Expr]*jsonnet.Expr{}
	for _, variable := range pipe.Decl {
		if pipe.IsAssign {
			scope.assignVariable(variable.Ident[0])
		} else {
			scope.defineVariable(variable.Ident[0])
		}
		assignments[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: variable.Ident[0],
		}] = expr
	}

	vsExpr := compileAddMap(compileIDDotKeys(preStateName, "vs"), assignments)

	return expr, vsExpr, nil
}

func compileCommand(scope *scope, preStateName string, cmd *parse.CommandNode, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	switch node := cmd.Args[0].(type) {
	case *parse.FieldNode:
		return compileField(compileDot(), node.Ident, cmd.Args, final)

	case *parse.ChainNode:
	case *parse.IdentifierNode:
	case *parse.PipeNode:

	case *parse.VariableNode:
		_, ok := scope.getVariable(node.Ident[0])
		if !ok {
			return nil, fmt.Errorf("variable not found: %s", node.Ident[0])
		}
		receiver := compileIDDotKeys(preStateName, "vs", node.Ident[0])
		if len(node.Ident) == 1 {
			return receiver, nil
		}
		return compileField(receiver, node.Ident[1:], cmd.Args, final)

	case *parse.BoolNode:
		return compileBool(node)

	case *parse.DotNode:
		return compileDot(), nil

	case *parse.NilNode:
		return nil, errors.New("nil is not a command")

	case *parse.NumberNode:
		return compileNumber(node)

	case *parse.StringNode:
		return compileString(node)
	}
	return nil, fmt.Errorf("unknown command: %v", reflect.ValueOf(cmd.Args[0]).Type())
}

func isRuneInt(s string) bool {
	return len(s) > 0 && s[0] == '\''
}

func isHexInt(s string) bool {
	return len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') &&
		!strings.ContainsAny(s, "pP")
}

func compileArg(arg parse.Node) (*jsonnet.Expr, error) {
	switch node := arg.(type) {
	case *parse.DotNode:
		return compileDot(), nil

	case *parse.NilNode:
		return compileNil(), nil

	case *parse.FieldNode:
		return compileField(compileDot(), node.Ident, []parse.Node{arg}, nil)

	case *parse.VariableNode:
	case *parse.PipeNode:
	case *parse.IdentifierNode:
	case *parse.ChainNode:

	case *parse.BoolNode:
		return compileBool(node)

	case *parse.NumberNode:
		return compileNumber(node)

	case *parse.StringNode:
		return compileString(node)
	}

	return nil, nil
}

func compileField(
	receiver *jsonnet.Expr,
	ident []string,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, error) {
	if len(ident) >= 2 {
		receiver = &jsonnet.Expr{
			Kind:          jsonnet.EIndexList,
			IndexListHead: receiver,
			IndexListTail: ident[0 : len(ident)-1],
		}
	}

	compiledArgs := []*jsonnet.Expr{}
	for i, arg := range args {
		if i == 0 {
			continue
		}
		compiledArg, err := compileArg(arg)
		if err != nil {
			return nil, err
		}
		compiledArgs = append(compiledArgs, compiledArg)
	}
	if final != nil {
		compiledArgs = append(compiledArgs, final)
	}
	return &jsonnet.Expr{
		Kind: jsonnet.ECall,
		CallFunc: &jsonnet.Expr{
			Kind: jsonnet.EIndexList,
			IndexListHead: &jsonnet.Expr{
				Kind:   jsonnet.EID,
				IDName: "helmhammer",
			},
			IndexListTail: []string{"field"},
		},
		CallArgs: []*jsonnet.Expr{
			receiver,
			{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: ident[len(ident)-1],
			},
			{
				Kind: jsonnet.EList,
				List: compiledArgs,
			},
		},
	}, nil
}

func compileBool(node *parse.BoolNode) (*jsonnet.Expr, error) {
	if node.True {
		return &jsonnet.Expr{Kind: jsonnet.ETrue}, nil
	}
	return &jsonnet.Expr{Kind: jsonnet.EFalse}, nil
}

func compileNumber(node *parse.NumberNode) (*jsonnet.Expr, error) {
	switch {
	case node.IsComplex:
		return nil, errors.New("complex is not implemented")
	case node.IsFloat &&
		!isHexInt(node.Text) && !isRuneInt(node.Text) &&
		strings.ContainsAny(node.Text, ".eEpP"):
		return nil, errors.New("float is not implemented")
	case node.IsInt:
		n := int(node.Int64)
		if int64(n) != node.Int64 {
			return nil, fmt.Errorf("%s overflows int", node.Text)
		}
		return &jsonnet.Expr{Kind: jsonnet.EIntLiteral, IntLiteral: n}, nil
	case node.IsUint:
		return nil, errors.New("uint is not implemented")
	}
	return nil, errors.New("invalid number")
}

func compileString(node *parse.StringNode) (*jsonnet.Expr, error) {
	return &jsonnet.Expr{Kind: jsonnet.EStringLiteral, StringLiteral: node.Text}, nil
}

func compileDot() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.EIndexList,
		IndexListHead: &jsonnet.Expr{
			Kind:   jsonnet.EID,
			IDName: "helmhammer",
		},
		IndexListTail: []string{"dot"},
	}
}

func compileNil() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ENull,
	}
}

// get [id].[key0].[key1]. ... .[keyn-1].
// if len(keys) == 0 then return [id].
func compileIDDotKeys(id string, keys ...string) *jsonnet.Expr {
	head := &jsonnet.Expr{
		Kind:   jsonnet.EID,
		IDName: id,
	}
	if len(keys) == 0 {
		return head
	}
	return &jsonnet.Expr{
		Kind:          jsonnet.EIndexList,
		IndexListHead: head,
		IndexListTail: keys,
	}
}

func compileAddMap(lhs *jsonnet.Expr, rhs map[*jsonnet.Expr]*jsonnet.Expr) *jsonnet.Expr {
	if len(rhs) == 0 {
		return lhs
	}
	return &jsonnet.Expr{
		Kind:     jsonnet.EAdd,
		BinOpLHS: lhs,
		BinOpRHS: &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map:  rhs,
		},
	}
}
