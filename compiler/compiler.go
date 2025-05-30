package compiler

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/ushitora-anqou/helmhammer/compiler/env"
	"github.com/ushitora-anqou/helmhammer/compiler/state"
	"github.com/ushitora-anqou/helmhammer/helm"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

func sequential[Item any](
	env *env.T,
	items []Item,
	fIter func(*env.T, int, Item, *jsonnet.Expr) (*jsonnet.Expr, *state.T, error),
	init *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	curState := env.State()
	acc := init
	for i, item := range items {
		var err error
		acc, curState, err = curState.Use(
			func(vs *jsonnet.Expr, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
				return fIter(env.WithVSAndH(vs, h), i, item, acc)
			},
		)
		if err != nil {
			return nil, nil, err
		}
	}

	return acc, curState, nil
}

func compileChartMetadata(
	initialHeap []*jsonnet.Expr,
	chart *helm.Chart,
) (*jsonnet.Expr, []*jsonnet.Expr, error) {
	crds := [][]byte{}
	for _, crd := range chart.CRDObjects {
		crds = append(crds, crd.File.Data)
	}

	var defaultValues *jsonnet.Expr
	var err error
	defaultValues, initialHeap, err = jsonnet.DeepAllocate(initialHeap, chart.Values)
	if err != nil {
		return nil, nil, err
	}

	compiledFiles := map[string]*jsonnet.Expr{}
	for name, data := range chart.Files {
		compiledFiles[name] = &jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: string(data),
		}
	}

	compiledSubCharts := []*jsonnet.Expr{}
	for _, subChart := range chart.SubCharts {
		var compiledSubChart *jsonnet.Expr
		compiledSubChart, initialHeap, err = compileChartMetadata(initialHeap, subChart)
		if err != nil {
			return nil, initialHeap, err
		}
		compiledSubCharts = append(compiledSubCharts, compiledSubChart)
	}

	return jsonnet.CallChartMetadata(
		chart.Name,
		chart.Version,
		chart.AppVersion,
		chart.TemplateBasePath,
		chart.Condition,
		chart.RenderedKeys,
		defaultValues,
		crds,
		compiledFiles,
		compiledSubCharts,
	), initialHeap, nil
}

func CompileChart(chart *helm.RootChart) (*jsonnet.Expr, error) {
	expr, err := Compile(chart.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to compile template: %w", err)
	}

	initialHeap := []*jsonnet.Expr{}
	var rootChart *jsonnet.Expr
	rootChart, initialHeap, err = compileChartMetadata(initialHeap, chart.Chart)
	if err != nil {
		return nil, fmt.Errorf("failed to compile chart metadata: %w", err)
	}

	convertedInitialHeap := map[string]*jsonnet.Expr{}
	for i, v := range initialHeap {
		convertedInitialHeap[strconv.Itoa(i)] = v
	}

	return jsonnet.CallChartMain(
		jsonnet.ConvertIntoJsonnet(chart.Capabilities),
		rootChart,
		jsonnet.Map(convertedInitialHeap),
		expr,
	), nil
}

func Compile(tmpl0 *template.Template) (*jsonnet.Expr, error) {
	sortedTemplates := []*template.Template{}
	for _, tmpl := range tmpl0.Templates() {
		sortedTemplates = append(sortedTemplates, tmpl)
	}
	slices.SortFunc(sortedTemplates, func(l, r *template.Template) int {
		return strings.Compare(l.Name(), r.Name())
	})

	compiledTemplates := []*jsonnet.MapEntry{}
	for _, tmpl := range sortedTemplates {
		globalEnv := env.New(tmpl0)
		compiledTemplate, err := compile(globalEnv, tmpl.Root)
		if err != nil {
			return nil, err
		}
		compiledTemplates = append(compiledTemplates, &jsonnet.MapEntry{
			K: &jsonnet.Expr{
				Kind:          jsonnet.EStringLiteral,
				StringLiteral: tmpl.Name(),
			},
			V: compiledTemplate,
		})
	}

	return &jsonnet.Expr{
		Kind: jsonnet.EMap,
		Map:  compiledTemplates,
	}, nil
}

