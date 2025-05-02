package jsonnet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ushitora-anqou/helmhammer/jsonnet"
)

func TestConvertIntoJsonnet(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		output *jsonnet.Expr
	}{
		{"int", 1000, &jsonnet.Expr{Kind: jsonnet.EIntLiteral, IntLiteral: 1000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonnet.ConvertIntoJsonnet(tt.input)
			assert.Equal(t, tt.output, got)
		})
	}
}
