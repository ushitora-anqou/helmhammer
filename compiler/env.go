package compiler

import (
	"fmt"
	"text/template"

	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

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

type envT struct {
	tmpl         *template.Template
	scope        *scopeT
	preStateName string
}

func newEnv(tmpl *template.Template, scope *scopeT, preStateName string) *envT {
	return &envT{
		tmpl,
		scope,
		preStateName,
	}
}

func (e *envT) defineVariable(name string) error {
	return e.scope.defineVariable(name)
}

func (e *envT) assignVariable(name string) error {
	return e.scope.assignVariable(name)
}

func (e *envT) getVariable(name string) (*variable, bool) {
	return e.scope.getVariable(name)
}

func (e *envT) withPreState(name string) *envT {
	return newEnv(e.tmpl, e.scope, name)
}

func withScope(
	env *envT,
	nestedPreStateName stateName,
	nested func(*envT) (*state, error),
) (*state, error) {
	newEnv := newEnv(
		env.tmpl,
		&scope{
			parent:    env.scope,
			variables: make(map[string]*variable),
		},
		nestedPreStateName,
	)
	nestedPostState, err := nested(newEnv)
	if err != nil {
		return nil, err
	}
	nestedPostStateName := generateStateName()

	// { ..., NAME: [preStateName].vs.[name], ... }
	// if v is assigned in the scope
	// for (name, v) in env.variables
	assignedVars := make(map[*jsonnet.Expr]*jsonnet.Expr)
	for name, v := range newEnv.scope.variables {
		if v.defined {
			continue
		}

		// propagate assignments to the parent scope
		if err := env.assignVariable(name); err != nil {
			return nil, err
		}

		// { ..., NAME: sX.vs.NAME, ... }
		assignedVars[&jsonnet.Expr{
			Kind:          jsonnet.EStringLiteral,
			StringLiteral: name,
		}] = jsonnet.Index(nestedPostStateName, stateVS, name)
	}

	// local [nestedPostState.name] = [nestedPostState.body];
	// {
	//   v: [nestedPostState.v],
	//   vs: [preStateName].vs + [assignedVars],
	//   h: [nestedPostState.h],
	// }
	expr := &jsonnet.Expr{
		Kind: jsonnet.ELocal,
		LocalBinds: []*jsonnet.LocalBind{
			{Name: nestedPostStateName, Body: nestedPostState.body},
		},
		LocalBody: newState(
			jsonnet.Index(nestedPostStateName, stateV),
			jsonnet.AddMap(jsonnet.Index(nestedPreStateName, stateVS), assignedVars),
			jsonnet.Index(nestedPostStateName, stateH),
		).body,
	}

	return &state{body: expr}, nil
}
