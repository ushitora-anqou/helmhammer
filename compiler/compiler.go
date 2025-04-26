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

func Compile(tmpl0 *template.Template) (*jsonnet.Expr, error) {
	globalVariables := map[string]*jsonnet.Expr{
		"printf":     jsonnet.Index("helmhammer", "printf"),
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

	globalEnv := newEnv(
		tmpl0,
		&scope{
			parent:    nil,
			variables: map[string]*variable{},
		},
		generateStateName(),
	)
	for key := range globalVariables {
		if err := globalEnv.defineVariable(key); err != nil {
			return nil, err
		}
	}

	compiledTemplates := map[*jsonnet.Expr]*jsonnet.Expr{}
	for _, tmpl := range tmpl0.Templates() {
		compiledTemplate, err := compile(globalEnv, tmpl.Root)
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

	initialState := newState(
		jsonnet.EmptyString(),
		&jsonnet.Expr{
			Kind: jsonnet.EMap,
			Map:  compiledGlobalVariables,
		},
	)

	return initialState.toLocal(globalEnv.preStateName, &jsonnet.Expr{
		Kind: jsonnet.EMap,
		Map:  compiledTemplates,
	}), nil
}

func compile(env *envT, node parse.Node) (*jsonnet.Expr, error) {
	preStateName := generateStateName()
	postState, err := withScope(
		env,
		preStateName,
		func(env *envT) (*state, error) {
			if err := env.defineVariable("$"); err != nil {
				return nil, err
			}
			return compileNode(env, node)
		},
	)
	if err != nil {
		return nil, err
	}

	preState := newState(
		jsonnet.EmptyString(),
		jsonnet.AddMap(
			jsonnet.Index(env.preStateName, stateVS),
			map[*jsonnet.Expr]*jsonnet.Expr{
				{Kind: jsonnet.EStringLiteral, StringLiteral: "$"}: compileDot(),
			}),
	)

	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"dot"},
		FunctionBody: preState.toLocal(preStateName, &jsonnet.Expr{
			Kind:          jsonnet.EIndexList,
			IndexListHead: postState.body,
			IndexListTail: []string{stateV},
		}),
	}, nil
}

func compileNode(env *envT, node parse.Node) (*state, error) {
	switch node := node.(type) {
	case *parse.ActionNode:
		vExpr, vsExpr, err := compilePipeline(env, node.Pipe)
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
		return compileIfOrWith(env, parse.NodeIf, node.Pipe, node.List, node.ElseList)

	case *parse.WithNode:
		return compileIfOrWith(env, parse.NodeWith, node.Pipe, node.List, node.ElseList)

	case *parse.ListNode:
		states := []*state{}
		stateNames := []string{}
		varsToBeJoined := []*jsonnet.Expr{}
		stateName := env.preStateName
		for _, node := range node.Nodes {
			newState, err := compileNode(env.withPreState(stateName), node)
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
			body = states[i].toLocal(stateNames[i], body)
		}
		return &state{body: body}, nil

	case *parse.RangeNode:
		return compileRange(env, node)

	case *parse.TextNode:
		return newState(
			&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: string(node.Text),
			},
			jsonnet.Index(env.preStateName, stateVS),
		), nil

	case *parse.CommentNode:

	case *parse.TemplateNode:
		if foundTmpl := env.tmpl.Lookup(node.Name); foundTmpl == nil {
			return nil, fmt.Errorf("template not found: %s", node.Name)
		}
		vExpr, vsExpr, err := compilePipeline(env, node.Pipe)
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

func compilePipelineWithoutDecls(env *envT, pipe *parse.PipeNode) (*jsonnet.Expr, error) {
	if pipe == nil {
		return nil, errors.New("pipe is nil")
	}

	var expr *jsonnet.Expr
	for _, cmd := range pipe.Cmds {
		var err error
		expr, err = compileCommand(env, cmd, expr)
		if err != nil {
			return nil, err
		}
	}

	return expr, nil
}

func compilePipeline(env *envT, pipe *parse.PipeNode) (*jsonnet.Expr, *jsonnet.Expr, error) {
	expr, err := compilePipelineWithoutDecls(env, pipe)
	if err != nil {
		return nil, nil, err
	}

	assignments := map[*jsonnet.Expr]*jsonnet.Expr{}
	for _, variable := range pipe.Decl {
		if pipe.IsAssign {
			env.assignVariable(variable.Ident[0])
		} else {
			env.defineVariable(variable.Ident[0])
		}
		assignments[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: variable.Ident[0],
		}] = expr
	}

	vsExpr := jsonnet.AddMap(jsonnet.Index(env.preStateName, stateVS), assignments)

	return expr, vsExpr, nil
}

