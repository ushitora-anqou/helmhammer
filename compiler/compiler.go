package compiler

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

var nextGenID = 0

func genid() int {
	nextGenID++
	return nextGenID
}

// state is output of execution of a text/template's Node.
// It has a printed value (`v`) and a variable map `vs` (name |-> value)
// after its execution.
type state struct {
	body *jsonnet.Expr
}

func newState(v *jsonnet.Expr, vs *jsonnet.Expr) *state {
	return &state{
		body: &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map: map[*jsonnet.Expr]*jsonnet.Expr{
				stringLiteralStateV:  v,
				stringLiteralStateVS: vs,
			},
		},
	}
}

const (
	stateV  = "v"
	stateVS = "vs"
)

var (
	stringLiteralStateV  = &jsonnet.Expr{Kind: jsonnet.EStringLiteral, StringLiteral: stateV}
	stringLiteralStateVS = &jsonnet.Expr{Kind: jsonnet.EStringLiteral, StringLiteral: stateVS}
)

type stateName = string

func generateStateName() stateName {
	return fmt.Sprintf("s%d", genid())
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
	nestedPostStateName := generateStateName()

	// { ..., NAME: [preStateName].vs.[name], ... }
	// if v is assigned in the scope
	// for (name, v) in env.variables
	assignedVars := make(map[*jsonnet.Expr]*jsonnet.Expr)
	for name, v := range env.variables {
		if v.defined {
			continue
		}

		// propagate assignments to the parent scope
		if err := parent.assignVariable(name); err != nil {
			return nil, err
		}

		// { ..., NAME: sX.vs.NAME, ... }
		assignedVars[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: name,
		}] = jsonnet.Index(nestedPostStateName, stateVS, name)
	}

	// local [nestedPostState.name] = [nestedPostState.body];
	// { v: [nestedPostState.v], vs: [preStateName].vs + [assignedVars] }
	expr := &jsonnet.Expr{
		Kind: jsonnet.ELocal,
		LocalBinds: []*jsonnet.LocalBind{
			{Name: nestedPostStateName, Body: nestedPostState.body},
		},
		LocalBody: newState(
			jsonnet.Index(nestedPostStateName, stateV),
			jsonnet.AddMap(jsonnet.Index(preStateName, stateVS), assignedVars),
		).body,
	}

	return &state{body: expr}, nil
}

func Compile(tmpl0 *template.Template) (*jsonnet.Expr, error) {
	globalVariables := map[string]*jsonnet.Expr{
		"printf":     jsonnet.Index("helmhammer", "printf"),
		"include":    jsonnet.Index("helmhammer", "include"),
		"nindent":    jsonnet.Index("helmhammer", "nindent"),
		"not":        jsonnet.Index("helmhammer", "not"),
		"quote":      jsonnet.Index("helmhammer", "quote"),
		"default":    jsonnet.Index("helmhammer", "default"),
		"replace":    jsonnet.Index("helmhammer", "replace"),
		"trunc":      jsonnet.Index("helmhammer", "trunc"),
		"toYaml":     jsonnet.Index("helmhammer", "toYaml"),
		"trimSuffix": jsonnet.Index("helmhammer", "trimSuffix"),
		"contains":   jsonnet.Index("helmhammer", "contains"),
	}
	initialStateName := generateStateName()

	globalScope := &scope{
		parent:    nil,
		variables: map[string]*variable{},
	}
	for key := range globalVariables {
		if err := globalScope.defineVariable(key); err != nil {
			return nil, err
		}
	}

	compiledTemplates := map[*jsonnet.Expr]*jsonnet.Expr{}
	for _, tmpl := range tmpl0.Templates() {
		compiledTemplate, err := compile(globalScope, initialStateName, tmpl0, tmpl.Root)
		if err != nil {
			return nil, err
		}
		compiledTemplates[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: tmpl.Name(),
		}] = compiledTemplate
	}

	compiledGlobalVariables := map[*jsonnet.Expr]*jsonnet.Expr{}
	for key, value := range globalVariables {
		compiledGlobalVariables[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: key,
		}] = value
	}

	return &jsonnet.Expr{
		Kind: jsonnet.ELocal,
		LocalBinds: []*jsonnet.LocalBind{
			{
				Name: initialStateName,
				Body: newState(
					jsonnet.EmptyString(),
					&jsonnet.Expr{
						Kind: jsonnet.EMap,
						Map:  compiledGlobalVariables,
					},
				).body,
			},
		},
		LocalBody: &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map:  compiledTemplates,
		},
	}, nil
}

