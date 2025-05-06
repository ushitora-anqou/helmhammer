package compiler

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/ushitora-anqou/helmhammer/helm"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

func CompileChart(chart *helm.Chart) (*jsonnet.Expr, error) {
	expr, err := Compile(chart.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to compile template: %w", err)
	}

	crds := [][]byte{}
	for _, crd := range chart.CRDObjects {
		crds = append(crds, crd.File.Data)
	}

	expr = jsonnet.CallChartMain(
		chart.Name, chart.Version, chart.AppVersion,
		chart.Name, "Helm",
		chart.TemplateBasePath,
		jsonnet.ConvertIntoJsonnet(chart.Capabilities),
		chart.RenderedKeys, jsonnet.ConvertIntoJsonnet(chart.Values),
		crds, expr,
	)

	return expr, nil
}

func Compile(tmpl0 *template.Template) (*jsonnet.Expr, error) {
	globalVariables := map[string]*jsonnet.Expr{}
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
		jsonnet.EmptyMap(),
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
				{Kind: jsonnet.EStringLiteral, StringLiteral: "$"}: jsonnet.Index("dot"),
			}),
		jsonnet.Index("heap"),
	)

	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"heap", "dot"},
		FunctionBody:   preState.toLocal(preStateName, postState.body),
	}, nil
}

func compileNode(env *envT, node parse.Node) (*state, error) {
	switch node := node.(type) {
	case *parse.ActionNode:
		pipeState, err := compilePipeline(env, node.Pipe)
		if err != nil {
			return nil, err
		}
		if len(node.Pipe.Decl) == 0 {
			return pipeState, nil
		}
		pipeStateName := generateStateName()
		return &state{
			body: pipeState.toLocal(
				pipeStateName,
				newStateSameContext(pipeStateName, jsonnet.EmptyString()).body,
			),
		}, nil

	case *parse.BreakNode:
	case *parse.ContinueNode:

	case *parse.IfNode:
		return compileIfOrWith(env, parse.NodeIf, node.Pipe, node.List, node.ElseList)

	case *parse.WithNode:
		return compileIfOrWith(env, parse.NodeWith, node.Pipe, node.List, node.ElseList)

	case *parse.ListNode:
		if len(node.Nodes) == 0 {
			return newStateSameContext(env.preStateName, jsonnet.EmptyString()), nil
		}
		return sequentialStates(
			env,
			node.Nodes,
			func(env *envT, _ int, node parse.Node) (*state, error) {
				return compileNode(env, node)
			},
			func(stateNames []stateName) (*state, error) {
				varsToBeJoined := make([]*jsonnet.Expr, 0, len(stateNames))
				for _, stateName := range stateNames {
					varsToBeJoined = append(
						varsToBeJoined,
						jsonnet.Index(stateName, stateV),
					)
				}
				return newStateSameContext(
					stateNames[len(stateNames)-1],
					jsonnet.CallJoin(
						jsonnet.Index(stateNames[len(stateNames)-1], stateH),
						varsToBeJoined,
					),
				), nil
			},
		)

	case *parse.RangeNode:
		return compileRange(env, node)

	case *parse.TextNode:
		return newStateSameContext(
			env.preStateName,
			&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: string(node.Text),
			},
		), nil

	case *parse.CommentNode:

	case *parse.TemplateNode:
		if foundTmpl := env.tmpl.Lookup(node.Name); foundTmpl == nil {
			return nil, fmt.Errorf("template not found: %s", node.Name)
		}
		pipeState, err := compilePipeline(env, node.Pipe)
		if err != nil {
			return nil, fmt.Errorf("template node: %s: %w", node.Name, err)
		}
		pipeStateName := generateStateName()
		return &state{
			body: pipeState.toLocal(
				pipeStateName,
				jsonnet.AddMap(
					&jsonnet.Expr{
						Kind:     jsonnet.ECall,
						CallFunc: jsonnet.Index("$", node.Name),
						CallArgs: []*jsonnet.Expr{
							jsonnet.Index(pipeStateName, stateH),
							jsonnet.Index(pipeStateName, stateV),
						},
					},
					map[*jsonnet.Expr]*jsonnet.Expr{
						stringLiteralStateVS: jsonnet.Index(pipeStateName, stateVS),
					},
				),
			),
		}, nil
	}
	return nil, fmt.Errorf("unknown node: %v", reflect.ValueOf(node).Type())
}

