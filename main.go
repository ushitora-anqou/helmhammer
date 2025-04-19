package main

import (
	"errors"
	"fmt"
	"log"
	"text/template"

	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

var file1 = `{{$}}`
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
			return fmt.Errorf("failed to parse: %s: %w", filename, err)
		}
	}

	t0 := t.Lookup(keys[0])
	if t0.Tree == nil || t0.Root == nil {
		return errors.New("failed to lookup")
	}

	out, err := compiler.Compile(t0.Root)
	if err != nil {
		return fmt.Errorf("failed to walk: %w", err)
	}
	out = &jsonnet.Expr{
		Kind:     jsonnet.ECall,
		CallFunc: out,
		CallArgs: []*jsonnet.Expr{
			jsonnet.ConvertDataToJsonnetExpr(
				map[string]any{
					"X": "x",
					"U": map[string]string{
						"V": "v",
					},
					"MSI": map[string]int{
						"one": 1,
						"two": 2,
					},
				},
			),
		},
	}

	print(out.StringWithPrologue())

	return nil
}

func main() {
	if err := doMain(); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}