func compile(scope *scopeT, preStateName stateName, tmpl *template.Template, node parse.Node) (*jsonnet.Expr, error) {
	preDefinedVariablesSrc := map[string]*jsonnet.Expr{
		"$": compileDot(),
	}

	initialStateName := generateStateName()
	postState, err := withScope(
		scope,
		initialStateName,
		func(scope *scopeT) (*state, error) {
			for key := range preDefinedVariablesSrc {
				if err := scope.defineVariable(key); err != nil {
					return nil, err
				}
			}
			return compileNode(tmpl, scope, initialStateName, node)
		},
	)
	if err != nil {
		return nil, err
	}

	preDefinedVariables := map[*jsonnet.Expr]*jsonnet.Expr{}
	for key, value := range preDefinedVariablesSrc {
		preDefinedVariables[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: key,
		}] = value
	}

	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"dot"},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ELocal,
			LocalBinds: []*jsonnet.LocalBind{
				{
					Name: initialStateName,
					Body: newState(
						jsonnet.EmptyString(),
						jsonnet.AddMap(jsonnet.Index(preStateName, stateVS), preDefinedVariables),
					).body,
				},
			},
			LocalBody: &jsonnet.Expr{
				Kind:          jsonnet.EIndexList,
				IndexListHead: postState.body,
				IndexListTail: []string{stateV},
			},
		},
	}, nil
}

func compileNode(tmpl *template.Template, scope *scope, preStateName stateName, node parse.Node) (*state, error) {
	switch node := node.(type) {
	case *parse.ActionNode:
		vExpr, vsExpr, err := compilePipeline(scope, preStateName, node.Pipe)
		if err != nil {
			return nil, err
		}
		if len(node.Pipe.Decl) == 0 {
			return newState(vExpr, vsExpr), nil
		}
		return newState(jsonnet.EmptyString(), vsExpr), nil

	case *parse.BreakNode:
	case *parse.ContinueNode:

	case *parse.IfNode:
		return compileIfOrWith(
			tmpl, parse.NodeIf, scope, preStateName, node.Pipe, node.List, node.ElseList)

	case *parse.WithNode:
		return compileIfOrWith(
			tmpl, parse.NodeWith, scope, preStateName, node.Pipe, node.List, node.ElseList)

	case *parse.ListNode:
		states := []*state{}
		stateNames := []string{}
		varsToBeJoined := []*jsonnet.Expr{}
		stateName := preStateName
		for _, node := range node.Nodes {
			newState, err := compileNode(tmpl, scope, stateName, node)
			if err != nil {
				return nil, err
			}
			newStateName := generateStateName()
			states = append(states, newState)
			stateNames = append(stateNames, newStateName)
			varsToBeJoined = append(varsToBeJoined, jsonnet.Index(newStateName, stateV))
			stateName = newStateName
		}
		body := newState(
			jsonnet.CallJoin(varsToBeJoined),
			jsonnet.Index(stateName, stateVS),
		).body
		for i := len(states) - 1; i >= 0; i-- {
			body = &jsonnet.Expr{
				Kind: jsonnet.ELocal,
				LocalBinds: []*jsonnet.LocalBind{
					{Name: stateNames[i], Body: states[i].body},
				},
				LocalBody: body,
			}
		}
		return &state{body: body}, nil

	case *parse.RangeNode:
		return compileRange(tmpl, scope, preStateName, node)

	case *parse.TextNode:
		return newState(
			&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: string(node.Text),
			},
			jsonnet.Index(preStateName, stateVS),
		), nil

	case *parse.CommentNode:

	case *parse.TemplateNode:
		if foundTmpl := tmpl.Lookup(node.Name); foundTmpl == nil {
			return nil, fmt.Errorf("template not found: %s", node.Name)
		}
		vExpr, vsExpr, err := compilePipeline(scope, preStateName, node.Pipe)
		if err != nil {
			return nil, err
		}
		return newState(
			&jsonnet.Expr{
				Kind:     jsonnet.ECall,
				CallFunc: jsonnet.Index("$", node.Name),
				CallArgs: []*jsonnet.Expr{vExpr},
			},
			vsExpr,
		), nil
	}
	return nil, fmt.Errorf("unknown node: %v", reflect.ValueOf(node).Type())
}

