package env

import (
	"fmt"
	"text/template"

	"github.com/ushitora-anqou/helmhammer/compiler/state"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

type variableT struct {
	defined bool
}

type scopeT struct {
	parent    *scopeT
	variables map[string]*variableT
}

func (sc *scopeT) defineVariable(name string) error {
	_, ok := sc.variables[name]
	if ok {
		return fmt.Errorf("variable already defined: %s", name)
	}
	sc.variables[name] = &variableT{
		defined: true,
	}
	return nil
}

func (sc *scopeT) assignVariable(name string) error {
	if _, ok := sc.variables[name]; ok { // defined or assigned in this scope
		return nil
	}
	if sc.parent != nil {
		if _, ok := sc.parent.getVariable(name); !ok {
			return fmt.Errorf("variable not found: %s", name)
		}
	}
	sc.variables[name] = &variableT{
		defined: false,
	}
	return nil
}

func (sc *scopeT) getVariable(name string) (*variableT, bool) {
	v, ok := sc.variables[name]
	if ok {
		return v, true
	}
	if sc.parent == nil {
		return nil, false
	}
	return sc.parent.getVariable(name)
}

type T struct {
	tmpl       *template.Template
	scope      *scopeT
	vs, h, dot *jsonnet.Expr
}

func New(tmpl *template.Template) *T {
	return &T{
		tmpl: tmpl,
		scope: &scopeT{
			parent:    nil,
			variables: map[string]*variableT{},
		},
		vs: jsonnet.EmptyMap(),
		h:  jsonnet.EmptyMap(),
	}
}

func newT(
	tmpl *template.Template,
	scope *scopeT,
	vs, h, dot *jsonnet.Expr,
) *T {
	return &T{
		tmpl:  tmpl,
		scope: scope,
		vs:    vs,
		h:     h,
		dot:   dot,
	}
}

func (e *T) Template() *template.Template {
	return e.tmpl
}

func (e *T) VS() *jsonnet.Expr {
	return e.vs
}

func (e *T) H() *jsonnet.Expr {
	return e.h
}

func (e *T) State() *state.T {
	return state.New([]*jsonnet.LocalBind{}, e.VS(), e.H())
}

func (e *T) WithVSAndH(vs *jsonnet.Expr, h *jsonnet.Expr) *T {
	return newT(e.tmpl, e.scope, vs, h, e.dot)
}

func (e *T) DefineVariable(name string) error {
	return e.scope.defineVariable(name)
}

func (e *T) AssignVariable(name string) error {
	return e.scope.assignVariable(name)
}

func (e *T) GetVariable(name string) (*variableT, bool) {
	return e.scope.getVariable(name)
}

func (e *T) WithScope(
	nested func(*T) (*jsonnet.Expr, *state.T, error),
) (*jsonnet.Expr, *state.T, error) {
	newEnv := newT(
		e.tmpl,
		&scopeT{
			parent:    e.scope,
			variables: make(map[string]*variableT),
		},
		e.vs,
		e.h,
		e.dot,
	)
	vExpr, newState, err := nested(newEnv)
	if err != nil {
		return nil, nil, err
	}

	outerVS := e.VS()
	return newState.Use(
		func(vs *jsonnet.Expr, h *jsonnet.Expr) (*jsonnet.Expr, *state.T, error) {
			// { ..., NAME: vs.[name], ... }
			// if v is assigned in the scope
			// for (name, v) in env.variables
			assignedVars := make(map[*jsonnet.Expr]*jsonnet.Expr)
			for name, v := range newEnv.scope.variables {
				if v.defined {
					continue
				}

				// propagate assignments to the parent scope
				if err := e.AssignVariable(name); err != nil {
					return nil, nil, err
				}

				// { ..., NAME: sX.vs.NAME, ... }
				assignedVars[&jsonnet.Expr{
					Kind:          jsonnet.EStringLiteral,
					StringLiteral: name,
				}] = &jsonnet.Expr{
					Kind:          jsonnet.EIndexList,
					IndexListHead: vs,
					IndexListTail: []string{name},
				}
			}

			if len(assignedVars) == 0 {
				return vExpr, state.New(nil, outerVS, h), nil
			}

			newVSName := state.GenerateBindName()
			return vExpr, state.New([]*jsonnet.LocalBind{
				{Name: newVSName, Body: jsonnet.AddMap(outerVS, assignedVars)},
			}, jsonnet.Index(newVSName), h), nil
		},
	)
}

func (e *T) WithDot(expr *jsonnet.Expr) *T {
	return newT(e.tmpl, e.scope, e.vs, e.h, expr)
}

func (e *T) Dot() *jsonnet.Expr {
	return e.dot
}