func compilePipelineWithoutDecls(env *envT, pipe *parse.PipeNode) (*state, error) {
	if pipe == nil {
		return newStateSameContext(
			env.preStateName,
			&jsonnet.Expr{Kind: jsonnet.ENull},
		), nil
	}

	if len(pipe.Cmds) == 0 {
		return nil, errors.New("pipe.Cmds is empty")
	}

	return sequentialStates(
		env,
		pipe.Cmds,
		func(env *envT, i int, cmd *parse.CommandNode) (*state, error) {
			final := jsonnet.Index(env.preStateName, stateV)
			if i == 0 {
				final = nil
			}
			return compileCommand(env, cmd, final)
		},
		nil,
	)
}

func compilePipeline(env *envT, pipe *parse.PipeNode) (*state, error) {
	nestedState, err := compilePipelineWithoutDecls(env, pipe)
	if err != nil {
		return nil, fmt.Errorf("compilePipeline: %w", err)
	}
	if pipe == nil {
		return nestedState, nil
	}

	nestedStateName := generateStateName()
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
		}] = jsonnet.Index(nestedStateName, stateV)
	}

	return &state{
		body: nestedState.toLocal(
			nestedStateName,
			newState(
				jsonnet.Index(nestedStateName, stateV),
				jsonnet.AddMap(jsonnet.Index(nestedStateName, stateVS), assignments),
				jsonnet.Index(nestedStateName, stateH),
			).body,
		),
	}, nil
}