func compilePipelineWithoutDecls(scope *scope, preStateName stateName, pipe *parse.PipeNode) (*jsonnet.Expr, error) {
	if pipe == nil {
		return nil, errors.New("pipe is nil")
	}

	var expr *jsonnet.Expr
	for _, cmd := range pipe.Cmds {
		var err error
		expr, err = compileCommand(scope, preStateName, cmd, expr)
		if err != nil {
			return nil, err
		}
	}

	return expr, nil
}

func compilePipeline(scope *scope, preStateName stateName, pipe *parse.PipeNode) (*jsonnet.Expr, *jsonnet.Expr, error) {
	expr, err := compilePipelineWithoutDecls(scope, preStateName, pipe)
	if err != nil {
		return nil, nil, err
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

	vsExpr := jsonnet.AddMap(jsonnet.Index(preStateName, stateVS), assignments)

	return expr, vsExpr, nil
}

func compileCommand(scope *scope, preStateName stateName, cmd *parse.CommandNode, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	switch node := cmd.Args[0].(type) {
	case *parse.FieldNode:
		return compileField(scope, preStateName, compileDot(), node.Ident, cmd.Args, final)

	case *parse.ChainNode:

	case *parse.IdentifierNode:
		return compileFunction(scope, preStateName, node, cmd.Args, final)

	case *parse.PipeNode:
		if len(node.Decl) != 0 {
			return nil, fmt.Errorf("unimplemented: parenthesized pipeline with declarations")
		}
		vExpr, _, err := compilePipeline(scope, preStateName, node)
		if err != nil {
			return nil, err
		}
		return vExpr, nil

	case *parse.VariableNode:
		return compileVariable(scope, preStateName, node, cmd.Args, final)

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

func compileArg(scope *scopeT, preStateName stateName, arg parse.Node) (*jsonnet.Expr, error) {
	switch node := arg.(type) {
	case *parse.DotNode:
		return compileDot(), nil

	case *parse.NilNode:
		return compileNil(), nil

	case *parse.FieldNode:
		return compileField(scope, preStateName, compileDot(), node.Ident, []parse.Node{arg}, nil)

	case *parse.VariableNode:
		return compileVariable(scope, preStateName, node, nil, nil)

	case *parse.PipeNode:
		vExpr, _, err := compilePipeline(scope, preStateName, node)
		if err != nil {
			return nil, err
		}
		// FIXME: handle vsExpr
		return vExpr, nil

	case *parse.IdentifierNode:
	case *parse.ChainNode:

	case *parse.BoolNode:
		return compileBool(node)

	case *parse.NumberNode:
		return compileNumber(node)

	case *parse.StringNode:
		return compileString(node)
	}

	return nil, fmt.Errorf("compile Arg: not implemented: %v", reflect.TypeOf(arg))
}

func compileField(
	scope *scopeT,
	preStateName stateName,
	receiver *jsonnet.Expr,
	ident []string,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, error) {
	compiledArgs, err := compileArgs(scope, preStateName, args, final)
	if err != nil {
		return nil, err
	}

	for i, id := range ident {
		compiledArgs1 := []*jsonnet.Expr{}
		if i == len(ident)-1 {
			compiledArgs1 = compiledArgs
		}
		receiver = jsonnet.CallField(
			receiver,
			&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: id,
			},
			&jsonnet.Expr{
				Kind: jsonnet.EList,
				List: compiledArgs1,
			},
		)
	}

	return receiver, nil
}

func compileVariable(scope *scopeT, preStateName stateName, node *parse.VariableNode, args []parse.Node, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	_, ok := scope.getVariable(node.Ident[0])
	if !ok {
		return nil, fmt.Errorf("variable not found: %s", node.Ident[0])
	}
	receiver := jsonnet.Index(preStateName, stateVS, node.Ident[0])
	if len(node.Ident) == 1 {
		return receiver, nil
	}
	return compileField(scope, preStateName, receiver, node.Ident[1:], args, final)
}

func compileFunction(scope *scope, preStateName stateName, node *parse.IdentifierNode, args []parse.Node, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	_, ok := scope.getVariable(node.Ident)
	if !ok {
		return nil, fmt.Errorf("function not found: %s", node.Ident)
	}

	function := jsonnet.Index(preStateName, stateVS, node.Ident)

	compiledArgs, err := compileArgs(scope, preStateName, args, final)
	if err != nil {
		return nil, err
	}

	return &jsonnet.Expr{
		Kind:     jsonnet.ECall,
		CallFunc: function,
		CallArgs: []*jsonnet.Expr{
			{
				Kind: jsonnet.EList,
				List: compiledArgs,
			},
		},
	}, nil
}

func compileArgs(scope *scope, preStateName stateName, args []parse.Node, final *jsonnet.Expr) ([]*jsonnet.Expr, error) {
	compiledArgs := []*jsonnet.Expr{}
	for i, arg := range args {
		if i == 0 {
			continue
		}
		compiledArg, err := compileArg(scope, preStateName, arg)
		if err != nil {
			return nil, err
		}
		compiledArgs = append(compiledArgs, compiledArg)
	}
	if final != nil {
		compiledArgs = append(compiledArgs, final)
	}

	return compiledArgs, nil
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
		return &jsonnet.Expr{Kind: jsonnet.EFloatLiteral, FloatLiteral: node.Float64}, nil
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
		Kind:   jsonnet.EID,
		IDName: "dot",
	}
}

func compileNil() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ENull,
	}
}

