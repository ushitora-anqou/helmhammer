package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/helm"
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
	if len(os.Args) <= 1 {
		return errors.New("chart dir not specified")
	}

	tmpl, err := helm.Load(os.Args[1])
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	expr, err := compiler.Compile(tmpl)
	if err != nil {
		return fmt.Errorf("failed to compile")
	}

	print(expr.StringWithPrologue())

	return nil
}

func main() {
	if err := doMain(); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}