func compile(e *env.T, node parse.Node) (*jsonnet.Expr, error) {
	enhancedVSName := state.GenerateBindName() // vs + {"$": dot}
	dotName := state.GenerateBindName()
	enhancedE := e.WithVSAndH(
		jsonnet.Index(enhancedVSName),
		jsonnet.Index("h"),
	).WithDot(jsonnet.Index(dotName))
	if err := enhancedE.DefineVariable("$"); err != nil {
		return nil, err
	}
	vExpr, newState, err := compileNode(enhancedE, node)
	if err != nil {
		return nil, err
	}

	// function(heap, dot) local vs0 = {"$": dot}; ...; [v, vs, h]
	return &jsonnet.Expr{
		Kind:           jsonnet.EFunction,
		FunctionParams: []string{"h", dotName},
		FunctionBody: &jsonnet.Expr{
			Kind: jsonnet.ELocal,
			LocalBinds: []*jsonnet.LocalBind{{
				Name: enhancedVSName,
				Body: jsonnet.Map(map[string]*jsonnet.Expr{"$": jsonnet.Index(dotName)}),
			}},
			LocalBody: newState.Finalize(vExpr),
		},
	}, nil
}

func compileNode(e *env.T, node parse.Node) (*jsonnet.Expr, *state.T, error) {
	switch node := node.(type) {
	case *parse.ActionNode:
		pipeExpr, pipeState, err := compilePipeline(e, node.Pipe)
		if err != nil {
			return nil, nil, err
		}
		if len(node.Pipe.Decl) == 0 {
			return pipeExpr, pipeState, nil
		}
		return jsonnet.EmptyString(), pipeState, nil

	case *parse.BreakNode:
		return nil, nil, errors.New("break not implemented")

	case *parse.ContinueNode:
		return nil, nil, errors.New("continue not implemented")

	case *parse.IfNode:
		return compileIfOrWith(e, parse.NodeIf, node.Pipe, node.List, node.ElseList)

	case *parse.WithNode:
		return compileIfOrWith(e, parse.NodeWith, node.Pipe, node.List, node.ElseList)

	case *parse.ListNode:
		if len(node.Nodes) == 0 {
			return jsonnet.EmptyString(), e.State(), nil
		}
		vExpr, newState, err := sequential(
			e,
			node.Nodes,
			func(
				e *env.T,
				_ int,
				node parse.Node,
				acc *jsonnet.Expr,
			) (*jsonnet.Expr, *state.T, error) {
				vExpr, newState, err := compileNode(e, node)
				if err != nil {
					return nil, nil, err
				}
				acc.List = append(acc.List, vExpr)
				return acc, newState, err
			},
			&jsonnet.Expr{Kind: jsonnet.EList, List: []*jsonnet.Expr{}},
		)
		if err != nil {
			return nil, nil, err
		}
		return newState.Use(
			func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
				return jsonnet.CallJoin(h, vExpr.List),
					state.New(nil, vs, h),
					nil
			},
		)

	case *parse.RangeNode:
		return compileRange(e, node)

	case *parse.TextNode:
		return &jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: string(node.Text),
		}, e.State(), nil

	case *parse.CommentNode:
		return nil, nil, errors.New("CommentNode not implemented")

	case *parse.TemplateNode:
		if foundTmpl := e.Template().Lookup(node.Name); foundTmpl == nil {
			return nil, nil, fmt.Errorf("template not found: %s", node.Name)
		}
		pipeExpr, pipeState, err := compilePipeline(e, node.Pipe)
		if err != nil {
			return nil, nil, fmt.Errorf("template node: %s: %w", node.Name, err)
		}
		return pipeState.Use(
			func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
				tmplResultName := state.GenerateBindName()
				newState := state.New(
					[]*jsonnet.LocalBind{
						{Name: tmplResultName, Body: &jsonnet.Expr{
							Kind:     jsonnet.ECall,
							CallFunc: jsonnet.Index("$", node.Name),
							CallArgs: []*jsonnet.Expr{h, pipeExpr},
						}},
					},
					vs,
					jsonnet.IndexInt(tmplResultName, 2), // h
				)
				vExpr := jsonnet.IndexInt(tmplResultName, 0) // v
				return vExpr, newState, nil
			},
		)
	}

	return nil, nil, fmt.Errorf("unknown node: %v", reflect.ValueOf(node).Type())
}