func compileRange(tmpl *template.Template, scope *scope, preStateName stateName, node *parse.RangeNode) (*state, error) {
	vExpr, err := compilePipelineWithoutDecls(scope, preStateName, node.Pipe)
	if err != nil {
		return nil, err
	}
	nestedPreStateName := generateStateName()

	nestedPostStateThen, err := withScope(
		scope,
		nestedPreStateName,
		func(scope *scopeT) (*state, error) {
			for _, variable := range node.Pipe.Decl {
				scope.defineVariable(variable.Ident[0])
			}
			return compileNode(tmpl, scope, nestedPreStateName, node.List)
		},
	)
	if err != nil {
		return nil, err
	}

	var nestedPostStateElse *state
	if node.ElseList == nil {
		nestedPostStateElse = &state{
			body: jsonnet.Index(nestedPreStateName),
		}
	} else {
		nestedPostStateElse, err = withScope(
			scope,
			nestedPreStateName,
			func(scope *scopeT) (*state, error) {
				return compileNode(tmpl, scope, nestedPreStateName, node.ElseList)
			},
		)
		if err != nil {
			return nil, err
		}
	}

	assignments := map[*jsonnet.Expr]*jsonnet.Expr{}
	switch len(node.Pipe.Decl) {
	case 0:
	// do nothing
	case 1:
		assignments[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: node.Pipe.Decl[0].Ident[0],
		}] = &jsonnet.Expr{
			Kind:   jsonnet.EID,
			IDName: "dot",
		}
	case 2:
		assignments[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: node.Pipe.Decl[0].Ident[0],
		}] = &jsonnet.Expr{
			Kind:   jsonnet.EID,
			IDName: "i",
		}
		assignments[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: node.Pipe.Decl[1].Ident[0],
		}] = &jsonnet.Expr{
			Kind:   jsonnet.EID,
			IDName: "dot",
		}
	default:
		return nil, fmt.Errorf("compileNode: not implemented: len(node.Pipe.Decl) > 2")
	}
	nestedPreStateName0 := generateStateName()
	nestedPreStateValue := jsonnet.Index(nestedPreStateName0)
	if len(assignments) > 0 {
		nestedPreStateValue = &jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map: map[*jsonnet.Expr]*jsonnet.Expr{
				stringLiteralStateV: jsonnet.Index(nestedPreStateName0, stateV),
				stringLiteralStateVS: jsonnet.AddMap(
					jsonnet.Index(nestedPreStateName0, stateVS),
					assignments,
				),
			},
		}
	}

	return &state{
		body: jsonnet.CallRange(
			&jsonnet.Expr{
				Kind:   jsonnet.EID,
				IDName: preStateName,
			},
			vExpr,
			&jsonnet.Expr{
				Kind:           jsonnet.EFunction,
				FunctionParams: []string{nestedPreStateName0, "i", "dot"},
				FunctionBody: &jsonnet.Expr{
					Kind: jsonnet.ELocal,
					LocalBinds: []*jsonnet.LocalBind{
						{Name: nestedPreStateName, Body: nestedPreStateValue},
					},
					LocalBody: nestedPostStateThen.body,
				},
			},
			&jsonnet.Expr{
				Kind:           jsonnet.EFunction,
				FunctionParams: []string{nestedPreStateName},
				FunctionBody:   nestedPostStateElse.body,
			},
		),
	}, nil
}

