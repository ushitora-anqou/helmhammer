package compiler

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"text/template/parse"

	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

func Compile(node parse.Node) (*jsonnet.Expr, error) {
	e, err := compileNode(node)
	if err != nil {
		return nil, err
	}

	e = &jsonnet.Expr{
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
			},
			LocalBody: e,
		},
	}

	return e, nil
}

func compileNode(node parse.Node) (*jsonnet.Expr, error) {
	switch node := node.(type) {
	case *parse.ActionNode:
	case *parse.BreakNode:
	case *parse.CommentNode:
	case *parse.ContinueNode:

	case *parse.IfNode:
		pipe, err := compilePipeline(node.Pipe)
		if err != nil {
			return nil, err
		}
		list, err := compileNode(node.List)
		if err != nil {
			return nil, err
		}
		elseList, err := compileNode(node.ElseList)
		if err != nil {
			return nil, err
		}
		return &jsonnet.Expr{
			Kind:   jsonnet.EIf,
			IfCond: pipe,
			IfThen: list,
			IfElse: elseList,
		}, nil

	case *parse.ListNode:
		list := []*jsonnet.Expr{}
		for _, node := range node.Nodes {
			e, err := compileNode(node)
			if err != nil {
				return nil, err
			}
			list = append(list, e)
		}
		return &jsonnet.Expr{
			Kind: jsonnet.ECall,
			CallFunc: &jsonnet.Expr{
				Kind:          jsonnet.EIndexList,
				IndexListHead: &jsonnet.Expr{Kind: jsonnet.EID, IDName: "std"},
				IndexListTail: []string{"join"},
			},
			CallArgs: []*jsonnet.Expr{
				{Kind: jsonnet.EStringLiteral, StringLiteral: ""},
				{Kind: jsonnet.EList, List: list},
			},
		}, nil

	case *parse.RangeNode:
	case *parse.TemplateNode:

	case *parse.TextNode:
		return &jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: string(node.Text),
		}, nil

	case *parse.WithNode:
	}
	return nil, fmt.Errorf("unknown node: %s", node)
}

func compilePipeline(pipe *parse.PipeNode) (*jsonnet.Expr, error) {
	if pipe == nil {
		return nil, errors.New("pipe is nil")
	}

	var expr *jsonnet.Expr
	for _, cmd := range pipe.Cmds {
		var err error
		expr, err = compileCommand(cmd, expr)
		if err != nil {
			return nil, err
		}
	}

	for _, variable := range pipe.Decl {
		if pipe.IsAssign {
			log.Printf(">> %s", variable.Ident[0])
		} else {
			log.Printf(">> %s", variable.Ident[0])
		}
	}

	return expr, nil
}

func compileCommand(cmd *parse.CommandNode, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	switch node := cmd.Args[0].(type) {
	case *parse.FieldNode:
		return compileField(node, cmd.Args, final)

	case *parse.ChainNode:
	case *parse.IdentifierNode:
	case *parse.PipeNode:
	case *parse.VariableNode:

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
	return nil, errors.New("invalid command")
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
		return compileField(node, []parse.Node{arg}, nil)

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

func compileField(node *parse.FieldNode, args []parse.Node, final *jsonnet.Expr) (*jsonnet.Expr, error) {
	receiver := compileDot()
	if len(node.Ident) >= 2 {
		receiver = &jsonnet.Expr{
			Kind:          jsonnet.EIndexList,
			IndexListHead: receiver,
			IndexListTail: node.Ident[0 : len(node.Ident)-1],
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
				StringLiteral: node.Ident[len(node.Ident)-1],
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
