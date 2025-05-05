package compiler

import (
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