func compileCommand(env *envT, cmd *parse.CommandNode, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	switch node := cmd.Args[0].(type) {
	case *parse.FieldNode:
		return compileField(env, compileDot(), node.Ident, cmd.Args, final)

	case *parse.ChainNode:

	case *parse.IdentifierNode:
		return compileFunction(env, node, cmd.Args, final)

	case *parse.PipeNode:
		if len(node.Decl) != 0 {
			return nil, fmt.Errorf("unimplemented: parenthesized pipeline with declarations")
		}
		vExpr, _, err := compilePipeline(env, node)
		if err != nil {
			return nil, err
		}
		return vExpr, nil

	case *parse.VariableNode:
		return compileVariable(env, node, cmd.Args, final)

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

func compileArg(env *envT, arg parse.Node) (*jsonnet.Expr, error) {
	switch node := arg.(type) {
	case *parse.DotNode:
		return compileDot(), nil

	case *parse.NilNode:
		return compileNil(), nil

	case *parse.FieldNode:
		return compileField(env, compileDot(), node.Ident, []parse.Node{arg}, nil)

	case *parse.VariableNode:
		return compileVariable(env, node, nil, nil)

	case *parse.PipeNode:
		vExpr, _, err := compilePipeline(env, node)
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
	env *envT,
	receiver *jsonnet.Expr,
	ident []string,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, error) {
	compiledArgs, err := compileArgs(env, args, final)
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

func compileVariable(env *envT, node *parse.VariableNode, args []parse.Node, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	if node.Ident[0] == "include" {
		if len(node.Ident) != 1 {
			return nil, errors.New("include is not a map")
		}
		return compileInclude(), nil
	}

	_, ok := env.getVariable(node.Ident[0])
	if !ok {
		return nil, fmt.Errorf("variable not found: %s", node.Ident[0])
	}
	receiver := jsonnet.Index(env.preStateName, stateVS, node.Ident[0])
	if len(node.Ident) == 1 {
		return receiver, nil
	}
	return compileField(env, receiver, node.Ident[1:], args, final)
}

func compileFunction(env *envT, node *parse.IdentifierNode, args []parse.Node, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	var function *jsonnet.Expr
	if node.Ident == "include" {
		function = compileInclude()
	} else {
		_, ok := env.getVariable(node.Ident)
		if !ok {
			return nil, fmt.Errorf("function not found: %s", node.Ident)
		}
		function = jsonnet.Index(env.preStateName, stateVS, node.Ident)
	}

	compiledArgs, err := compileArgs(env, args, final)
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

func compileArgs(env *envT, args []parse.Node, final *jsonnet.Expr) ([]*jsonnet.Expr, error) {
	compiledArgs := []*jsonnet.Expr{}
	for i, arg := range args {
		if i == 0 {
			continue
		}
		compiledArg, err := compileArg(env, arg)
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

func compileRange(env *envT, node *parse.RangeNode) (*state, error) {
	vExpr, err := compilePipelineWithoutDecls(env, node.Pipe)
	if err != nil {
		return nil, err
	}
	nestedPreStateName := generateStateName()

	nestedPostStateThen, err := withScope(
		env,
		nestedPreStateName,
		func(env *envT) (*state, error) {
			for _, variable := range node.Pipe.Decl {
				env.defineVariable(variable.Ident[0])
			}
			return compileNode(env, node.List)
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
			env,
			nestedPreStateName,
			func(env *envT) (*state, error) {
				return compileNode(env, node.ElseList)
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
				IDName: env.preStateName,
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

func compileIfOrWith(env *envT, typ parse.NodeType, pipe *parse.PipeNode, list *parse.ListNode, elseList *parse.ListNode) (*state, error) {
	return withScope(
		env,
		env.preStateName,
		func(env *envT) (*state, error) {
			vExpr, vsExpr, err := compilePipeline(env, pipe)
			if err != nil {
				return nil, err
			}
			nestedPreState := newState(vExpr, vsExpr)
			nestedPreStateName := generateStateName()

			nestedPostStateThen, err := withScope(
				env,
				nestedPreStateName,
				func(env *envT) (*state, error) {
					state, err := compileNode(env, list)
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
				nestedPostStateElse = newState(
					jsonnet.EmptyString(),
					jsonnet.Index(nestedPreStateName, stateVS),
				)
			} else {
				nestedPostStateElse, err = withScope(
					env,
					nestedPreStateName,
					func(env *envT) (*state, error) {
						return compileNode(env, elseList)
					},
				)
				if err != nil {
					return nil, err
				}
			}

			return &state{
				body: nestedPreState.toLocal(nestedPreStateName, &jsonnet.Expr{
					Kind:   jsonnet.EIf,
					IfCond: jsonnet.CallIsTrue(jsonnet.Index(nestedPreStateName, stateV)),
					IfThen: nestedPostStateThen.body,
					IfElse: nestedPostStateElse.body,
				}),
			}, nil
		},
	)
}

func compileInclude() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ERaw,
		Raw:  `helmhammer.include($)`,
	}
}
