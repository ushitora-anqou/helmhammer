package main

import (
	"errors"
	"fmt"
	"log"
	"text/template"

	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

var file1 = `{{ $x := 1 }}{{ if true }}{{ $x = 2 }}{{ if true }}{{ $x = 3 }}{{ end }}{{ end }}{{ $x }}`
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

	out, err := compiler.Compile(t)
	if err != nil {
		return fmt.Errorf("failed to walk: %w", err)
	}
	out = &jsonnet.Expr{
		Kind: jsonnet.ECall,
		CallFunc: &jsonnet.Expr{
			Kind:          jsonnet.EIndexList,
			IndexListHead: out,
			IndexListTail: []string{"file1"},
		},
		CallArgs: []*jsonnet.Expr{
			{
				Kind: jsonnet.ERaw,
				Raw:  `{SI: [1,2,3]}`,
			},
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