func compilePipelineWithoutDecls(
	e *env.T,
	pipe *parse.PipeNode,
) (*jsonnet.Expr, *state.T, error) {
	if pipe == nil {
		return &jsonnet.Expr{Kind: jsonnet.ENull}, e.State(), nil
	}

	if len(pipe.Cmds) == 0 {
		return nil, nil, errors.New("pipe.Cmds is empty")
	}

	return sequential(
		e,
		pipe.Cmds,
		func(
			e *env.T,
			i int,
			cmd *parse.CommandNode,
			final *jsonnet.Expr,
		) (*jsonnet.Expr, *state.T, error) {
			return compileCommand(e, cmd, final)
		},
		nil,
	)
}

func compilePipeline(e *env.T, pipe *parse.PipeNode) (*jsonnet.Expr, *state.T, error) {
	if pipe == nil {
		return &jsonnet.Expr{Kind: jsonnet.ENull}, e.State(), nil
	}

	pipeExpr, pipeState, err := compilePipelineWithoutDecls(e, pipe)
	if err != nil {
		return nil, nil, fmt.Errorf("compilePipeline: %w", err)
	}

	return pipeState.Use(
		func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			pipeExprName := state.GenerateBindName()

			assignments := []*jsonnet.MapEntry{}
			for _, variable := range pipe.Decl {
				if pipe.IsAssign {
					e.AssignVariable(variable.Ident[0])
				} else {
					e.DefineVariable(variable.Ident[0])
				}
				assignments = append(assignments, &jsonnet.MapEntry{
					K: &jsonnet.Expr{
						Kind:          jsonnet.EStringLiteral,
						StringLiteral: variable.Ident[0],
					},
					V: jsonnet.Index(pipeExprName),
				})
			}

			if len(assignments) == 0 {
				return pipeExpr, state.New(nil, vs, h), nil
			}

			newVSName := state.GenerateBindName()
			return jsonnet.Index(pipeExprName), state.New(
				[]*jsonnet.LocalBind{
					{Name: pipeExprName, Body: pipeExpr},
					{Name: newVSName, Body: jsonnet.AddMap(vs, assignments)},
				},
				jsonnet.Index(newVSName),
				h,
			), nil
		},
	)
}

func compileCommand(
	env *env.T,
	cmd *parse.CommandNode,
	final *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	var vExpr *jsonnet.Expr
	var err error
	switch node := cmd.Args[0].(type) {
	case *parse.BoolNode:
		vExpr, err = compileBool(node)

	case *parse.DotNode:
		vExpr = compileDot(env)

	case *parse.NilNode:
		return nil, nil, errors.New("nil is not a command")

	case *parse.NumberNode:
		vExpr, err = compileNumber(node)

	case *parse.StringNode:
		vExpr, err = compileString(node)
	}

	if err != nil {
		return nil, nil, err
	}

	if vExpr != nil {
		return vExpr, env.State(), nil
	}

	switch node := cmd.Args[0].(type) {
	case *parse.FieldNode:
		return compileField(env, compileDot(env), node.Ident, cmd.Args, final)

	case *parse.ChainNode:
		return compileChain(env, node, cmd.Args, final)

	case *parse.IdentifierNode:
		return compileFunction(env, node, cmd.Args, final)

	case *parse.PipeNode:
		return compilePipeline(env, node)

	case *parse.VariableNode:
		return compileVariable(env, node, cmd.Args, final)
	}

	return nil, nil, fmt.Errorf("unknown command: %v", reflect.ValueOf(cmd.Args[0]).Type())
}

func isRuneInt(s string) bool {
	return len(s) > 0 && s[0] == '\''
}

func isHexInt(s string) bool {
	return len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') &&
		!strings.ContainsAny(s, "pP")
}

func compileArg(env *env.T, arg parse.Node) (*jsonnet.Expr, *state.T, error) {
	var vExpr *jsonnet.Expr
	var err error
	switch node := arg.(type) {
	case *parse.DotNode:
		vExpr = compileDot(env)

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
		return nil, nil, err
	}

	if vExpr != nil {
		return vExpr, env.State(), nil
	}

	switch node := arg.(type) {
	case *parse.FieldNode:
		return compileField(env, compileDot(env), node.Ident, []parse.Node{arg}, nil)

	case *parse.VariableNode:
		return compileVariable(env, node, nil, nil)

	case *parse.PipeNode:
		return compilePipeline(env, node)

	case *parse.IdentifierNode:
		return compileFunction(env, node, nil, nil)

	case *parse.ChainNode:
		return compileChain(env, node, nil, nil)
	}

	return nil, nil, fmt.Errorf("compile Arg: not implemented: %v", reflect.TypeOf(arg))
}