func compileCommand(
	env *envT,
	cmd *parse.CommandNode,
	final *jsonnet.Expr,
) (*state, error) {
	var vExpr *jsonnet.Expr
	var err error
	switch node := cmd.Args[0].(type) {
	case *parse.BoolNode:
		vExpr, err = compileBool(node)

	case *parse.DotNode:
		vExpr = compileDot()

	case *parse.NilNode:
		return nil, errors.New("nil is not a command")

	case *parse.NumberNode:
		vExpr, err = compileNumber(node)

	case *parse.StringNode:
		vExpr, err = compileString(node)
	}

	if err != nil {
		return nil, err
	}

	if vExpr != nil {
		return newStateSameContext(env.preStateName, vExpr), nil
	}

	switch node := cmd.Args[0].(type) {
	case *parse.FieldNode:
		return compileField(env, compileDot(), node.Ident, cmd.Args, final)

	case *parse.ChainNode:
		return compileChain(env, node, cmd.Args, final)

	case *parse.IdentifierNode:
		return compileFunction(env, node, cmd.Args, final)

	case *parse.PipeNode:
		return compilePipeline(env, node)

	case *parse.VariableNode:
		return compileVariable(env, node, cmd.Args, final)
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

func compileArg(env *envT, arg parse.Node) (*state, error) {
	var vExpr *jsonnet.Expr
	var err error
	switch node := arg.(type) {
	case *parse.DotNode:
		vExpr = compileDot()

	case *parse.NilNode:
		vExpr = compileNil()

	case *parse.BoolNode:
		vExpr, err = compileBool(node)

	case *parse.NumberNode:
		vExpr, err = compileNumber(node)

	case *parse.StringNode:
		vExpr, err = compileString(node)
	}

	if err != nil {
		return nil, err
	}

	if vExpr != nil {
		return newStateSameContext(env.preStateName, vExpr), nil
	}

	switch node := arg.(type) {
	case *parse.FieldNode:
		return compileField(env, compileDot(), node.Ident, []parse.Node{arg}, nil)

	case *parse.VariableNode:
		return compileVariable(env, node, nil, nil)

	case *parse.PipeNode:
		return compilePipeline(env, node)

	case *parse.IdentifierNode:
		return compileFunction(env, node, nil, nil)

	case *parse.ChainNode:
		return compileChain(env, node, nil, nil)
	}

	return nil, fmt.Errorf("compile Arg: not implemented: %v", reflect.TypeOf(arg))
}

func compileChain(
	env *envT,
	chain *parse.ChainNode,
	args []parse.Node,
	final *jsonnet.Expr,
) (*state, error) {
	if len(chain.Field) == 0 {
		return nil, errors.New("compileChain: no fields")
	}
	if chain.Node.Type() == parse.NodeNil {
		return nil, errors.New("nil indirection")
	}

	pipeState, err := compileArg(env, chain.Node)
	if err != nil {
		return nil, err
	}

	pipeStateName := generateStateName()
	postState, err := compileField(
		env.withPreState(pipeStateName),
		jsonnet.Index(pipeStateName, stateV),
		chain.Field,
		args,
		final,
	)
	if err != nil {
		return nil, err
	}

	return &state{
		body: pipeState.toLocal(
			pipeStateName,
			postState.body,
		),
	}, nil
}

func compileField(
	env *envT,
	receiver *jsonnet.Expr,
	ident []string,
	args []parse.Node,
	final *jsonnet.Expr,
) (*state, error) {
	argsState, err := compileArgs(env, args, final)
	if err != nil {
		return nil, err
	}
	argsStateName := generateStateName()

	for i, id := range ident {
		compiledArgs := &jsonnet.Expr{
			Kind: jsonnet.EList,
			List: []*jsonnet.Expr{},
		}
		if i == len(ident)-1 {
			compiledArgs = jsonnet.Index(argsStateName, stateV)
		}
		receiver = jsonnet.CallField(
			jsonnet.Index(argsStateName, stateH),
			receiver,
			&jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: id,
			},
			compiledArgs,
		)
	}

	return &state{
		body: argsState.toLocal(
			argsStateName,
			newStateSameContext(argsStateName, receiver).body,
		),
	}, nil
}

func compileVariable(env *envT, node *parse.VariableNode, args []parse.Node, final *jsonnet.Expr) (*state, error) {
	_, ok := env.getVariable(node.Ident[0])
	if !ok {
		return nil, fmt.Errorf("variable not found: %s", node.Ident[0])
	}
	receiver := jsonnet.Index(env.preStateName, stateVS, node.Ident[0])
	if len(node.Ident) == 1 {
		return newStateSameContext(env.preStateName, receiver), nil
	}
	return compileField(env, receiver, node.Ident[1:], args, final)
}

func compileFunction(env *envT, node *parse.IdentifierNode, args []parse.Node, final *jsonnet.Expr) (*state, error) {
	argsState, err := compileArgs(env, args, final)
	if err != nil {
		return nil, err
	}
	argsStateName := generateStateName()

	if builtin := compileBuiltinFunctions(
		env.withPreState(argsStateName),
		node.Ident,
		jsonnet.Index(argsStateName, stateV),
	); builtin != nil {
		return &state{body: argsState.toLocal(argsStateName, builtin.body)}, nil
	}

	if _, ok := env.getVariable(node.Ident); !ok {
		return nil, fmt.Errorf("function not found: %s", node.Ident)
	}
	function := jsonnet.Index(argsStateName, stateVS, node.Ident)

	return &state{
		body: argsState.toLocal(
			argsStateName,
			newStateSameContext(
				argsStateName,
				&jsonnet.Expr{
					Kind:     jsonnet.ECall,
					CallFunc: function,
					CallArgs: []*jsonnet.Expr{
						jsonnet.Index(argsStateName, stateV),
					},
				},
			).body,
		),
	}, nil
}

func compileArgs(
	env *envT,
	args []parse.Node,
	final *jsonnet.Expr,
) (*state, error) {
	if len(args) <= 1 { // no arguments except `final`
		vExpr := &jsonnet.Expr{
			Kind: jsonnet.EList,
			List: []*jsonnet.Expr{},
		}
		if final != nil {
			vExpr.List = append(vExpr.List, final)
		}
		return newStateSameContext(env.preStateName, vExpr), nil
	}

	return sequentialStates(
		env,
		args[1:],
		func(env *envT, _ int, arg parse.Node) (*state, error) {
			return compileArg(env, arg)
		},
		func(stateNames []stateName) (*state, error) {
			vExpr := &jsonnet.Expr{
				Kind: jsonnet.EList,
				List: []*jsonnet.Expr{},
			}
			for _, stateName := range stateNames {
				vExpr.List = append(vExpr.List, jsonnet.Index(stateName, stateV))
			}
			if final != nil {
				vExpr.List = append(vExpr.List, final)
			}
			return newStateSameContext(stateNames[len(stateNames)-1], vExpr), nil
		},
	)
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
	pipeState, err := compilePipelineWithoutDecls(env, node.Pipe)
	if err != nil {
		return nil, fmt.Errorf("compileRange: %w", err)
	}
	pipeStateName := generateStateName()
	env = env.withPreState(pipeStateName)

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
				stringLiteralStateH: jsonnet.Index(nestedPreStateName0, stateH),
				stringLiteralStateVS: jsonnet.AddMap(
					jsonnet.Index(nestedPreStateName0, stateVS),
					assignments,
				),
			},
		}
	}

	return &state{
		body: pipeState.toLocal(
			pipeStateName,
			jsonnet.CallRange(
				&jsonnet.Expr{
					Kind:   jsonnet.EID,
					IDName: pipeStateName,
				},
				jsonnet.Index(pipeStateName, stateV),
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
		),
	}, nil
}

