package state

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

// T is context of execution of text/template's Node.
// It is a scalar value or an array having the following values:
// - `vs`: a variable map (name |-> value, optional)
// - `h`: heap storing values (optional)
type T struct {
	localBinds []*jsonnet.LocalBind
	vs, h      *jsonnet.Expr
}

func New(localBinds []*jsonnet.LocalBind, vs, h *jsonnet.Expr) *T {
	return &T{
		localBinds: localBinds,
		vs:         vs,
		h:          h,
	}
}

func (t *T) Use(
	f func(vs *jsonnet.Expr, h *jsonnet.Expr) (*jsonnet.Expr, *T, error),
) (*jsonnet.Expr, *T, error) {
	v, nested, err := f(t.vs, t.h)
	if err != nil {
		return nil, nil, err
	}
	if nested == nil {
		return nil, nil, errors.New("nested is nil")
	}
	newState := &T{
		localBinds: slices.Concat(t.localBinds, nested.localBinds),
		vs:         nested.vs,
		h:          nested.h,
	}
	return v, newState, nil
}

func (t *T) Finalize(v *jsonnet.Expr) *jsonnet.Expr {
	body := &jsonnet.Expr{
		Kind: jsonnet.EList,
		List: []*jsonnet.Expr{v, t.vs, t.h},
	}
	if t.localBinds == nil || len(t.localBinds) == 0 {
		return body
	}
	return &jsonnet.Expr{
		Kind:       jsonnet.ELocal,
		LocalBinds: t.localBinds,
		LocalBody:  body,
	}
}

func (t *T) PrependLocalBind(b *jsonnet.LocalBind) {
	t.localBinds = slices.Concat([]*jsonnet.LocalBind{b}, t.localBinds)
}

var nextGenID = 0

func genid() int {
	nextGenID++
	return nextGenID
}

func GenerateBindName() string {
	return fmt.Sprintf("t%d", genid())
}
