package compiler

import (
	"errors"
	"fmt"

	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

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

func newStateSameVS(env *envT, v *jsonnet.Expr) *state {
	return newState(v, jsonnet.Index(env.preStateName, stateVS))
}

func (s *state) toLocal(stateName stateName, localBody *jsonnet.Expr) *jsonnet.Expr {
	return &jsonnet.Expr{
		Kind: jsonnet.ELocal,
		LocalBinds: []*jsonnet.LocalBind{
			{
				Name: stateName,
				Body: s.body,
			},
		},
		LocalBody: localBody,
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
	id := genid()
	return fmt.Sprintf("s%d", id)
}

var nextGenID = 0

func genid() int {
	nextGenID++
	return nextGenID
}

func sequentialStates[T any](
	env *envT,
	items []T,
	fIter func(*envT, int, T) (*state, error),
	fBody func([]stateName) (*state, error),
) (*state, error) {
	states := []*state{}
	stateNames := []string{}
	stateName := env.preStateName
	for i, item := range items {
		newState, err := fIter(env.withPreState(stateName), i, item)
		if err != nil {
			return nil, err
		}
		newStateName := generateStateName()
		states = append(states, newState)
		stateNames = append(stateNames, newStateName)
		stateName = newStateName
	}

	if len(states) == 0 {
		return nil, errors.New("sequentialStates: no available states")
	}

	if fBody == nil {
		body := states[len(states)-1].body
		for i := len(states) - 2; i >= 0; i-- {
			body = states[i].toLocal(stateNames[i], body)
		}
		return &state{body: body}, nil
	}

	finalState, err := fBody(stateNames)
	if err != nil {
		return nil, err
	}
	body := finalState.body
	for i := len(states) - 1; i >= 0; i-- {
		body = states[i].toLocal(stateNames[i], body)
	}
	return &state{body: body}, nil
}