func compileIfOrWith(env *envT, typ parse.NodeType, pipe *parse.PipeNode, list *parse.ListNode, elseList *parse.ListNode) (*state, error) {
	return withScope(
		env,
		env.preStateName,
		func(env *envT) (*state, error) {
			nestedPreState, err := compilePipeline(env, pipe)
			if err != nil {
				return nil, err
			}
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
				nestedPostStateElse = newStateSameContext(
					nestedPreStateName,
					jsonnet.EmptyString(),
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
					Kind: jsonnet.EIf,
					IfCond: jsonnet.CallIsTrueOnHeap(
						jsonnet.Index(nestedPreStateName, stateH),
						jsonnet.Index(nestedPreStateName, stateV),
					),
					IfThen: nestedPostStateThen.body,
					IfElse: nestedPostStateElse.body,
				}),
			}, nil
		},
	)
}

func compileBuiltinFunctions(
	env *envT,
	ident string,
	compiledArgs *jsonnet.Expr,
) *state {
	if _, ok := jsonnet.PredefinedFunctions()[ident]; ok {
		return &state{
			body: &jsonnet.Expr{
				Kind: jsonnet.ECall,
				CallFunc: &jsonnet.Expr{
					Kind: jsonnet.ERaw,
					Raw:  `helmhammer.callBuiltin`,
				},
				CallArgs: []*jsonnet.Expr{
					jsonnet.Index(env.preStateName),
					{Kind: jsonnet.EStringLiteral, StringLiteral: ident},
					compiledArgs,
				},
			},
		}
	}

	switch ident {
	case "include", "tpl", "set", "mergeOverwrite":
	default:
		return nil
	}

	return &state{
		body: &jsonnet.Expr{
			Kind: jsonnet.ECall,
			CallFunc: &jsonnet.Expr{
				Kind: jsonnet.ERaw,
				Raw:  fmt.Sprintf(`helmhammer.%s`, ident),
			},
			CallArgs: []*jsonnet.Expr{
				jsonnet.Map(map[string]*jsonnet.Expr{
					"$": {
						Kind:   jsonnet.EID,
						IDName: "$",
					},
					"args": compiledArgs,
					"vs":   jsonnet.Index(env.preStateName, stateVS),
					"heap": jsonnet.Index(env.preStateName, stateH),
				}),
			},
		},
	}
}
