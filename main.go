package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"text/template"
	"text/template/parse"
)

var file1 = `hel{{ if true }} lo2 {{ else }} lo3 {{ end }}`
var file2 = `world`

var tpls = map[string]struct {
	tpl string
}{
	"file1": {tpl: file1},
	"file2": {tpl: file2},
}

func doMain() error {
	t := template.New("gotpl")
	keys := []string{"file1", "file2"}

	for _, filename := range keys {
		r := tpls[filename]
		if _, err := t.New(filename).Parse(r.tpl); err != nil {
			return fmt.Errorf("faield to parse: %s: %w", filename, err)
		}
	}

	t0 := t.Lookup(keys[0])
	if t0.Tree == nil || t0.Root == nil {
		return errors.New("failed to lookup")
	}

	out, err := compile(t0.Root)
	if err != nil {
		return fmt.Errorf("failed to walk: %w", err)
	}

	out = &JsonnetExpr{
		Kind:         ECall,
		CallFuncName: "std.deepJoin",
		CallArgs:     []*JsonnetExpr{out},
	}

	print(out.String())

	return nil
}

func main() {
	if err := doMain(); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

const (
	EStringLiteral = iota
	ETrue
	EList
	EIf
	ECall
)

type JsonnetExpr struct {
	Kind          int
	StringLiteral string
	List          []*JsonnetExpr
	IfCond        *JsonnetExpr
	IfThen        *JsonnetExpr
	IfElse        *JsonnetExpr
	CallFuncName  string
	CallArgs      []*JsonnetExpr
}

func (e *JsonnetExpr) String() string {
	switch e.Kind {
	case EStringLiteral:
		return fmt.Sprintf("\"%s\"", e.StringLiteral) // FIXME: escape

	case ETrue:
		return "true"

	case EList:
		b := strings.Builder{}
		b.WriteString("[")
		for _, head := range e.List {
			b.WriteString(head.String())
			b.WriteString(",")
		}
		b.WriteString("]")
		return b.String()

	case EIf:
		return fmt.Sprintf("if (%s) then (%s) else (%s)",
			e.IfCond.String(), e.IfThen.String(), e.IfElse.String())

	case ECall:
		b := strings.Builder{}
		b.WriteString(e.CallFuncName)
		b.WriteString("(")
		for _, arg := range e.CallArgs {
			b.WriteString(arg.String())
			b.WriteString(",")
		}
		b.WriteString(")")
		return b.String()
	}

	panic("JsonnetExpr.String: invalid kind")
}

func compile(node parse.Node) (*JsonnetExpr, error) {
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
		list, err := compile(node.List)
		if err != nil {
			return nil, err
		}
		elseList, err := compile(node.ElseList)
		if err != nil {
			return nil, err
		}
		return &JsonnetExpr{
			Kind:   EIf,
			IfCond: pipe,
			IfThen: list,
			IfElse: elseList,
		}, nil

	case *parse.ListNode:
		list := []*JsonnetExpr{}
		for _, node := range node.Nodes {
			e, err := compile(node)
			if err != nil {
				return nil, err
			}
			list = append(list, e)
		}
		return &JsonnetExpr{Kind: EList, List: list}, nil

	case *parse.RangeNode:
	case *parse.TemplateNode:

	case *parse.TextNode:
		return &JsonnetExpr{
			Kind:          EStringLiteral,
			StringLiteral: string(node.Text),
		}, nil

	case *parse.WithNode:
	}
	return nil, fmt.Errorf("unknown node: %s", node)
}

func compilePipeline(pipe *parse.PipeNode) (*JsonnetExpr, error) {
	return &JsonnetExpr{Kind: ETrue}, nil
}
