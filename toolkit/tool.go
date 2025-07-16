package toolkit

import "github.com/bosley/beau"

type tooling struct {
	toolDefinition beau.Tool
	executor       func(input []byte) (interface{}, error)
}

var _ LlmTool = &tooling{}

func NewTool(
	schema beau.ToolSchema,
	executor func(input []byte) (interface{}, error)) *tooling {
	return &tooling{
		toolDefinition: beau.Tool{
			Type:     "function",
			Function: schema,
		},
		executor: executor,
	}
}

func (t *tooling) GetDefinition() beau.Tool {
	return t.toolDefinition
}

func (t *tooling) Call(input []byte) (interface{}, error) {
	return t.executor(input)
}