func compileIfOrWith(tmpl *template.Template, typ parse.NodeType, scope *scope, preStateName stateName, pipe *parse.PipeNode, list *parse.ListNode, elseList *parse.ListNode) (*state, error) {
	return withScope(
		scope,
		preStateName,
		func(scope *scopeT) (*state, error) {
			vExpr, vsExpr, err := compilePipeline(scope, preStateName, pipe)
			if err != nil {
				return nil, err
			}
			nestedPreState := newState(vExpr, vsExpr)
			nestedPreStateName := generateStateName()

			nestedPostStateThen, err := withScope(
				scope,
				nestedPreStateName,
				func(scope *scopeT) (*state, error) {
					state, err := compileNode(tmpl, scope, nestedPreStateName, list)
					if err != nil {
						return nil, err
					}
					if typ == parse.NodeWith {
						state.body = &jsonnet.Expr{
							Kind: jsonnet.ELocal,
							LocalBinds: []*jsonnet.LocalBind{
								{Name: "dot", Body: jsonnet.Index(nestedPreStateName, stateV)},
							},
							LocalBody: state.body,
						}
					}
					return state, nil
				},
			)
			if err != nil {
				return nil, err
			}

			var nestedPostStateElse *state
			if elseList == nil {
				nestedPostStateElse = &state{
					body: &jsonnet.Expr{
						Kind:   jsonnet.EID,
						IDName: nestedPreStateName,
					},
				}
			} else {
				nestedPostStateElse, err = withScope(
					scope,
					nestedPreStateName,
					func(scope *scopeT) (*state, error) {
						return compileNode(tmpl, scope, nestedPreStateName, elseList)
					},
				)
				if err != nil {
					return nil, err
				}
			}

			return &state{
				body: &jsonnet.Expr{
					Kind: jsonnet.ELocal,
					LocalBinds: []*jsonnet.LocalBind{
						{Name: nestedPreStateName, Body: nestedPreState.body},
					},
					LocalBody: &jsonnet.Expr{
						Kind:   jsonnet.EIf,
						IfCond: jsonnet.CallIsTrue(jsonnet.Index(nestedPreStateName, stateV)),
						IfThen: nestedPostStateThen.body,
						IfElse: nestedPostStateElse.body,
					},
				},
			}, nil
		},
	)
}