func compileChain(
	e *env.T,
	chain *parse.ChainNode,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	if len(chain.Field) == 0 {
		return nil, nil, errors.New("compileChain: no fields")
	}
	if chain.Node.Type() == parse.NodeNil {
		return nil, nil, errors.New("nil indirection")
	}

	pipeExpr, pipeState, err := compileArg(e, chain.Node)
	if err != nil {
		return nil, nil, err
	}

	return pipeState.Use(
		func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			return compileField(e.WithVSAndH(vs, h), pipeExpr, chain.Field, args, final)
		},
	)
}

func compileField(
	e *env.T,
	initialReceiver *jsonnet.Expr,
	ident []string,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	argsExpr, argsState, err := compileArgs(e, args, final)
	if err != nil {
		return nil, nil, err
	}

	return argsState.Use(
		func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			if len(ident) == 0 {
				return initialReceiver, state.New(nil, vs, h), nil
			}

			receiver := initialReceiver
			for i := range len(ident) - 1 {
				receiver = &jsonnet.Expr{
					Kind: jsonnet.EIndex,
					BinOpLHS: jsonnet.CallField(
						h,
						receiver,
						&jsonnet.Expr{
							Kind:          jsonnet.EStringLiteral,
							StringLiteral: ident[i],
						},
						&jsonnet.Expr{
							Kind: jsonnet.EList,
							List: []*jsonnet.Expr{},
						},
					),
					BinOpRHS: &jsonnet.Expr{
						Kind:       jsonnet.EIntLiteral,
						IntLiteral: 1,
					},
				}
			}

			resultName := state.GenerateBindName()
			newState := state.New(
				[]*jsonnet.LocalBind{
					{Name: resultName, Body: jsonnet.CallField(
						h,
						receiver,
						&jsonnet.Expr{
							Kind:          jsonnet.EStringLiteral,
							StringLiteral: ident[len(ident)-1],
						},
						argsExpr,
					)},
				},
				vs,
				jsonnet.IndexInt(resultName, 0),
			)
			return jsonnet.IndexInt(resultName, 1), newState, nil
		},
	)
}

func compileVariable(
	e *env.T,
	node *parse.VariableNode,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	if _, ok := e.GetVariable(node.Ident[0]); !ok {
		return nil, nil, fmt.Errorf("variable not found: %s", node.Ident[0])
	}
	receiver := &jsonnet.Expr{
		Kind:     jsonnet.EIndex,
		BinOpLHS: e.VS(),
		BinOpRHS: &jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: node.Ident[0],
		},
	}
	if len(node.Ident) == 1 {
		return receiver, e.State(), nil
	}
	return compileField(e, receiver, node.Ident[1:], args, final)
}

