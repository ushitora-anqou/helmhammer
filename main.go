package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/ushitora-anqou/helmhammer/compiler"
	"github.com/ushitora-anqou/helmhammer/helm"
)

func doMain() error {
	if len(os.Args) <= 1 {
		return errors.New("chart dir not specified")
	}

	chart, err := helm.Load(os.Args[1])
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	expr, err := compiler.CompileChart(chart)
	if err != nil {
		return fmt.Errorf("faield to compile chart: %w", err)
	}

	fmt.Print(expr.StringWithPrologue())

	return nil
}

func main() {
	if err := doMain(); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}