func compileFunction(
	e *env.T,
	node *parse.IdentifierNode,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	if node.Ident == "and" || node.Ident == "or" { // short circuit
		if len(args) <= 1 {
			if final == nil {
				return nil, nil, errors.New("and/or with no args")
			}
			return final, e.State(), nil
		}

		vExpr := &jsonnet.Expr{}
		ifExpr, newState, err := sequential(
			e,
			args[1:],
			func(e *env.T, i int, arg parse.Node, acc *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
				vExpr, newState, err := compileArg(e, arg)
				if err != nil {
					return nil, nil, err
				}
				return newState.Use(func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
					if node.Ident == "and" {
						acc.IfThen = &jsonnet.Expr{
							Kind:   jsonnet.EIf,
							IfCond: jsonnet.CallIsTrueOnHeap(h, vExpr),
							IfThen: nil,
							IfElse: &jsonnet.Expr{
								Kind: jsonnet.EList,
								List: []*jsonnet.Expr{vExpr, vs, h},
							},
						}
						acc = acc.IfThen
					} else /* or */ {
						acc.IfElse = &jsonnet.Expr{
							Kind:   jsonnet.EIf,
							IfCond: jsonnet.CallIsTrueOnHeap(h, vExpr),
							IfThen: &jsonnet.Expr{
								Kind: jsonnet.EList,
								List: []*jsonnet.Expr{vExpr, vs, h},
							},
							IfElse: nil,
						}
						acc = acc.IfElse
					}
					return acc, state.New(nil, vs, h), nil
				})
			},
			vExpr,
		)
		if err != nil {
			return nil, nil, err
		}

		return newState.Use(func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			if node.Ident == "and" {
				vExpr = vExpr.IfThen

				if final == nil {
					ifExpr.IfThen = ifExpr.IfElse
				} else {
					ifExpr.IfThen = &jsonnet.Expr{
						Kind: jsonnet.EList,
						List: []*jsonnet.Expr{final, e.VS(), e.H()},
					}
				}
			} else /* or */ {
				vExpr = vExpr.IfElse

				if final == nil {
					ifExpr.IfElse = ifExpr.IfThen
				} else {
					ifExpr.IfElse = &jsonnet.Expr{
						Kind: jsonnet.EList,
						List: []*jsonnet.Expr{final, e.VS(), e.H()},
					}
				}
			}

			resultName := state.GenerateBindName()

			return jsonnet.IndexInt(resultName, 0),
				state.New([]*jsonnet.LocalBind{
					{Name: resultName, Body: vExpr},
				}, jsonnet.IndexInt(resultName, 1), jsonnet.IndexInt(resultName, 2)), nil
		})
	}

	argsExpr, argsState, err := compileArgs(e, args, final)
	if err != nil {
		return nil, nil, err
	}

	return argsState.Use(
		func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			if vExpr, newState, ok := compilePredefinedFunctions(
				e.WithVSAndH(vs, h),
				node.Ident,
				argsExpr,
			); ok {
				return vExpr, newState, nil
			}

			if _, ok := e.GetVariable(node.Ident); !ok {
				return nil, nil, fmt.Errorf("function not found: %s", node.Ident)
			}

			return nil, nil, errors.New("function call for not predefined functions is not implemented")
		},
	)
}

func compileArgs(
	e *env.T,
	args []parse.Node,
	final *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, error) {
	if len(args) <= 1 { // no arguments except `final`
		vExpr := &jsonnet.Expr{
			Kind: jsonnet.EList,
			List: []*jsonnet.Expr{},
		}
		if final != nil {
			vExpr.List = append(vExpr.List, final)
		}
		return vExpr, e.State(), nil
	}

	vExpr, newState, err := sequential(
		e,
		args[1:],
		func(
			e *env.T,
			_ int,
			arg parse.Node,
			acc *jsonnet.Expr,
		) (*jsonnet.Expr, *state.T, error) {
			vExpr, newState, err := compileArg(e, arg)
			if err != nil {
				return nil, nil, err
			}
			acc.List = append(acc.List, vExpr)
			return acc, newState, nil
		},
		&jsonnet.Expr{
			Kind: jsonnet.EList,
			List: []*jsonnet.Expr{},
		},
	)
	if err != nil {
		return nil, nil, err
	}
	if final != nil {
		vExpr.List = append(vExpr.List, final)
	}

	return vExpr, newState, nil
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

func compileDot(e *env.T) *jsonnet.Expr {
	return e.Dot()
}

func compileNil() *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ENull,
	}
}

func compileRange(e *env.T, node *parse.RangeNode) (*jsonnet.Expr, *state.T, error) {
	pipeExpr, pipeState, err := compilePipelineWithoutDecls(e, node.Pipe)
	if err != nil {
		return nil, nil, fmt.Errorf("compileRange: %w", err)
	}

	nestedVSName := state.GenerateBindName()
	nestedHName := state.GenerateBindName()
	dotName := state.GenerateBindName()

	thenExpr, thenState, err := e.WithVSAndH(
		jsonnet.Index(nestedVSName),
		jsonnet.Index(nestedHName),
	).WithDot(jsonnet.Index(dotName)).WithScope(
		func(e *env.T) (*jsonnet.Expr, *state.T, error) {
			for _, variable := range node.Pipe.Decl {
				e.DefineVariable(variable.Ident[0])
			}
			return compileNode(e, node.List)
		},
	)
	if err != nil {
		return nil, nil, err
	}

	return pipeState.Use(
		func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			elseExpr := jsonnet.EmptyString()
			elseState := state.New(nil, vs, h)
			if node.ElseList != nil {
				elseExpr, elseState, err = e.WithVSAndH(vs, h).WithScope(
					func(e *env.T) (*jsonnet.Expr, *state.T, error) {
						return compileNode(e, node.ElseList)
					},
				)
				if err != nil {
					return nil, nil, err
				}
			}

			assignments := []*jsonnet.MapEntry{}
			switch len(node.Pipe.Decl) {
			case 0:
			// do nothing
			case 1:
				assignments = append(assignments, &jsonnet.MapEntry{
					K: &jsonnet.Expr{
						Kind:          jsonnet.EStringLiteral,
						StringLiteral: node.Pipe.Decl[0].Ident[0],
					},
					V: jsonnet.Index(dotName),
				})
			case 2:
				assignments = append(assignments, &jsonnet.MapEntry{
					K: &jsonnet.Expr{
						Kind:          jsonnet.EStringLiteral,
						StringLiteral: node.Pipe.Decl[0].Ident[0],
					},
					V: jsonnet.Index("i"),
				})
				assignments = append(assignments, &jsonnet.MapEntry{
					K: &jsonnet.Expr{
						Kind:          jsonnet.EStringLiteral,
						StringLiteral: node.Pipe.Decl[1].Ident[0],
					},
					V: jsonnet.Index(dotName),
				})
			default:
				return nil, nil, fmt.Errorf("compileNode: not implemented: len(node.Pipe.Decl) > 2")
			}

			nestedHValue := jsonnet.Index("h")
			nestedVSValue := jsonnet.Index("vs")
			if len(assignments) > 0 {
				nestedVSValue = jsonnet.AddMap(nestedVSValue, assignments)
			}

			resultName := state.GenerateBindName()
			newState := state.New(
				[]*jsonnet.LocalBind{
					{Name: resultName, Body: jsonnet.CallRange(
						vs,
						h,
						pipeExpr,
						&jsonnet.Expr{
							Kind:           jsonnet.EFunction,
							FunctionParams: []string{"vs", "h", "i", dotName},
							FunctionBody: &jsonnet.Expr{
								Kind: jsonnet.ELocal,
								LocalBinds: []*jsonnet.LocalBind{
									{Name: nestedVSName, Body: nestedVSValue},
									{Name: nestedHName, Body: nestedHValue},
								},
								LocalBody: thenState.Finalize(thenExpr),
							},
						},
						&jsonnet.Expr{
							Kind:           jsonnet.EFunction,
							FunctionParams: []string{},
							FunctionBody:   elseState.Finalize(elseExpr),
						},
					)},
				},
				jsonnet.IndexInt(resultName, 1),
				jsonnet.IndexInt(resultName, 2),
			)
			return jsonnet.IndexInt(resultName, 0), newState, nil
		},
	)
}

func compileIfOrWith(
	e *env.T,
	typ parse.NodeType,
	pipe *parse.PipeNode,
	list *parse.ListNode,
	elseList *parse.ListNode,
) (*jsonnet.Expr, *state.T, error) {
	return e.WithScope(
		func(e *env.T) (*jsonnet.Expr, *state.T, error) {
			pipeExpr, pipeState, err := compilePipeline(e, pipe)
			if err != nil {
				return nil, nil, err
			}

			return pipeState.Use(
				func(vs, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
					dotName := state.GenerateBindName()
					enhancedE := e.WithVSAndH(vs, h)
					if typ == parse.NodeWith {
						enhancedE = enhancedE.WithDot(jsonnet.Index(dotName))
					}
					thenExpr, thenState, err := enhancedE.WithScope(
						func(env *env.T) (*jsonnet.Expr, *state.T, error) {
							vExpr, newState, err := compileNode(env, list)
							if err != nil {
								return nil, nil, err
							}
							if typ == parse.NodeWith {
								newState.PrependLocalBind(&jsonnet.LocalBind{
									Name: dotName,
									Body: pipeExpr,
								})
							}
							return vExpr, newState, nil
						},
					)
					if err != nil {
						return nil, nil, err
					}

					elseExpr := jsonnet.EmptyString()
					elseState := state.New(nil, vs, h)
					if elseList != nil {
						elseExpr, elseState, err = e.WithVSAndH(vs, h).WithScope(
							func(e *env.T) (*jsonnet.Expr, *state.T, error) {
								return compileNode(e, elseList)
							},
						)
						if err != nil {
							return nil, nil, err
						}
					}

					result := &jsonnet.Expr{
						Kind:   jsonnet.EIf,
						IfCond: jsonnet.CallIsTrueOnHeap(h, pipeExpr),
						IfThen: thenState.Finalize(thenExpr),
						IfElse: elseState.Finalize(elseExpr),
					}
					resultName := state.GenerateBindName()
					newState := state.New(
						[]*jsonnet.LocalBind{{Name: resultName, Body: result}},
						jsonnet.IndexInt(resultName, 1),
						jsonnet.IndexInt(resultName, 2),
					)
					return jsonnet.IndexInt(resultName, 0), newState, nil
				},
			)
		},
	)
}

func compilePredefinedFunctions(
	e *env.T,
	ident string,
	compiledArgs *jsonnet.Expr,
) (*jsonnet.Expr, *state.T, bool) {
	switch ident {
	case
		"add",
		"b64enc",
		"ceil",
		"contains",
		"dir",
		"div",
		"divf",
		"eq",
		"fail",
		"gt",
		"indent",
		"int",
		"int64",
		"lower",
		"min",
		"mul",
		"mulf",
		"mustRegexReplaceAllLiteral",
		"ne",
		"nindent",
		"print",
		"printf",
		"quote",
		"regexReplaceAll",
		"regexReplaceAllLiteral",
		"replace",
		"required",
		"semverCompare",
		"sha256sum",
		"squote",
		"ternary",
		"toString",
		"trim",
		"trimAll",
		"trimSuffix",
		"trunc":
		vExpr := &jsonnet.Expr{
			Kind:     jsonnet.ECall,
			CallFunc: jsonnet.Index(ident),
			CallArgs: []*jsonnet.Expr{
				compiledArgs,
			},
		}
		return vExpr, e.State(), true

	case
		"concat",
		"dateInZone",
		"fromJson",
		"fromYaml",
		"has",
		"hasKey",
		"now",
		"omit",
		"toYaml",
		"typeIs":
		resultName := state.GenerateBindName()
		newState := state.New(
			[]*jsonnet.LocalBind{{
				Name: resultName,
				Body: &jsonnet.Expr{
					Kind: jsonnet.ECall,
					CallFunc: &jsonnet.Expr{
						Kind: jsonnet.ERaw,
						Raw:  `callBuiltin`,
					},
					CallArgs: []*jsonnet.Expr{
						e.H(),
						jsonnet.Index(ident),
						compiledArgs,
					},
				},
			}},
			e.VS(),
			jsonnet.IndexInt(resultName, 0),
		)
		return jsonnet.IndexInt(resultName, 1), newState, true

	case
		"add1",
		"and",
		"append",
		"b64dec",
		"base",
		"camelcase",
		"clean",
		"coalesce",
		"compact",
		"deepCopy",
		"default",
		"dict",
		"dig",
		"empty",
		"ext",
		"first",
		"fromYamlArray",
		"ge",
		"genCA",
		"genSignedCert",
		"get",
		"hasPrefix",
		"hasSuffix",
		"include",
		"index",
		"join",
		"kebabcase",
		"keys",
		"kindIs",
		"kindOf",
		"last",
		"len",
		"list",
		"lookup",
		"lt",
		"merge",
		"mergeOverwrite",
		"mustMerge",
		"mustUniq",
		"not",
		"or",
		"randAlphaNum",
		"regexFind",
		"regexMatch",
		"regexSplit",
		"reverse",
		"set",
		"sortAlpha",
		"split",
		"splitList",
		"sub",
		"toJson",
		"toRawJson",
		"tpl",
		"tuple",
		"typeOf",
		"uniq",
		"unset",
		"until",
		"untitle",
		"upper",
		"urlParse",
		"without":
		resultName := state.GenerateBindName()
		newState := state.New(
			[]*jsonnet.LocalBind{{
				Name: resultName,
				Body: &jsonnet.Expr{
					Kind:     jsonnet.ECall,
					CallFunc: jsonnet.Index(ident),
					CallArgs: []*jsonnet.Expr{
						jsonnet.Map(map[string]*jsonnet.Expr{
							"$": {
								Kind:   jsonnet.EID,
								IDName: "$",
							},
							"args": compiledArgs,
							"vs":   e.VS(),
							"h":    e.H(),
						}),
					},
				},
			}},
			jsonnet.IndexInt(resultName, 1),
			jsonnet.IndexInt(resultName, 2),
		)
		return jsonnet.IndexInt(resultName, 0), newState, true
	}

	return nil, nil, false
}
